package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/process"
	"github.com/gabinante/flywheel/internal/runlimit"
	"github.com/gabinante/flywheel/internal/runstatus"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
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
	SessionID          string
	StartedAt, EndedAt time.Time
	CostUSD            float64
	Success            bool
	Output             string
	Error              string
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
	return strings.ReplaceAll(buildTypedTaskPromptBase(wt, ticketID, projectID), "call the claim_ticket MCP tool", fmt.Sprintf("call the claim_ticket MCP tool with ticket_id %q (claim only this exact ticket),", ticketID))
}
func buildTypedTaskPromptBase(wt WorkerType, ticketID, projectID string) string {
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
				"FIRST claim_ticket for this exact ticket, then start_ticket. Submit your findings with submit_ticket when finished. "+
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
	info := runstatus.Info(ctx)
	info.ProjectID = projectID
	info.TicketID = ticketID
	info.Harness = w.Driver.Name()
	info.WorkDir = workDir
	info.Prompt = taskMessage
	if grant, ok := auth.RunFromContext(ctx); ok {
		info.Kind = grant.Role
	}
	if info.Kind == "orchestrator" {
		info.TicketID = ""
		info.Title = "Project planning"
	}
	if info.Kind == "" {
		info.Kind = "dispatch"
	}
	ctx, finish := runstatus.Default.Begin(ctx, info)
	defer finish()
	release, err := runlimit.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	driver := w.Driver
	if grant, ok := auth.RunFromContext(ctx); ok && (grant.Role == "orchestrator" || grant.Role == "planner" || grant.Role == "investigator" || grant.Role == "operator") {
		switch d := driver.(type) {
		case *ClaudeDriver:
			copy := *d
			copy.readOnly = true
			driver = &copy
		case *CodexDriver:
			copy := *d
			copy.readOnly = true
			driver = &copy
		}
	}
	// Apply driver's prompt formatting.
	systemPrompt = driver.FormatPrompt(systemPrompt)

	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	grant, ok := auth.RunFromContext(ctx)
	if !ok {
		grant = auth.RunGrant{Role: "executor"}
	}
	grant.ParentKey = w.APIKey
	grant.TicketID = ticketID
	grant.ProjectID = projectID
	runKey, revoke, err := auth.IssueRun(grant)
	if err != nil {
		return nil, err
	}
	defer revoke()
	mcpConn := buildMCPConnection(serverURL, runKey)

	// Write temporary MCP config file for this worker.
	configFile, err := os.CreateTemp("", "flywheel-mcp-*.json")
	if err != nil {
		return nil, err
	}
	mcpCfgPath := configFile.Name()
	defer os.Remove(mcpCfgPath)
	if err := configFile.Close(); err != nil {
		return nil, err
	}
	cfgBytes, err := json.Marshal(mcpConn.SSEConfig())
	if err != nil {
		return nil, fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(mcpCfgPath, cfgBytes, 0o600); err != nil {
		return nil, fmt.Errorf("write mcp config: %w", err)
	}
	defer os.Remove(mcpCfgPath)

	// Get executable and arguments from the driver.
	exe := driver.Executable()
	args := driver.BuildCLIArgs(systemPrompt, taskMessage, mcpConn, mcpCfgPath)

	cmd := exec.CommandContext(ctx, exe, args...)
	process.Configure(cmd)
	cmd.Dir = workDir

	// Build environment: start with parent env, apply driver's filters and additions.
	driverEnv := driver.Env(systemPrompt, taskMessage, mcpConn, mcpCfgPath)
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
	if envName := driver.CredentialEnvName(); envName != "" {
		if credential := driver.ResolveCredential(w.AgentAPIKey); credential != "" {
			env = append(env, envName+"="+credential)
		}
	}

	cmd.Env = env

	// Drivers with structured output get a parser so the trace sees semantic
	// events and the result is the agent's answer rather than a JSON transcript.
	var parser OutputParser
	if parsing, ok := driver.(OutputParsingDriver); ok {
		parser = parsing.NewOutputParser()
	}

	started := time.Now()
	if parser != nil {
		parser = &liveSessionParser{OutputParser: parser, ctx: ctx}
	}
	output, parsed, err := runCommandStreamingContext(ctx, cmd, onOutput, parser)
	result := &WorkerResult{Output: output, Success: err == nil, StartedAt: started, EndedAt: time.Now()}
	if parsed != nil {
		result.SessionID = parsed.SessionID
		runstatus.Session(ctx, parsed.SessionID)
		result.CostUSD = parsed.CostUSD
	}
	if parsed != nil {
		slog.Info("dispatch: worker run reported result", "driver", driver.Name(), "session", parsed.SessionID, "turns", parsed.NumTurns, "cost_usd", parsed.CostUSD, "is_error", parsed.IsError)
	}
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	if parsed != nil && parsed.IsError {
		result.Success = false
		result.Error = parsed.Error
		return result, nil
	}

	return result, nil
}

type workerOutputEvent struct {
	stream string
	text   string
}

// runCommandStreaming runs cmd and forwards each output line to onOutput as it
// arrives. With a parser, structured stdout lines become semantic events and the
// parser's final result becomes the returned output; unstructured lines and
// stderr pass through unchanged. Without one, the output is the raw transcript.
func runCommandStreaming(cmd *exec.Cmd, onOutput WorkerOutputHandler, parser OutputParser) (string, *ParsedResult, error) {
	return runCommandStreamingContext(context.Background(), cmd, onOutput, parser)
}

func runCommandStreamingContext(ctx context.Context, cmd *exec.Cmd, onOutput WorkerOutputHandler, parser OutputParser) (string, *ParsedResult, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, err
	}
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	runstatus.Started(ctx, cmd.Process.Pid)

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

	emit := func(stream, text string) {
		if onOutput != nil {
			onOutput(stream, text)
		}
	}
	var raw strings.Builder
	for event := range events {
		if event.stream == "stderr" || parser == nil {
			runstatus.Output(ctx)
		}
		if parser != nil && event.stream == "stdout" {
			if parsed, ok := parser.ParseLine(event.text); ok {
				for _, pe := range parsed {
					emit(pe.Stream, pe.Text)
				}
				continue
			}
		}
		if raw.Len() > 0 {
			raw.WriteByte('\n')
		}
		raw.WriteString(event.text)
		emit(event.stream, event.text)
	}

	waitErr := cmd.Wait()
	runstatus.Exited(ctx)
	for streamErr := range errCh {
		if streamErr != nil && waitErr == nil {
			waitErr = streamErr
		}
	}

	output := strings.TrimSpace(raw.String())
	if parser == nil {
		return output, nil, waitErr
	}
	result := parser.Result()
	answer := ""
	if result != nil {
		answer = strings.TrimSpace(result.Output)
	}
	if answer == "" {
		answer = strings.TrimSpace(parser.Transcript())
	}
	if waitErr == nil && (result == nil || !result.IsError) {
		if answer != "" {
			return answer, result, nil
		}
		return output, result, nil
	}
	// On failure keep the harness's own words and whatever else it printed
	// (auth errors arrive as plain text) so failover matching and the run log
	// see both.
	return joinNonEmpty(answer, output), result, waitErr
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
