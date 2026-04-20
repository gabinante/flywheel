package dispatch

import "fmt"

// AgentDriver encapsulates agent-specific behavior for the dispatch system.
// The dispatch infrastructure (worktree management, MCP config generation,
// prompt assembly, Docker resource limits, and event handling) remains
// agent-agnostic. Only the CLI invocation and environment differ per agent.
//
// To add a new agent driver:
//  1. Implement the AgentDriver interface.
//  2. Register it in the driverRegistry map below.
//  3. Set DISPATCH_AGENT_DRIVER=<name> in your environment.
//
// See driver_claude.go and driver_generic.go for examples.
type AgentDriver interface {
	// Name returns the driver identifier for logging and config selection.
	Name() string

	// BuildCLIArgs returns the executable path and arguments for host-mode execution.
	// The returned exe is the binary to invoke; args are its CLI arguments.
	// systemPrompt and taskMessage are the assembled prompt strings.
	// mcpConfigPath is the path to the written MCP config JSON file.
	BuildCLIArgs(systemPrompt, taskMessage, mcpConfigPath string) (exe string, args []string)

	// BuildDockerCmd returns the shell command to run inside a Docker container.
	// Standard file mount paths:
	//   /tmp/system-prompt.txt  — system prompt content
	//   /tmp/task-prompt.txt    — task message content
	//   /tmp/mcp-config.json   — MCP server configuration
	// branch is the git branch to check out inside the container.
	BuildDockerCmd(branch string) string

	// DockerImage returns the preferred Docker image for this agent.
	// Return empty string to use the default from DispatchConfig.DockerImage.
	DockerImage() string

	// FormatPrompt optionally transforms the system prompt for agent-specific needs.
	// Most drivers return the input unchanged.
	FormatPrompt(systemPrompt string) string

	// Env returns environment configuration for the agent process.
	Env() DriverEnv

	// ResolveCredential resolves the agent's API credential.
	// For agents with dynamic credential sources (e.g., OAuth keychain),
	// this returns the resolved credential. Falls back to staticKey if no
	// dynamic source is available. Return empty string if no credential is needed.
	ResolveCredential(staticKey string) string

	// ExtraDockerArgs returns additional docker run arguments (e.g., volumes, env vars)
	// specific to this agent. These are appended before the image name.
	ExtraDockerArgs() []string
}

// DriverEnv describes environment configuration for an agent driver.
type DriverEnv struct {
	// FilterPrefixes lists env var prefixes to remove from the parent environment.
	// Each entry should include the '=' suffix for prefix matching.
	// Example: ["CLAUDECODE=", "ANTHROPIC_API_KEY="]
	FilterPrefixes []string

	// Set lists additional env vars to set for the agent process.
	// Example: {"CLAUDE_CODE_ENTRYPOINT": "warrant-dispatch"}
	Set map[string]string
}

// driverRegistry maps driver names to constructor functions.
var driverRegistry = map[string]func(cfg DriverConfig) AgentDriver{
	"claude":  func(cfg DriverConfig) AgentDriver { return NewClaudeDriver(cfg) },
	"generic": func(cfg DriverConfig) AgentDriver { return NewGenericDriver(cfg) },
}

// DriverConfig holds configuration used by driver constructors.
type DriverConfig struct {
	// CLIPath is the path to the agent binary (e.g., "claude", "opencode").
	CLIPath string

	// ExtraArgs are additional static arguments to pass to the agent command.
	ExtraArgs []string
}

// LookupDriver returns an AgentDriver by name from the registry.
// Returns an error if the driver name is not registered.
func LookupDriver(name string, cfg DriverConfig) (AgentDriver, error) {
	ctor, ok := driverRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent driver %q (available: claude, generic)", name)
	}
	return ctor(cfg), nil
}

// RegisterDriver registers a new agent driver constructor.
// Use this in init() functions to add custom drivers.
func RegisterDriver(name string, ctor func(cfg DriverConfig) AgentDriver) {
	driverRegistry[name] = ctor
}
