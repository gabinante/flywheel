package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeDriver implements AgentDriver for Claude Code CLI.
// It preserves the existing dispatch behavior: --print mode, --system-prompt,
// --dangerously-skip-permissions, --mcp-config, OAuth token resolution, and
// the persistent .claude data directory mount.
type ClaudeDriver struct {
	// CLIPath is the path to the claude binary. Default: "claude".
	CLIPath string
}

// NewClaudeDriver creates a ClaudeDriver from config.
func NewClaudeDriver(cfg DriverConfig) *ClaudeDriver {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "claude"
	}
	return &ClaudeDriver{CLIPath: cliPath}
}

func (d *ClaudeDriver) Name() string { return "claude" }

// BuildCLIArgs returns the claude CLI invocation for host-mode execution.
func (d *ClaudeDriver) BuildCLIArgs(systemPrompt, taskMessage, mcpConfigPath string) (string, []string) {
	exe := d.CLIPath
	if exe == "" {
		exe = "claude"
	}
	args := []string{
		"--print",
		"--dangerously-skip-permissions",
		"--system-prompt", systemPrompt,
		taskMessage,
		"--mcp-config", mcpConfigPath,
	}
	return exe, args
}

// BuildDockerCmd returns the shell command to run claude inside a container.
// All long inputs are read from mounted files to avoid shell escaping issues.
func (d *ClaudeDriver) BuildDockerCmd(branch string) string {
	return fmt.Sprintf(
		`set -e
git clone /repo /workspace 2>/dev/null
cd /workspace
git checkout -b %s 2>/dev/null || git checkout %s
claude --print --dangerously-skip-permissions --system-prompt "$(cat /tmp/system-prompt.txt)" "$(cat /tmp/task-prompt.txt)" --mcp-config /tmp/mcp-config.json`,
		branch, branch,
	)
}

// DockerImage returns empty to use the default image from config.
func (d *ClaudeDriver) DockerImage() string { return "" }

// FormatPrompt passes through unchanged — Claude Code consumes markdown natively.
func (d *ClaudeDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

// Env returns Claude-specific environment configuration.
// Removes CLAUDECODE (prevents nested sessions) and ANTHROPIC_API_KEY
// (forces OAuth session reuse). Adds CLAUDE_CODE_ENTRYPOINT marker.
func (d *ClaudeDriver) Env() DriverEnv {
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

// ExtraDockerArgs returns the persistent .claude data directory mount.
func (d *ClaudeDriver) ExtraDockerArgs() []string {
	claudeDataDir := defaultClaudeDataDir()
	_ = os.MkdirAll(claudeDataDir, 0o755)
	return []string{
		"-v", claudeDataDir + ":/home/claude/.claude:delegated",
	}
}

// defaultClaudeDataDir returns the default path for persistent Claude CLI state.
func defaultClaudeDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".warrant", "claude-data")
}
