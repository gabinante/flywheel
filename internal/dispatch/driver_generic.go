package dispatch

import "fmt"

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

// BuildDockerCmd returns a shell command that sets up the workspace and
// invokes the agent with prompts available via environment and files.
func (d *GenericDriver) BuildDockerCmd(branch string, mcp mcpConnection) string {
	exe := d.Executable()
	// Build the args string for the docker command.
	argsStr := ""
	for _, a := range d.Args {
		argsStr += " " + a
	}
	return fmt.Sprintf(
		`set -e
git clone /repo /workspace 2>/dev/null
cd /workspace
git checkout -b %s 2>/dev/null || git checkout %s
export FLYWHEEL_SYSTEM_PROMPT="$(cat /tmp/system-prompt.txt)"
export FLYWHEEL_TASK_MESSAGE="$(cat /tmp/task-prompt.txt)"
export FLYWHEEL_MCP_URL=%q
export FLYWHEEL_MCP_SSE_URL=%q
export FLYWHEEL_MCP_CONFIG_PATH=/tmp/mcp-config.json
export FLYWHEEL_MCP_HEADERS_JSON=%q
export WARRANT_SYSTEM_PROMPT="$FLYWHEEL_SYSTEM_PROMPT"
export WARRANT_TASK_MESSAGE="$FLYWHEEL_TASK_MESSAGE"
export WARRANT_MCP_URL="$FLYWHEEL_MCP_URL"
export WARRANT_MCP_SSE_URL="$FLYWHEEL_MCP_SSE_URL"
export WARRANT_MCP_CONFIG_PATH="$FLYWHEEL_MCP_CONFIG_PATH"
export WARRANT_MCP_HEADERS_JSON="$FLYWHEEL_MCP_HEADERS_JSON"
%s%s`,
		branch, branch, mcp.HTTPURL, mcp.SSEURL, mustJSON(mcp.Headers), exe, argsStr,
	)
}

// DockerImage returns empty to use the default image from config.
// Operators should set DISPATCH_DOCKER_IMAGE to an image with their agent installed.
func (d *GenericDriver) DockerImage() string { return "" }

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

// ExtraDockerArgs returns no additional Docker arguments.
// Operators can customize Docker behavior via DISPATCH_DOCKER_* config.
func (d *GenericDriver) ExtraDockerArgs() []string { return nil }

func mustJSON(headers map[string]string) string {
	if len(headers) == 0 {
		return "{}"
	}
	buf := "{"
	first := true
	for k, v := range headers {
		if !first {
			buf += ","
		}
		first = false
		buf += fmt.Sprintf("%q:%q", k, v)
	}
	buf += "}"
	return buf
}
