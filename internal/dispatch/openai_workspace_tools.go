package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	openAIWorkspaceMaxFileBytes      = 128 * 1024
	openAIWorkspaceMaxCommandBytes   = 64 * 1024
	openAIWorkspaceDefaultListLimit  = 200
	openAIWorkspaceDefaultCmdTimeout = 30 * time.Second
)

type openAIFunctionToolDefinition struct {
	Name        string
	Description string
	Parameters  any
}

func openAIWorkspaceToolDefinitions() []openAIFunctionToolDefinition {
	return []openAIFunctionToolDefinition{
		{
			Name:        "workspace_list_files",
			Description: "List files under the current ticket workspace. Paths must stay within the workspace root.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to list. Use . for the workspace root.",
					},
					"max_entries": map[string]any{
						"type":        "integer",
						"description": "Maximum number of entries to return.",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "workspace_read_file",
			Description: "Read a UTF-8 text file from the current ticket workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file to read.",
					},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "workspace_write_file",
			Description: "Create or overwrite a UTF-8 text file in the current ticket workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path to the file to write.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full file contents to write.",
					},
				},
				"required":             []string{"path", "content"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "workspace_run_command",
			Description: "Run a shell command inside the current ticket workspace and return its combined stdout and stderr.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to execute in the workspace.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Optional timeout in seconds. Default 30.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
	}
}

func openAIWorkspaceTools() []openAIResponsesTool {
	return openAIResponsesFunctionTools(openAIWorkspaceToolDefinitions())
}

func openAIResponsesFunctionTools(defs []openAIFunctionToolDefinition) []openAIResponsesTool {
	tools := make([]openAIResponsesTool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, openAIResponsesTool{
			Type:        "function",
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
			Strict:      true,
		})
	}
	return tools
}

func openAIChatFunctionTools(defs []openAIFunctionToolDefinition) []openAIChatTool {
	tools := make([]openAIChatTool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, openAIChatTool{
			Type: "function",
			Function: openAIChatToolFunction{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			},
		})
	}
	return tools
}

func isOpenAIWorkspaceTool(name string) bool {
	for _, def := range openAIWorkspaceToolDefinitions() {
		if def.Name == name {
			return true
		}
	}
	return false
}

func executeOpenAIWorkspaceTool(ctx context.Context, workDir string, call openAIResponsesOutputItem) string {
	return executeOpenAIFunctionTool(ctx, workDir, call.Name, call.Arguments)
}

func executeOpenAIFunctionTool(ctx context.Context, workDir, name, rawArguments string) string {
	switch name {
	case "workspace_list_files":
		var args struct {
			Path       string `json:"path"`
			MaxEntries int    `json:"max_entries"`
		}
		if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
			return jsonToolError(err)
		}
		return workspaceListFiles(workDir, args.Path, args.MaxEntries)
	case "workspace_read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
			return jsonToolError(err)
		}
		return workspaceReadFile(workDir, args.Path)
	case "workspace_write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
			return jsonToolError(err)
		}
		return workspaceWriteFile(workDir, args.Path, args.Content)
	case "workspace_run_command":
		var args struct {
			Command        string `json:"command"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
			return jsonToolError(err)
		}
		return workspaceRunCommand(ctx, workDir, args.Command, args.TimeoutSeconds)
	default:
		return jsonToolError(fmt.Errorf("unknown workspace tool %q", name))
	}
}

func workspaceListFiles(workDir, rel string, maxEntries int) string {
	root, relPath, err := resolveWorkspacePath(workDir, rel, true)
	if err != nil {
		return jsonToolError(err)
	}
	baseAbs, err := filepath.Abs(workDir)
	if err != nil {
		return jsonToolError(err)
	}
	if maxEntries <= 0 {
		maxEntries = openAIWorkspaceDefaultListLimit
	}

	entries := make([]string, 0, maxEntries)
	truncated := false
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		if len(entries) >= maxEntries {
			truncated = true
			return filepath.SkipAll
		}
		relEntry, err := filepath.Rel(baseAbs, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(relEntry))
		return nil
	})
	if err != nil {
		return jsonToolError(err)
	}
	return workspaceMustJSON(map[string]any{
		"ok":          true,
		"path":        relPath,
		"entries":     entries,
		"truncated":   truncated,
		"max_entries": maxEntries,
	})
}

func workspaceReadFile(workDir, rel string) string {
	path, relPath, err := resolveWorkspacePath(workDir, rel, false)
	if err != nil {
		return jsonToolError(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return jsonToolError(err)
	}
	truncated := false
	if len(data) > openAIWorkspaceMaxFileBytes {
		data = data[:openAIWorkspaceMaxFileBytes]
		truncated = true
	}
	return workspaceMustJSON(map[string]any{
		"ok":        true,
		"path":      relPath,
		"content":   string(data),
		"truncated": truncated,
	})
}

func workspaceWriteFile(workDir, rel, content string) string {
	path, relPath, err := resolveWorkspacePath(workDir, rel, false)
	if err != nil {
		return jsonToolError(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return jsonToolError(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return jsonToolError(err)
	}
	return workspaceMustJSON(map[string]any{
		"ok":   true,
		"path": relPath,
	})
}

func workspaceRunCommand(parent context.Context, workDir, command string, timeoutSeconds int) string {
	if strings.TrimSpace(command) == "" {
		return jsonToolError(fmt.Errorf("command is required"))
	}
	timeout := openAIWorkspaceDefaultCmdTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	output := string(out)
	truncated := false
	if len(output) > openAIWorkspaceMaxCommandBytes {
		output = output[:openAIWorkspaceMaxCommandBytes]
		truncated = true
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			exitCode = -1
		} else {
			exitCode = 1
		}
	}

	return workspaceMustJSON(map[string]any{
		"ok":          err == nil,
		"exit_code":   exitCode,
		"output":      output,
		"timed_out":   ctx.Err() == context.DeadlineExceeded,
		"truncated":   truncated,
		"command":     command,
		"working_dir": filepath.ToSlash(workDir),
	})
}

func resolveWorkspacePath(workDir, rel string, allowDir bool) (string, string, error) {
	if workDir == "" {
		return "", "", fmt.Errorf("workspace directory is required")
	}
	baseAbs, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", err
	}
	if rel == "" {
		rel = "."
	}
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(baseAbs, rel)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	relativeToBase, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", "", err
	}
	if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q escapes workspace", rel)
	}
	info, statErr := os.Stat(targetAbs)
	if statErr == nil && info.IsDir() && !allowDir {
		return "", "", fmt.Errorf("path %q is a directory", rel)
	}
	if statErr != nil && allowDir && !os.IsNotExist(statErr) {
		return "", "", statErr
	}
	return targetAbs, filepath.ToSlash(relativeToBase), nil
}

func jsonToolError(err error) string {
	return workspaceMustJSON(map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
}

func workspaceMustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
