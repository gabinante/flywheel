package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkerResult is the outcome of a worker execution.
type WorkerResult struct {
	Success bool
	Output  string
	Error   string
}

// Worker spawns a Claude Code session for a ticket.
type Worker interface {
	Spawn(ctx context.Context, ticketID, systemPrompt, workDir, serverURL string) (*WorkerResult, error)
}

// mcpConfig is the MCP configuration file structure for Claude Code.
type mcpConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func buildMCPConfig(serverURL, apiKey string) mcpConfig {
	serverCfg := mcpServerConfig{Type: "sse", URL: serverURL + "/sse"}
	if apiKey != "" {
		serverCfg.Headers = map[string]string{"X-API-Key": apiKey}
	}
	return mcpConfig{
		MCPServers: map[string]mcpServerConfig{
			"warrant": serverCfg,
		},
	}
}

func buildTaskPrompt(ticketID string) string {
	return fmt.Sprintf(
		"Execute the warrant ticket %s. Follow the workflow in your system prompt: "+
			"claim the ticket, start it, do the work with log_step calls, then submit with outputs. "+
			"Commit your changes to the current branch before submitting.",
		ticketID,
	)
}

// CLIWorker spawns a claude CLI subprocess directly on the host.
type CLIWorker struct {
	ClaudePath string // path to the claude binary (default: "claude")
	APIKey     string // warrant API key for MCP authentication
}

// Spawn starts a claude CLI process with the given system prompt and MCP config.
func (w *CLIWorker) Spawn(ctx context.Context, ticketID, systemPrompt, workDir, serverURL string) (*WorkerResult, error) {
	claudePath := w.ClaudePath
	if claudePath == "" {
		claudePath = "claude"
	}

	// Write temporary MCP config file for this worker.
	mcpCfgPath := filepath.Join(workDir, ".warrant-mcp-config.json")
	cfgBytes, err := json.Marshal(buildMCPConfig(serverURL, w.APIKey))
	if err != nil {
		return nil, fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(mcpCfgPath, cfgBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write mcp config: %w", err)
	}
	defer os.Remove(mcpCfgPath)

	args := []string{
		"--print",
		"--system-prompt", systemPrompt,
		buildTaskPrompt(ticketID),
		"--mcp-config", mcpCfgPath,
	}

	cmd := exec.CommandContext(ctx, claudePath, args...)
	cmd.Dir = workDir

	// Build a clean environment: inherit parent env but remove CLAUDECODE
	// (which prevents nested claude sessions).
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "CLAUDE_CODE_ENTRYPOINT=warrant-dispatch")
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		return &WorkerResult{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, nil
	}

	return &WorkerResult{
		Success: true,
		Output:  output,
	}, nil
}

// DockerWorker spawns workers inside Docker containers for full isolation.
// Each worker gets a fresh container with claude CLI, the repo cloned into its
// own workspace, and resource limits enforced. This avoids the nested-session
// problem (no CLAUDECODE env var in the container) and provides filesystem,
// network, and resource isolation following the friendslist pattern.
type DockerWorker struct {
	Image        string // Docker image (default: "warrant-worker")
	APIKey       string // warrant API key for MCP authentication
	RepoDir      string // host path to the git repository to mount
	AnthropicKey string // ANTHROPIC_API_KEY for claude CLI inside the container
	ClaudeDataDir string // host path for persistent claude config (default: ~/.warrant/claude-data)
	Memory       string // container memory limit (default: "4g")
	CPUs         string // container CPU limit (default: "2")
	PIDsLimit    string // container PID limit (default: "256")
	Firewall     bool   // enable default-deny firewall with allowlist
	AllowedHosts string // comma-separated hosts for firewall allowlist
}

