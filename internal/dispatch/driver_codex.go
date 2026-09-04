package dispatch

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CodexDriver runs OpenAI Codex CLI as a dispatch harness.
//
// It invokes `codex exec --json` non-interactively in the ticket worktree with a
// workspace-write sandbox, wires the Flywheel MCP server through -c overrides
// (replacing the operator's interactive MCP servers for the run), and delivers the
// system prompt as an instructions preamble because Codex has no system-prompt flag.
type CodexDriver struct {
	cliPath   string
	extraArgs []string
	model     string
	effort    string
}

// NewCodexDriver creates a Codex driver.
func NewCodexDriver(cfg DriverConfig) *CodexDriver {
	return &CodexDriver{cliPath: cfg.CLIPath, extraArgs: cfg.ExtraArgs, model: cfg.Model, effort: cfg.ReasoningEffort}
}

// Name returns "codex".
func (d *CodexDriver) Name() string { return "codex" }

// Executable returns the codex binary path.
func (d *CodexDriver) Executable() string {
	if d.cliPath != "" {
		return d.cliPath
	}
	return "codex"
}

// BuildCLIArgs constructs the codex exec invocation. mcpConfigPath is unused:
// Codex takes MCP configuration as -c overrides, not a Claude-style JSON file.
func (d *CodexDriver) BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, _ string) []string {
	args := []string{
		"exec", "--json",
		"-s", "workspace-write",
		"-c", "approval_policy=never",
		"-c", "mcp_servers={}",
	}
	if d.model != "" {
		args = append(args, "-m", d.model)
	}
	if d.effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+tomlQuote(d.effort))
	}
	if mcp.HTTPURL != "" {
		name := mcp.Name
		if name == "" {
			name = "flywheel"
		}
		args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.url=%s", name, tomlQuote(mcp.HTTPURL)))
		if len(mcp.Headers) > 0 {
			parts := make([]string, 0, len(mcp.Headers))
			for k, v := range mcp.Headers {
				parts = append(parts, tomlQuote(k)+" = "+tomlQuote(v))
			}
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.http_headers={ %s }", name, strings.Join(parts, ", ")))
		}
	}
	args = append(args, d.extraArgs...)
	prompt := taskMessage
	if strings.TrimSpace(systemPrompt) != "" {
		prompt = systemPrompt + "\n\n# Task\n\n" + taskMessage
	}
	return append(args, prompt)
}

// FormatPrompt labels the system prompt as instructions for the preamble.
func (d *CodexDriver) FormatPrompt(systemPrompt string) string {
	if strings.TrimSpace(systemPrompt) == "" {
		return ""
	}
	return "# Instructions\n\n" + systemPrompt
}

// Env marks the process as a Flywheel dispatch so the session collector can classify it.
func (d *CodexDriver) Env(_, _ string, mcp mcpConnection, _ string) DriverEnv {
	return DriverEnv{
		Set: map[string]string{
			"FLYWHEEL_DISPATCH": "true",
			"FLYWHEEL_MCP_URL":  mcp.HTTPURL,
		},
	}
}

// ResolveCredential returns the explicit key; Codex normally uses its own ChatGPT login.
func (d *CodexDriver) ResolveCredential(staticKey string) string { return staticKey }

// CredentialEnvName is the provider variable Codex reads when an API key is used.
func (d *CodexDriver) CredentialEnvName() string { return "OPENAI_API_KEY" }

// DefaultAllowedHosts lists the endpoints Codex needs.
func (d *CodexDriver) DefaultAllowedHosts() []string {
	return []string{"api.openai.com", "chatgpt.com", "auth.openai.com"}
}

// tomlQuote renders s as a TOML basic string (JSON escaping is a valid subset).
func tomlQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
