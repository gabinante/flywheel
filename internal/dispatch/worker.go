package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// readClaudeOAuthToken reads the Claude Code OAuth access token from the macOS
// keychain. This lets dispatch workers use the operator's Claude Code
// subscription instead of burning API credits via ANTHROPIC_API_KEY.
// Returns empty string on any failure (non-macOS, no keychain entry, etc.).
func readClaudeOAuthToken() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return ""
	}
	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
			ExpiresAt   int64  `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(out, &creds); err != nil {
		return ""
	}
	return creds.ClaudeAiOauth.AccessToken
}

// WorkerResult is the outcome of a worker execution.
type WorkerResult struct {
	Success bool
	Output  string
	Error   string
}

// Worker spawns an agent session for a ticket.
type Worker interface {
	Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error)
}

func buildTaskPrompt(ticketID, projectID string) string {
	return buildTypedTaskPrompt("", ticketID, projectID)
}

// buildTypedTaskPrompt returns the user-turn prompt tailored to the worker type.
func buildTypedTaskPrompt(wt WorkerType, ticketID, projectID string) string {
	switch wt {
	case WorkerTypePlanner:
		return fmt.Sprintf(
			"Plan Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, investigate the codebase, produce a structured plan, "+
				"call log_step for each finding, and submit_ticket with the plan in outputs. "+
				"You MUST use the Flywheel MCP tools — do not skip any steps.",
			ticketID, projectID,
		)
	case WorkerTypeExecutor:
		return fmt.Sprintf(
			"Execute Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, do the implementation work (call log_step for each step), "+
				"commit your changes to the current branch, and call submit_ticket with outputs. "+
				"You MUST use the Flywheel MCP tools — do not skip any steps.",
			ticketID, projectID,
		)
	case WorkerTypeValidator:
		return fmt.Sprintf(
			"Review Flywheel ticket %s. "+
				"Read the PR diff, check code quality and correctness against the ticket objectives, "+
				"run tests if applicable, then either approve_ticket or reject_ticket with notes. "+
				"You MUST use the Flywheel MCP tools to approve or reject.",
			ticketID,
		)
	case WorkerTypeDeployer:
		return fmt.Sprintf(
			"Deploy Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, execute the deployment plan, verify health, "+
				"call log_step for each action, and submit_ticket with deployment status. "+
				"You MUST use the Flywheel MCP tools — do not skip any steps.",
			ticketID, projectID,
		)
	case WorkerTypeInvestigator:
		return fmt.Sprintf(
			"Investigate for Flywheel ticket %s. "+
				"Read code, search for patterns, and report findings via log_step. "+
				"You are read-only — do not modify any files or state.",
			ticketID,
		)
	default:
		return fmt.Sprintf(
			"Execute Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, do the implementation work (call log_step for each step), "+
				"commit your changes to the current branch, and call submit_ticket with outputs. "+
				"You MUST use the Flywheel MCP tools — do not skip any steps.",
			ticketID, projectID,
		)
	}
}

// CLIWorker spawns an agent subprocess directly on the host.
// It delegates agent-specific behavior (CLI flags, env vars) to the AgentDriver.
type CLIWorker struct {
	Driver AgentDriver // agent-specific behavior
	APIKey string      // Flywheel API key for MCP authentication
}

// Spawn starts an agent process with the given system prompt and MCP config.
func (w *CLIWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	// Apply driver's prompt formatting.
	systemPrompt = w.Driver.FormatPrompt(systemPrompt)

	mcpConn := buildMCPConnection(serverURL, w.APIKey)

	// Write temporary MCP config file for this worker.
	mcpCfgPath := filepath.Join(workDir, ".flywheel-mcp-config.json")
	cfgBytes, err := json.Marshal(mcpConn.SSEConfig())
	if err != nil {
		return nil, fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(mcpCfgPath, cfgBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write mcp config: %w", err)
	}
	defer os.Remove(mcpCfgPath)

	// Get executable and arguments from the driver.
	exe := w.Driver.Executable()
	args := w.Driver.BuildCLIArgs(systemPrompt, taskMessage, mcpConn, mcpCfgPath)

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = workDir

	// Build environment: start with parent env, apply driver's filters and additions.
	driverEnv := w.Driver.Env(systemPrompt, taskMessage, mcpConn, mcpCfgPath)
	var env []string
	for _, e := range os.Environ() {
		filtered := false
		for _, prefix := range driverEnv.FilterPrefixes {
			if strings.HasPrefix(e, prefix) {
				filtered = true
				break
			}
		}
		if !filtered {
			env = append(env, e)
		}
	}
	for k, v := range driverEnv.Set {
		env = append(env, k+"="+v)
	}

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
// Each worker gets a fresh container with the agent, the repo cloned into its
// own workspace, and resource limits enforced.
type DockerWorker struct {
	Driver       AgentDriver // agent-specific behavior
	Image        string      // Docker image (overrides driver's DockerImage if set)
	APIKey       string      // Flywheel API key for MCP authentication
	RepoDir      string      // host path to the git repository to mount
	AgentAPIKey  string      // static API key for the selected agent inside the container
	Memory       string      // container memory limit (default: "4g")
	CPUs         string      // container CPU limit (default: "2")
	PIDsLimit    string      // container PID limit (default: "256")
	Firewall     bool        // enable default-deny firewall with allowlist
	AllowedHosts string      // comma-separated hosts for firewall allowlist
}

// Spawn runs an agent process inside a Docker container.
func (w *DockerWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	// Apply driver's prompt formatting.
	systemPrompt = w.Driver.FormatPrompt(systemPrompt)

	// Determine image: driver preference, then worker config, then default.
	image := w.Driver.DockerImage()
	if image == "" {
		image = w.Image
	}
	if image == "" {
		image = "flywheel-worker"
	}

	// Write MCP config and prompts to a temp dir on host.
	tmpDir, err := os.MkdirTemp("", "flywheel-worker-*")
	if err != nil {
		return nil, fmt.Errorf("worker tmpdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// MCP config: rewrite localhost → host.docker.internal for container access.
	containerServerURL := strings.Replace(serverURL, "localhost", "host.docker.internal", 1)
	containerServerURL = strings.Replace(containerServerURL, "127.0.0.1", "host.docker.internal", 1)
	mcpConn := buildMCPConnection(containerServerURL, w.APIKey)
	mcpCfgPath := filepath.Join(tmpDir, "mcp-config.json")
	cfgBytes, err := json.Marshal(mcpConn.SSEConfig())
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

	// Task prompt: write to file.
	taskPromptPath := filepath.Join(tmpDir, "task-prompt.txt")
	if err := os.WriteFile(taskPromptPath, []byte(taskMessage), 0o644); err != nil {
		return nil, fmt.Errorf("write task prompt: %w", err)
	}

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

	// Resolve the API credential via the driver (e.g., OAuth token for Claude).
	credential := w.Driver.ResolveCredential(w.AgentAPIKey)
	if credential != "" {
		if credential != w.AgentAPIKey {
			log.Printf("dispatch: using dynamic credential for worker %s", ticketID)
		} else {
			log.Printf("dispatch: using static credential for worker %s", ticketID)
		}
	}

	branch := "ticket/" + ticketID
	containerName := "flywheel-worker-" + sanitizeContainerName(ticketID)

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
		// Host access for MCP server.
		"--add-host", "host.docker.internal:host-gateway",
	}

	// Add credential as environment variable if available.
	if credential != "" {
		if envName := w.Driver.CredentialEnvName(); envName != "" {
			args = append(args, "-e", envName+"="+credential)
		}
	}

	// Add driver-specific environment variables.
	driverEnv := w.Driver.Env(systemPrompt, taskMessage, mcpConn, "/tmp/mcp-config.json")
	for k, v := range driverEnv.Set {
		args = append(args, "-e", k+"="+v)
	}

	// Add driver-specific Docker arguments (e.g., volume mounts).
	args = append(args, w.Driver.ExtraDockerArgs()...)

	// Firewall: default-deny with allowlist.
	if w.Firewall {
		args = append(args,
			"--cap-add", "NET_ADMIN",
			"--cap-add", "NET_RAW",
			"-e", "FLYWHEEL_FIREWALL=true",
		)
		allowedHosts := w.AllowedHosts
		if allowedHosts == "" {
			if defaults := w.Driver.DefaultAllowedHosts(); len(defaults) > 0 {
				allowedHosts = strings.Join(defaults, ",")
			} else {
				allowedHosts = "registry.npmjs.org,github.com"
			}
		}
		args = append(args, "-e", "FLYWHEEL_ALLOWED_HOSTS="+allowedHosts)
	}

	// Image (must come after all -v/-e flags, before the command).
	args = append(args, image)

	// Get the agent-specific command from the driver.
	agentCmd := w.Driver.BuildDockerCmd(branch, mcpConn)
	args = append(args, agentCmd)

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
