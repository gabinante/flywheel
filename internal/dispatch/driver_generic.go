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

// BuildCLIArgs returns the agent command with prompt info delivered via environment.
// The agent binary receives WARRANT_* env vars (set via Env()) and the MCP config path.
// No agent-specific flags are assumed — only the bare command and optional extra args.
func (d *GenericDriver) BuildCLIArgs(systemPrompt, taskMessage, mcpConfigPath string) (string, []string) {
	exe := d.Command
	if exe == "" {
		exe = "agent"
	}
	// Pass extra args if configured; the agent is expected to read prompts from env.
	args := make([]string, len(d.Args))
	copy(args, d.Args)
	return exe, args
}

// BuildDockerCmd returns a shell command that sets up the workspace and
// invokes the agent with prompts available via environment and files.
func (d *GenericDriver) BuildDockerCmd(branch string) string {
	exe := d.Command
	if exe == "" {
		exe = "agent"
	}
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
export WARRANT_SYSTEM_PROMPT="$(cat /tmp/system-prompt.txt)"
export WARRANT_TASK_MESSAGE="$(cat /tmp/task-prompt.txt)"
export WARRANT_MCP_CONFIG_PATH=/tmp/mcp-config.json
%s%s`,
		branch, branch, exe, argsStr,
	)
}

// DockerImage returns empty to use the default image from config.
// Operators should set DISPATCH_DOCKER_IMAGE to an image with their agent installed.
func (d *GenericDriver) DockerImage() string { return "" }

// FormatPrompt passes through unchanged — generic agents receive raw markdown.
func (d *GenericDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

// Env returns environment variables that deliver prompts to the agent.
// Unlike Claude which uses CLI flags, the generic driver uses env vars.
func (d *GenericDriver) Env() DriverEnv {
	return DriverEnv{
		FilterPrefixes: nil, // Don't filter any parent env vars by default.
		Set:            map[string]string{
			// WARRANT_SYSTEM_PROMPT and WARRANT_TASK_MESSAGE are set dynamically
			// in the worker before spawning. We declare the entrypoint marker here.
			"WARRANT_DISPATCH": "true",
		},
	}
}

// ResolveCredential returns the static key unchanged.
// Generic agents manage their own authentication.
func (d *GenericDriver) ResolveCredential(staticKey string) string {
	return staticKey
}

// ExtraDockerArgs returns no additional Docker arguments.
// Operators can customize Docker behavior via DISPATCH_DOCKER_* config.
func (d *GenericDriver) ExtraDockerArgs() []string { return nil }
