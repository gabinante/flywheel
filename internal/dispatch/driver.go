package dispatch

import (
	"fmt"
	"sort"
	"strings"
)

// AgentDriver encapsulates harness-specific behavior for CLI and Docker workers.
// API-native runners do not use AgentDriver directly; they own their own request
// and tool execution loop. The dispatch infrastructure (worktree management, MCP
// config generation, prompt assembly, Docker resource limits, and event handling)
// remains agent-agnostic.
//
// To add a new CLI or Docker agent driver:
//  1. Implement the AgentDriver interface.
//  2. Register it in the driverRegistry map below.
//  3. Set DISPATCH_AGENT_DRIVER=<name> in your environment.
//
// See driver_claude.go and driver_generic.go for examples.
type AgentDriver interface {
	// Name returns the driver identifier for logging and config selection.
	Name() string

	// Executable returns the binary to invoke for host-mode execution.
	Executable() string

	// BuildCLIArgs returns the CLI arguments for host-mode execution.
	// systemPrompt and taskMessage are the assembled prompt strings.
	// mcp provides both SSE and streamable HTTP endpoint details.
	// mcpConfigPath is the path to the written Claude-compatible MCP config file.
	BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) []string

	// FormatPrompt optionally transforms the system prompt for agent-specific needs.
	// Most drivers return the input unchanged.
	FormatPrompt(systemPrompt string) string

	// Env returns environment configuration for the agent process.
	Env(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) DriverEnv

	// ResolveCredential resolves the agent's API credential.
	// For agents with dynamic credential sources (e.g., OAuth keychain),
	// this returns the resolved credential. Falls back to staticKey if no
	// dynamic source is available. Return empty string if no credential is needed.
	ResolveCredential(staticKey string) string

	// CredentialEnvName is the environment variable name used to pass the
	// resolved credential into Docker workers. Return empty when not applicable.
	CredentialEnvName() string

	// DefaultAllowedHosts returns the default firewall allowlist for dockerized
	// workers when DISPATCH_DOCKER_FIREWALL is enabled and no explicit host list
	// is configured.
	DefaultAllowedHosts() []string
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
	"codex":   func(cfg DriverConfig) AgentDriver { return NewCodexDriver(cfg) },
	"generic": func(cfg DriverConfig) AgentDriver { return NewGenericDriver(cfg) },
}

// DriverConfig holds configuration used by driver constructors.
type DriverConfig struct {
	// CLIPath is the path to the agent binary (e.g., "claude", "opencode").
	CLIPath string

	// ExtraArgs are additional static arguments to pass to the agent command.
	ExtraArgs []string

	// Model and ReasoningEffort are passed to harnesses that accept them (claude --model, codex -m / -c model_reasoning_effort).
	Model           string
	ReasoningEffort string
}

// LookupDriver returns an AgentDriver by name from the registry.
// Returns an error if the driver name is not registered.
func LookupDriver(name string, cfg DriverConfig) (AgentDriver, error) {
	ctor, ok := driverRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent driver %q (available: %s)", name, strings.Join(AvailableDrivers(), ", "))
	}
	return ctor(cfg), nil
}

// RegisterDriver registers a new agent driver constructor.
// Use this in init() functions to add custom drivers.
func RegisterDriver(name string, ctor func(cfg DriverConfig) AgentDriver) {
	driverRegistry[name] = ctor
}

// AvailableDrivers returns the registered driver names in stable order.
func AvailableDrivers() []string {
	names := make([]string, 0, len(driverRegistry))
	for name := range driverRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
