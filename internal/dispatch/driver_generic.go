package dispatch

import "encoding/json"

// GenericDriver implements AgentDriver for any MCP-capable coding agent.
// It delivers prompts via environment variables and stdin rather than
// agent-specific CLI flags. This enables agents like OpenCode, Aider,
// Cline, or custom agents to work with the dispatch system.
//
// Environment variables set by GenericDriver:
//
//	WARRANT_SYSTEM_PROMPT    — the full system prompt text
//	WARRANT_TASK_MESSAGE     — the task instruction
//	WARRANT_MCP_CONFIG_PATH  — path to the MCP config JSON file
//	WARRANT_TICKET_ID        — current ticket ID (extracted from task message)
//
// The agent is expected to:
//  1. Read WARRANT_SYSTEM_PROMPT for context and instructions.
//  2. Read WARRANT_TASK_MESSAGE for the specific task to execute.
//  3. Connect to the MCP server using the config at WARRANT_MCP_CONFIG_PATH.
//  4. Execute the work and produce output on stdout.
type GenericDriver struct {
	// Command is the executable to run. Defaults to DISPATCH_AGENT_CMD env var
	// or "agent" if unset.
	Command string

	// Args are additional static arguments to pass to the command.
	Args []string
}

// NewGenericDriver creates a GenericDriver from config.
func NewGenericDriver(cfg DriverConfig) *GenericDriver {
	cmd := cfg.CLIPath
	if cmd == "" {
		cmd = "agent"
	}
	return &GenericDriver{
		Command: cmd,
		Args:    cfg.ExtraArgs,
	}
}

func (d *GenericDriver) Name() string { return "generic" }

func (d *GenericDriver) Executable() string {
	if d.Command == "" {
		return "agent"
	}
	return d.Command
}

// BuildCLIArgs returns the agent command with prompt info delivered via environment.
// The agent binary receives FLYWHEEL_* and WARRANT_* env vars (set via Env()).
// No agent-specific flags are assumed — only the bare command and optional extra args.
func (d *GenericDriver) BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) []string {
	// Pass extra args if configured; the agent is expected to read prompts from env.
	args := make([]string, len(d.Args))
	copy(args, d.Args)
	return args
}

// FormatPrompt passes through unchanged — generic agents receive raw markdown.
func (d *GenericDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

// Env returns environment variables that deliver prompts to the agent.
// Unlike Claude which uses CLI flags, the generic driver uses env vars.
func (d *GenericDriver) Env(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) DriverEnv {
	return DriverEnv{
		FilterPrefixes: nil, // Don't filter any parent env vars by default.
		Set: map[string]string{
			"WARRANT_DISPATCH":          "true",
			"FLYWHEEL_DISPATCH":         "true",
			"FLYWHEEL_SYSTEM_PROMPT":    systemPrompt,
			"FLYWHEEL_TASK_MESSAGE":     taskMessage,
			"FLYWHEEL_MCP_URL":          mcp.HTTPURL,
			"FLYWHEEL_MCP_SSE_URL":      mcp.SSEURL,
			"FLYWHEEL_MCP_CONFIG_PATH":  mcpConfigPath,
			"FLYWHEEL_MCP_HEADERS_JSON": mustJSON(mcp.Headers),
			"WARRANT_SYSTEM_PROMPT":     systemPrompt,
			"WARRANT_TASK_MESSAGE":      taskMessage,
			"WARRANT_MCP_URL":           mcp.HTTPURL,
			"WARRANT_MCP_SSE_URL":       mcp.SSEURL,
			"WARRANT_MCP_CONFIG_PATH":   mcpConfigPath,
			"WARRANT_MCP_HEADERS_JSON":  mustJSON(mcp.Headers),
		},
	}
}

// ResolveCredential returns the static key unchanged.
// Generic agents manage their own authentication.
func (d *GenericDriver) ResolveCredential(staticKey string) string {
	return staticKey
}

func (d *GenericDriver) CredentialEnvName() string { return "" }

func (d *GenericDriver) DefaultAllowedHosts() []string { return nil }

// mustJSON marshals v for embedding in environment variables; marshal errors
// are impossible for the map types used here, so they fall back to "{}".
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
