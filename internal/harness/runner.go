// Package harness runs the local coding agents — Codex and Claude Code — as
// headless, single-turn processes and normalizes their output: final text,
// optional schema-conforming JSON, the harness's own session id, and token usage.
// Dispatch workers and code review both build on it; the harness for a phase is
// configuration, never a vendor API.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gabinante/flywheel/internal/process"
	"github.com/gabinante/flywheel/internal/runlimit"
	"github.com/gabinante/flywheel/internal/runstatus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Kind names a harness.
type Kind string

const (
	Codex      Kind = "codex"
	ClaudeCode Kind = "claude_code"
)

// ParseKind accepts the config spellings used across Flywheel.
func ParseKind(s string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "codex":
		return Codex, nil
	case "claude", "claude_code", "claude-code", "claudecode":
		return ClaudeCode, nil
	}
	return "", fmt.Errorf("unknown harness %q (want codex or claude)", s)
}

// Sandbox levels.
const (
	SandboxReadOnly       = "read-only"
	SandboxWorkspaceWrite = "workspace-write"
	// SandboxFull lets the agent act on the operator's behalf (network, gh, pushes): Codex
	// danger-full-access, Claude --dangerously-skip-permissions. Used for conversations the
	// operator drives interactively.
	SandboxFull = "danger-full-access"
)

// MCPServer is an MCP endpoint to expose to the agent.
type MCPServer struct {
	Name    string
	URL     string
	Headers map[string]string
}

// Spec describes one headless run.
type Spec struct {
	Harness      Kind
	Model        string
	Effort       string
	WorkDir      string
	SystemPrompt string
	Prompt       string
	OutputSchema json.RawMessage // when set, the final message must conform and is returned in Result.Structured
	Sandbox      string          // SandboxReadOnly (default) or SandboxWorkspaceWrite
	Timeout      time.Duration   // default 30m
	MCP          []MCPServer     // optional MCP servers (Codex: replaces the user's configured servers for this run)
	Binary       string          // override executable
	Env          []string        // extra KEY=VALUE entries
	Resume       string          // resume this harness session (Codex thread id / Claude session id) instead of starting fresh
}

// Result is the normalized outcome.
type Result struct {
	Harness           Kind
	ExternalSessionID string // Codex thread id / Claude session id
	Output            string // final agent message
	Structured        json.RawMessage
	TokensIn          int64
	TokensOut         int64
	CostUSD           float64
	Duration          time.Duration
	ExitCode          int
	Stderr            string
	Command           string
	Model             string
}

// Runner executes Specs.
type Runner interface {
	Run(ctx context.Context, spec Spec) (*Result, error)
}

// Config holds binary locations.
type Config struct {
	CodexBin  string // default "codex"
	ClaudeBin string // default "claude"
}

// CLIRunner runs harnesses as subprocesses.
type CLIRunner struct {
	mu  sync.RWMutex
	cfg Config
}

// Reconfigure swaps the binaries at runtime (operator settings).
func (r *CLIRunner) Reconfigure(cfg Config) {
	if cfg.CodexBin == "" {
		cfg.CodexBin = "codex"
	}
	if cfg.ClaudeBin == "" {
		cfg.ClaudeBin = "claude"
	}
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
}

func (r *CLIRunner) conf() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

// New returns a CLIRunner.
func New(cfg Config) *CLIRunner {
	if cfg.CodexBin == "" {
		cfg.CodexBin = "codex"
	}
	if cfg.ClaudeBin == "" {
		cfg.ClaudeBin = "claude"
	}
	return &CLIRunner{cfg: cfg}
}

// Run executes the spec.
func (r *CLIRunner) Run(ctx context.Context, spec Spec) (*Result, error) {
	info := runstatus.Info(ctx)
	info.Harness = string(spec.Harness)
	info.Model = spec.Model
	info.WorkDir = spec.WorkDir
	info.Prompt = spec.Prompt
	if info.Kind == "" {
		info.Kind = "harness"
		info.Title = "Harness run"
	}
	ctx, finish := runstatus.Default.Begin(ctx, info)
	defer finish()
	release, err := runlimit.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	runstatus.Session(ctx, spec.Resume)

	if spec.Timeout <= 0 {
		spec.Timeout = 30 * time.Minute
	}
	if spec.Sandbox == "" {
		spec.Sandbox = SandboxReadOnly
	}
	switch spec.Harness {
	case Codex:
		return r.runCodex(ctx, spec)
	case ClaudeCode:
		return r.runClaude(ctx, spec)
	}
	return nil, fmt.Errorf("harness: unsupported harness %q", spec.Harness)
}

// fullPrompt prepends the system prompt for harnesses without a system-prompt flag.
func fullPrompt(spec Spec) string {
	if strings.TrimSpace(spec.SystemPrompt) == "" {
		return spec.Prompt
	}
	return "# Instructions\n\n" + strings.TrimSpace(spec.SystemPrompt) + "\n\n# Task\n\n" + spec.Prompt
}

