package dispatch

import (
	"os"
	"path/filepath"
)

// ClaudeDriver implements AgentDriver for Claude Code CLI.
// It preserves the existing dispatch behavior: --print mode, --system-prompt,
// --dangerously-skip-permissions, --mcp-config, OAuth token resolution, and
// the persistent .claude data directory mount.
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
		"--dangerously-skip-permissions",
		"--system-prompt", systemPrompt,
	}
	if d.model != "" {
		args = append(args, "--model", d.model)
	}
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

// ResolveCredential attempts to read the Claude Code OAuth access token from
// the macOS keychain. Falls back to the static API key.
func (d *ClaudeDriver) ResolveCredential(staticKey string) string {
	if token := readClaudeOAuthToken(); token != "" {
		return token
	}
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
