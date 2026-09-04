package dispatch

import (
	"os"
	"path/filepath"
)

// ClaudeDriver implements AgentDriver for Claude Code CLI.
// It runs `claude --print --output-format stream-json` so each session event
// (init, every assistant message, every tool result, the final result) reaches
// the dispatcher as it happens; claudeStreamParser turns those records into the
// semantic worker output stream. It keeps --system-prompt,
// --dangerously-skip-permissions, --mcp-config and OAuth login reuse.
type ClaudeDriver struct {
	model string
	// CLIPath is the path to the claude binary. Default: "claude".
	CLIPath string
}

// NewClaudeDriver creates a ClaudeDriver from config.
func NewClaudeDriver(cfg DriverConfig) *ClaudeDriver {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "claude"
	}
	return &ClaudeDriver{CLIPath: cliPath, model: cfg.Model}
}

func (d *ClaudeDriver) Name() string { return "claude" }

func (d *ClaudeDriver) Executable() string {
	if d.CLIPath == "" {
		return "claude"
	}
	return d.CLIPath
}

// BuildCLIArgs returns the claude CLI invocation for host-mode execution.
func (d *ClaudeDriver) BuildCLIArgs(systemPrompt, taskMessage string, _ mcpConnection, mcpConfigPath string) []string {
	args := []string{
		"--print",
		// Plain --print buffers everything until the run ends. stream-json emits
		// one JSON record per event as it happens, which is what lets the UI show
		// live activity; Claude Code requires --verbose alongside it.
		"--output-format", "stream-json", "--verbose",
		"--dangerously-skip-permissions",
		"--system-prompt", systemPrompt,
	}
	if d.model != "" {
		args = append(args, "--model", d.model)
	}
	// The task must precede --mcp-config: that flag takes a list of files and
	// would swallow a trailing positional prompt.
	return append(args, taskMessage, "--mcp-config", mcpConfigPath)
}

// FormatPrompt passes through unchanged — Claude Code consumes markdown natively.
func (d *ClaudeDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

// Env returns Claude-specific environment configuration.
// Removes CLAUDECODE (prevents nested sessions) and ANTHROPIC_API_KEY
// (forces OAuth session reuse). Adds CLAUDE_CODE_ENTRYPOINT marker.
func (d *ClaudeDriver) Env(_, _ string, _ mcpConnection, _ string) DriverEnv {
	return DriverEnv{
		FilterPrefixes: []string{"CLAUDECODE=", "ANTHROPIC_API_KEY="},
		Set: map[string]string{
			"CLAUDE_CODE_ENTRYPOINT": "warrant-dispatch",
		},
	}
}

// ResolveCredential returns only an explicitly configured API key. The Claude Code
// CLI manages its own claude.ai login; injecting the keychain OAuth token as
// ANTHROPIC_API_KEY made the CLI treat it as an API key and fail ("ANTHROPIC_API_KEY
// ... takes precedence over your claude.ai login").
func (d *ClaudeDriver) ResolveCredential(staticKey string) string {
	return staticKey
}

func (d *ClaudeDriver) CredentialEnvName() string { return "ANTHROPIC_API_KEY" }

func (d *ClaudeDriver) DefaultAllowedHosts() []string {
	return []string{"api.anthropic.com", "registry.npmjs.org", "github.com"}
}

// defaultClaudeDataDir returns the default path for persistent Claude CLI state.
func defaultClaudeDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".warrant", "claude-data")
}