// Spawn runs a claude CLI process inside a Docker container.
func (w *DockerWorker) Spawn(ctx context.Context, ticketID, systemPrompt, workDir, serverURL string) (*WorkerResult, error) {
	image := w.Image
	if image == "" {
		image = "warrant-worker"
	}

	// Write MCP config and system prompt to a temp dir on host.
	tmpDir, err := os.MkdirTemp("", "warrant-worker-*")
	if err != nil {
		return nil, fmt.Errorf("worker tmpdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// MCP config: rewrite localhost → host.docker.internal for container access.
	containerServerURL := strings.Replace(serverURL, "localhost", "host.docker.internal", 1)
	containerServerURL = strings.Replace(containerServerURL, "127.0.0.1", "host.docker.internal", 1)
	mcpCfgPath := filepath.Join(tmpDir, "mcp-config.json")
	cfgBytes, err := json.Marshal(buildMCPConfig(containerServerURL, w.APIKey))
	if err != nil {
		return nil, fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(mcpCfgPath, cfgBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write mcp config: %w", err)
	}

	// System prompt: write to file to avoid shell escaping issues.
	promptPath := filepath.Join(tmpDir, "system-prompt.txt")
	if err := os.WriteFile(promptPath, []byte(systemPrompt), 0o644); err != nil {
		return nil, fmt.Errorf("write system prompt: %w", err)
	}

	// Ensure claude data dir exists.
	claudeDataDir := w.ClaudeDataDir
	if claudeDataDir == "" {
		home, _ := os.UserHomeDir()
		claudeDataDir = filepath.Join(home, ".warrant", "claude-data")
	}
	_ = os.MkdirAll(claudeDataDir, 0o755)

	memory := w.Memory
	if memory == "" {
		memory = "4g"
	}
	cpus := w.CPUs
	if cpus == "" {
		cpus = "2"
	}
	pidsLimit := w.PIDsLimit
	if pidsLimit == "" {
		pidsLimit = "256"
	}

	// Write the task prompt to a file (shell escaping is fragile with long prompts).
	taskPromptPath := filepath.Join(tmpDir, "task-prompt.txt")
	if err := os.WriteFile(taskPromptPath, []byte(buildTaskPrompt(ticketID)), 0o644); err != nil {
		return nil, fmt.Errorf("write task prompt: %w", err)
	}

	branch := "ticket/" + ticketID
	containerName := "warrant-worker-" + sanitizeContainerName(ticketID)

	args := []string{
		"run", "--rm",
		"--name", containerName,
		// Resource limits.
		"--memory", memory,
		"--cpus", cpus,
		"--pids-limit", pidsLimit,
		// Mount repo read-only for cloning; worker gets its own copy.
		"-v", w.RepoDir + ":/repo:ro",
		// Mount MCP config, system prompt, and task prompt.
		"-v", mcpCfgPath + ":/tmp/mcp-config.json:ro",
		"-v", promptPath + ":/tmp/system-prompt.txt:ro",
		"-v", taskPromptPath + ":/tmp/task-prompt.txt:ro",
		// Persistent claude CLI state.
		"-v", claudeDataDir + ":/home/claude/.claude:delegated",
		// Environment.
		"-e", "ANTHROPIC_API_KEY=" + w.AnthropicKey,
		// Host access for MCP server.
		"--add-host", "host.docker.internal:host-gateway",
	}

	// Firewall: default-deny with allowlist.
	if w.Firewall {
		args = append(args,
			"--cap-add", "NET_ADMIN",
			"--cap-add", "NET_RAW",
			"-e", "WARRANT_FIREWALL=true",
		)
		allowedHosts := w.AllowedHosts
		if allowedHosts == "" {
			allowedHosts = "api.anthropic.com,registry.npmjs.org,github.com"
		}
		args = append(args, "-e", "WARRANT_ALLOWED_HOSTS="+allowedHosts)
	}

	// Image (must come after all -v/-e flags, before the command).
	args = append(args, image)

	// The entrypoint runs as root (for firewall), then drops to claude user.
	// All long inputs are read from mounted files to avoid shell escaping issues.
	claudeCmd := fmt.Sprintf(
		`set -e
git clone /repo /workspace 2>/dev/null
cd /workspace
git checkout -b %s 2>/dev/null || git checkout %s
claude --print --dangerously-skip-permissions --system-prompt "$(cat /tmp/system-prompt.txt)" "$(cat /tmp/task-prompt.txt)" --mcp-config /tmp/mcp-config.json`,
		branch, branch,
	)
	args = append(args, claudeCmd)

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		return &WorkerResult{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, nil
	}

	return &WorkerResult{
		Success: true,
		Output:  output,
	}, nil
}

func sanitizeContainerName(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32 // lowercase
		}
		return '-'
	}, s)
}
