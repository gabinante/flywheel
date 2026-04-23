package dispatch

import (
	"fmt"
	"strings"
)

// CodexDriver implements AgentDriver for the OpenAI Codex CLI.
// It uses `codex exec` in host mode and connects to Flywheel over the
// streamable HTTP MCP endpoint at /mcp.
type CodexDriver struct {
	CLIPath string
}

func NewCodexDriver(cfg DriverConfig) *CodexDriver {
	cliPath := cfg.CLIPath
	if cliPath == "" {
		cliPath = "codex"
	}
	return &CodexDriver{CLIPath: cliPath}
}

func (d *CodexDriver) Name() string { return "codex" }

func (d *CodexDriver) Executable() string {
	if d.CLIPath == "" {
		return "codex"
	}
	return d.CLIPath
}

func (d *CodexDriver) BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, _ string) []string {
	args := []string{
		"exec",
		"--ask-for-approval", "never",
		"--sandbox", "workspace-write",
		"--color", "never",
		"-c", "sandbox_workspace_write.network_access=true",
	}
	args = append(args, d.mcpOverrides(mcp)...)
	args = append(args, buildCodexPrompt(systemPrompt, taskMessage))
	return args
}

func (d *CodexDriver) BuildDockerCmd(branch string, mcp mcpConnection) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("git clone /repo /workspace 2>/dev/null\n")
	b.WriteString("cd /workspace\n")
	b.WriteString(fmt.Sprintf("git checkout -b %s 2>/dev/null || git checkout %s\n", branch, branch))
	if apiKey := mcp.Headers["X-API-Key"]; apiKey != "" {
		b.WriteString("export FLYWHEEL_MCP_API_KEY=" + shellQuote(apiKey) + "\n")
	}
	b.WriteString("{ printf '## System Instructions\\n\\n'; cat /tmp/system-prompt.txt; ")
	b.WriteString("printf '\\n\\n## Task\\n\\n'; cat /tmp/task-prompt.txt; } | ")
	b.WriteString("codex exec --ask-for-approval never --dangerously-bypass-approvals-and-sandbox --color never ")
	for _, arg := range d.mcpOverrides(mcp) {
		b.WriteString(shellQuote(arg))
		b.WriteString(" ")
	}
	b.WriteString("-\n")
	return b.String()
}

func (d *CodexDriver) DockerImage() string { return "" }

func (d *CodexDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

func (d *CodexDriver) Env(_, _ string, _ mcpConnection, _ string) DriverEnv {
	return DriverEnv{
		FilterPrefixes: nil,
		Set: map[string]string{
			"WARRANT_DISPATCH":  "true",
			"FLYWHEEL_DISPATCH": "true",
		},
	}
}

func (d *CodexDriver) ResolveCredential(staticKey string) string { return staticKey }

func (d *CodexDriver) CredentialEnvName() string { return "OPENAI_API_KEY" }

func (d *CodexDriver) DefaultAllowedHosts() []string {
	return []string{"api.openai.com", "registry.npmjs.org", "github.com"}
}

func (d *CodexDriver) ExtraDockerArgs() []string { return nil }

func (d *CodexDriver) mcpOverrides(mcp mcpConnection) []string {
	args := []string{
		"-c", fmt.Sprintf(`mcp_servers.%s.url=%q`, mcp.Name, mcp.HTTPURL),
	}
	if apiKey := mcp.Headers["X-API-Key"]; apiKey != "" {
		args = append(args,
			"-c", fmt.Sprintf(`mcp_servers.%s.env_http_headers={"X-API-Key"="FLYWHEEL_MCP_API_KEY"}`, mcp.Name),
		)
	}
	return args
}

func buildCodexPrompt(systemPrompt, taskMessage string) string {
	return strings.TrimSpace(fmt.Sprintf("## System Instructions\n\n%s\n\n## Task\n\n%s", systemPrompt, taskMessage))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
