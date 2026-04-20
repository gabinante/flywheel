# Agent Drivers

The dispatch system uses a pluggable **AgentDriver** interface to support any coding agent — not just Claude Code. This document explains how agent drivers work and how to add new ones.

## Overview

The dispatch architecture separates **infrastructure** from **agent behavior**:

- **Infrastructure (agent-agnostic):** Dispatcher event handling, capacity management, worktree management, MCP config generation, prompt assembly, Docker resource limits, lease management, failure recovery.
- **Agent behavior (driver-specific):** CLI arguments, environment variables, Docker image, prompt formatting, credential resolution, volume mounts.

## Built-in Drivers

### `claude` (default)

The Claude Code driver. Invokes `claude --print --dangerously-skip-permissions --system-prompt <prompt> <task> --mcp-config <path>`.

- Resolves OAuth tokens from the macOS keychain for cost-free usage.
- Mounts persistent `.claude` data directory in Docker mode.
- Filters `CLAUDECODE=` and `ANTHROPIC_API_KEY=` from the parent environment.
- Sets `CLAUDE_CODE_ENTRYPOINT=warrant-dispatch`.

### `generic`

A generic driver that delivers prompts via environment variables. Works with any agent that reads from env vars.

Environment variables set for the agent:
- `WARRANT_SYSTEM_PROMPT` — full system prompt text
- `WARRANT_TASK_MESSAGE` — task instruction
- `WARRANT_MCP_CONFIG_PATH` — path to the MCP configuration JSON file
- `WARRANT_DISPATCH=true` — marker that the agent was spawned by warrant

The agent is expected to:
1. Read its instructions from the environment variables above.
2. Connect to the MCP server using the config file.
3. Execute the work and produce output on stdout.

## Configuration

Set the driver via environment variable:

```bash
# Select the agent driver (default: "claude")
DISPATCH_AGENT_DRIVER=generic

# Override the agent binary path (default depends on driver)
DISPATCH_AGENT_CMD=/usr/local/bin/opencode

# Standard dispatch settings still apply
DISPATCH_ENABLED=true
DISPATCH_MAX_WORKERS=4
DISPATCH_DOCKER_ENABLED=false
```

## Adding a New Driver

### 1. Implement the `AgentDriver` interface

Create a new file `internal/dispatch/driver_myagent.go`:

```go
package dispatch

// MyAgentDriver implements AgentDriver for MyAgent.
type MyAgentDriver struct {
    CLIPath string
}

func NewMyAgentDriver(cfg DriverConfig) *MyAgentDriver {
    path := cfg.CLIPath
    if path == "" {
        path = "myagent"
    }
    return &MyAgentDriver{CLIPath: path}
}

func (d *MyAgentDriver) Name() string { return "myagent" }

func (d *MyAgentDriver) BuildCLIArgs(systemPrompt, taskMessage, mcpConfigPath string) (string, []string) {
    return d.CLIPath, []string{
        "--prompt", systemPrompt,
        "--task", taskMessage,
        "--mcp", mcpConfigPath,
    }
}

func (d *MyAgentDriver) BuildDockerCmd(branch string) string {
    return fmt.Sprintf(`set -e
git clone /repo /workspace 2>/dev/null
cd /workspace
git checkout -b %s 2>/dev/null || git checkout %s
myagent --prompt "$(cat /tmp/system-prompt.txt)" --task "$(cat /tmp/task-prompt.txt)" --mcp /tmp/mcp-config.json`,
        branch, branch)
}

func (d *MyAgentDriver) DockerImage() string { return "myagent-worker" }

func (d *MyAgentDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

func (d *MyAgentDriver) Env() DriverEnv {
    return DriverEnv{
        FilterPrefixes: nil,
        Set:            map[string]string{"MYAGENT_MODE": "dispatch"},
    }
}

func (d *MyAgentDriver) ResolveCredential(staticKey string) string { return staticKey }

func (d *MyAgentDriver) ExtraDockerArgs() []string { return nil }
```

### 2. Register the driver

Add registration in an `init()` function or in the driver file:

```go
func init() {
    RegisterDriver("myagent", func(cfg DriverConfig) AgentDriver {
        return NewMyAgentDriver(cfg)
    })
}
```

### 3. Configure and use

```bash
DISPATCH_AGENT_DRIVER=myagent
DISPATCH_AGENT_CMD=/usr/local/bin/myagent
DISPATCH_DOCKER_IMAGE=myagent-worker  # if using Docker mode
```

## Interface Reference

```go
type AgentDriver interface {
    // Name returns the driver identifier for logging and config.
    Name() string

    // BuildCLIArgs returns the executable and CLI arguments for host-mode execution.
    BuildCLIArgs(systemPrompt, taskMessage, mcpConfigPath string) (exe string, args []string)

    // BuildDockerCmd returns the shell command to run inside a Docker container.
    // Files are mounted at: /tmp/system-prompt.txt, /tmp/task-prompt.txt, /tmp/mcp-config.json
    BuildDockerCmd(branch string) string

    // DockerImage returns the preferred Docker image. Empty = use config default.
    DockerImage() string

    // FormatPrompt transforms the system prompt for agent-specific needs.
    FormatPrompt(systemPrompt string) string

    // Env returns environment configuration (filters and additions).
    Env() DriverEnv

    // ResolveCredential resolves the agent's API credential dynamically.
    ResolveCredential(staticKey string) string

    // ExtraDockerArgs returns additional docker run arguments.
    ExtraDockerArgs() []string
}
```

## Design Decisions

- **Prompt delivery is driver-specific.** Claude uses CLI flags; generic uses env vars. Each agent has its own way of receiving instructions.
- **MCP config format is shared.** All agents connect to the same warrant MCP server using the same JSON config format. The driver doesn't need to know about MCP internals.
- **Docker resource limits are infrastructure.** Memory, CPU, PID limits, and firewall rules are configured globally, not per-driver.
- **Credentials are driver-resolved.** Claude can use OAuth tokens from the keychain; other agents may use API keys or different auth mechanisms.
- **The dispatcher remains agent-agnostic.** Event handling, capacity management, worktree creation, lease management, and failure recovery work the same regardless of which agent driver is selected.