func tomlString(s string) string {
	b, _ := json.Marshal(s) // JSON string escaping is valid TOML basic-string escaping for our inputs
	return string(b)
}

// ---- Codex ---------------------------------------------------------------

type codexEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Item     *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	Usage *struct {
		InputTokens       int64 `json:"input_tokens"`
		CachedInputTokens int64 `json:"cached_input_tokens"`
		OutputTokens      int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r *CLIRunner) runCodex(ctx context.Context, spec Spec) (*Result, error) {
	bin := spec.Binary
	if bin == "" {
		bin = r.conf().CodexBin
	}
	tmp, err := os.MkdirTemp("", "flywheel-codex-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	lastPath := filepath.Join(tmp, "last.txt")
	args := []string{"exec"}
	if spec.Resume != "" {
		// `codex exec resume` accepts -c/-m/--json/-o/--output-schema but not -s or -C:
		// the sandbox goes through config and the working directory through cmd.Dir.
		args = append(args, "resume", spec.Resume, "--json", "-c", "sandbox_mode="+tomlString(spec.Sandbox), "-c", "approval_policy=never", "-o", lastPath, "--skip-git-repo-check")
	} else {
		args = append(args, "--json", "-s", spec.Sandbox, "-c", "approval_policy=never", "-o", lastPath)
		if spec.WorkDir != "" {
			args = append(args, "-C", spec.WorkDir)
		}
	}
	if spec.Model != "" {
		args = append(args, "-m", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+tomlString(spec.Effort))
	}
	// Do not load the operator's interactive MCP servers into headless runs; expose only what the spec asks for.
	args = append(args, "-c", "mcp_servers={}")
	for _, m := range spec.MCP {
		args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.url=%s", m.Name, tomlString(m.URL)))
		if len(m.Headers) > 0 {
			var parts []string
			for k, v := range m.Headers {
				parts = append(parts, tomlString(k)+" = "+tomlString(v))
			}
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.http_headers={ %s }", m.Name, strings.Join(parts, ", ")))
		}
	}
	if len(spec.OutputSchema) > 0 {
		schemaPath := filepath.Join(tmp, "schema.json")
		if err := os.WriteFile(schemaPath, spec.OutputSchema, 0o600); err != nil {
			return nil, err
		}
		args = append(args, "--output-schema", schemaPath)
	}
	args = append(args, "-") // prompt on stdin

	cctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	process.Configure(cmd)
	cmd.Stdin = strings.NewReader(fullPrompt(spec))
	cmd.Env = append(os.Environ(), spec.Env...)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	var stderr bytes.Buffer
	cmd.Stderr = runstatus.WatchOutput(ctx, &stderr)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("harness: start codex: %w", err)
	}
	runstatus.Started(cctx, cmd.Process.Pid)
	res := &Result{Harness: Codex, Command: bin + " " + strings.Join(redactArgs(args), " "), Model: spec.Model, ExternalSessionID: spec.Resume}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var lastErr string
	completed := false
	for sc.Scan() {
		runstatus.ParseOutput(ctx, sc.Bytes())
		var ev codexEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			res.ExternalSessionID = ev.ThreadID
			runstatus.Session(ctx, ev.ThreadID)
		case "item.completed":
			if ev.Item != nil && ev.Item.Type == "agent_message" {
				res.Output = ev.Item.Text
			}
		case "turn.completed":
			completed = true
			if ev.Usage != nil {
				res.TokensIn += ev.Usage.InputTokens
				res.TokensOut += ev.Usage.OutputTokens
			}
		case "error", "turn.failed":
			lastErr = ev.Message
			if lastErr == "" {
				lastErr = ev.Type
			}
			if ev.Error != nil {
				lastErr = ev.Error.Message
			}
		}
	}
	scanErr := sc.Err()
	if scanErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	runstatus.Exited(ctx)
	if waitErr == nil {
		waitErr = scanErr
	}
	res.Duration = time.Since(start)
	res.Stderr = truncate(stderr.String(), 4000)
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
	}
	if b, err := os.ReadFile(lastPath); err == nil && len(bytes.TrimSpace(b)) > 0 {
		res.Output = strings.TrimSpace(string(b))
	}
	if len(spec.OutputSchema) > 0 && json.Valid([]byte(res.Output)) {
		res.Structured = json.RawMessage(res.Output)
	}
	if waitErr != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("harness: codex timed out after %s", spec.Timeout)
		}
		if lastErr == "" {
			lastErr = firstLine(stderr.String())
		}
		return res, fmt.Errorf("harness: codex exited %d: %s", res.ExitCode, lastErr)
	}
	if lastErr != "" {
		return res, fmt.Errorf("harness: codex: %s", lastErr)
	}
	if !completed {
		return res, errors.New("harness: codex exited without a completed turn")
	}
	return res, nil
}

// ---- Claude Code ---------------------------------------------------------

type claudeResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	SessionID        string          `json:"session_id"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	DurationMS       int64           `json:"duration_ms"`
	Usage            struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (r *CLIRunner) runClaude(ctx context.Context, spec Spec) (*Result, error) {
	bin := spec.Binary
	if bin == "" {
		bin = r.conf().ClaudeBin
	}
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
	if spec.Resume != "" {
		args = append(args, "--resume", spec.Resume)
	}
	switch spec.Sandbox {
	case SandboxFull:
		args = append(args, "--dangerously-skip-permissions")
	case SandboxWorkspaceWrite:
		args = append(args, "--permission-mode", "acceptEdits")
	default:
		args = append(args, "--permission-mode", "plan")
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if strings.TrimSpace(spec.SystemPrompt) != "" {
		args = append(args, "--append-system-prompt", spec.SystemPrompt)
	}
	if len(spec.OutputSchema) > 0 {
		args = append(args, "--json-schema", string(spec.OutputSchema))
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	args = append(args, "--strict-mcp-config")
	var mcpPath string
	if len(spec.MCP) > 0 {
		tmp, err := os.MkdirTemp("", "flywheel-claude-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		servers := map[string]any{}
		var allowed []string
		for _, m := range spec.MCP {
			servers[m.Name] = map[string]any{"type": "http", "url": m.URL, "headers": m.Headers}
			allowed = append(allowed, "mcp__"+m.Name+"__*")
		}
		b, _ := json.Marshal(map[string]any{"mcpServers": servers})
		mcpPath = filepath.Join(tmp, "mcp.json")
		if err := os.WriteFile(mcpPath, b, 0o600); err != nil {
			return nil, err
		}
		// The prompt has to precede these flags: both take a list of values and
		// would swallow a trailing positional prompt, after which the CLI exits
		// with "Input must be provided either through stdin or as a prompt argument".
		args = append(args, spec.Prompt, "--mcp-config", mcpPath, "--allowedTools", strings.Join(allowed, ","))
	} else {
		args = append(args, spec.Prompt)
	}

	cctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	process.Configure(cmd)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	env := append(os.Environ(), spec.Env...)
	// Prevent nested-session detection and force the harness's own login session.
	env = filterEnv(env, "CLAUDECODE")
	env = append(env, "CLAUDE_CODE_ENTRYPOINT=flywheel-dispatch")
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = runstatus.WatchOutput(ctx, &stderr)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("harness: start claude: %w", err)
	}
	runstatus.Started(cctx, cmd.Process.Pid)
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 64<<20)
	var cr claudeResult
	complete := false
	nativeID := spec.Resume
	for sc.Scan() {
		runstatus.ParseOutput(ctx, sc.Bytes())
		var event claudeResult
		if json.Unmarshal(sc.Bytes(), &event) == nil {
			if event.SessionID != "" {
				nativeID = event.SessionID
			}
			if event.Type == "result" {
				cr, complete = event, true
			}
		}
	}
	if sc.Err() != nil {
		cancel()
	}
	runErr := cmd.Wait()
	runstatus.Exited(ctx)
	if runErr == nil {
		runErr = sc.Err()
	}
	res := &Result{Harness: ClaudeCode, ExternalSessionID: nativeID, Command: bin + " " + strings.Join(redactArgs(args), " "), Model: spec.Model, Duration: time.Since(start), Stderr: truncate(stderr.String(), 4000)}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
	}
	if complete {
		runstatus.Session(ctx, cr.SessionID)
		res.Output = cr.Result
		res.TokensIn = cr.Usage.InputTokens + cr.Usage.CacheReadInputTokens + cr.Usage.CacheCreationInputTokens
		res.TokensOut = cr.Usage.OutputTokens
		res.CostUSD = cr.TotalCostUSD
		if len(cr.StructuredOutput) > 0 && string(cr.StructuredOutput) != "null" {
			res.Structured = cr.StructuredOutput
		} else if len(spec.OutputSchema) > 0 && json.Valid([]byte(cr.Result)) {
			res.Structured = json.RawMessage(cr.Result)
		}
		if cr.IsError {
			return res, fmt.Errorf("harness: claude reported an error: %s", truncate(cr.Result, 300))
		}
	}
	if runErr != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("harness: claude timed out after %s", spec.Timeout)
		}
		return res, fmt.Errorf("harness: claude exited %d: %s", res.ExitCode, firstLine(stderr.String()))
	}
	if !complete {
		return res, errors.New("harness: claude exited without a final result")
	}
	return res, nil
}

func filterEnv(env []string, prefix string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// redactArgs hides the (long) prompt and schema in the recorded command line.
func redactArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.Contains(a, "http_headers") || strings.Contains(a, "X-API-Key") {
			out[i] = "[redacted credentials]"
		} else if len(a) > 120 {
			out[i] = a[:60] + "…"
		} else {
			out[i] = a
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncate(s, 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
