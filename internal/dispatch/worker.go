package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// WorkerOutputHandler receives incremental stdout/stderr lines from a worker process.
type WorkerOutputHandler func(stream, text string)

// StreamableWorker is a Worker that can emit incremental output while it runs.
type StreamableWorker interface {
	Worker
	SpawnStream(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string, onOutput WorkerOutputHandler) (*WorkerResult, error)
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
	case WorkerTypeDecomposer:
		return fmt.Sprintf(
			"Decompose Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, analyze the ticket scope, and decide whether to decompose. "+
				"If well-scoped, submit immediately. If not, create subtickets via create_ticket "+
				"with depends_on ordering, then submit_ticket listing the created subticket IDs. "+
				"You MUST use the Flywheel MCP tools — do not skip any steps.",
			ticketID, projectID,
		)
	case WorkerTypeOperator:
		return fmt.Sprintf(
			"Operate on Flywheel ticket %s. "+
				"FIRST: call the claim_ticket MCP tool with project_id \"%s\". "+
				"This returns ticket_id and lease_token — use these for all subsequent MCP calls. "+
				"THEN: call start_ticket, analyze the situation, query state, log findings via log_step, "+
				"and submit_ticket with your analysis and recommendations. "+
				"Do NOT write code or modify files.",
			ticketID, projectID,
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
	Driver      AgentDriver // agent-specific behavior
	APIKey      string      // Flywheel API key for MCP authentication
	AgentAPIKey string      // provider credential for the selected worker
}

// Spawn starts an agent process with the given system prompt and MCP config.
func (w *CLIWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	return w.SpawnStream(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL, nil)
}

// SpawnStream starts an agent process and emits incremental stdout/stderr lines.
func (w *CLIWorker) SpawnStream(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string, onOutput WorkerOutputHandler) (*WorkerResult, error) {
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
	if envName := w.Driver.CredentialEnvName(); envName != "" {
		if credential := w.Driver.ResolveCredential(w.AgentAPIKey); credential != "" {
			env = append(env, envName+"="+credential)
		}
	}

	cmd.Env = env

	output, err := runCommandStreaming(cmd, onOutput)

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

type workerOutputEvent struct {
	stream string
	text   string
}

func runCommandStreaming(cmd *exec.Cmd, onOutput WorkerOutputHandler) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	events := make(chan workerOutputEvent, 128)
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go streamPipeOutput("stdout", stdout, events, errCh, &wg)
	go streamPipeOutput("stderr", stderr, events, errCh, &wg)
	go func() {
		wg.Wait()
		close(events)
		close(errCh)
	}()

	var output strings.Builder
	for event := range events {
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(event.text)
		if onOutput != nil {
			onOutput(event.stream, event.text)
		}
	}

	waitErr := cmd.Wait()
	for streamErr := range errCh {
		if streamErr != nil && waitErr == nil {
			waitErr = streamErr
		}
	}

	return strings.TrimSpace(output.String()), waitErr
}

func streamPipeOutput(stream string, reader io.Reader, events chan<- workerOutputEvent, errCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		events <- workerOutputEvent{
			stream: stream,
			text:   strings.TrimRight(scanner.Text(), "\r"),
		}
	}
	errCh <- scanner.Err()
}
