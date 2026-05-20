# Dispatch Backends

The dispatch system separates **execution backend** from **agent harness** so Flywheel can support CLI agents, Dockerized agents, and API-native agents.

## Overview

The dispatch architecture now has two extension points:

- **Runner:** how the worker is executed.
  - `cli` runs a local agent binary in the ticket worktree.
  - `docker` runs the agent inside a container.
  - `openai-responses` uses the OpenAI Responses API with Flywheel MCP plus local workspace tools.
  - `openai-compatible` uses the standard OpenAI Chat Completions API with Flywheel MCP plus local workspace tools.
- **Driver:** how a CLI or Docker harness wants its prompt, MCP config, env vars, and credentials.

Infrastructure such as dispatcher event handling, worktree management, prompt assembly, lease recovery, and MCP connection generation stays shared.

## Built-in Runners

### `cli`

Runs a local agent binary directly in the ticket worktree.

### `docker`

Runs a harness inside the configured worker image with the existing sandbox and firewall controls.

### `openai-responses`

Uses the OpenAI Responses API as the worker runtime.

- Connects to Flywheel over streamable HTTP MCP at `/mcp`.
- Exposes local workspace tools for file reads, file writes, directory listing, and shell commands inside the ticket worktree.
- Uses `OPENAI_API_KEY` as the provider fallback when `DISPATCH_AGENT_API_KEY` is unset.
- Is currently a host-side backend: it operates in the local worktree rather than inside the Docker worker image.

### `openai-compatible`

Uses the OpenAI-compatible `/chat/completions` API as the worker runtime.

- Designed for providers and gateways that implement the standard OpenAI tool-calling surface, including LiteLLM and vLLM.
- Connects to Flywheel over streamable HTTP MCP at `/mcp` and mirrors MCP tools into chat-completions function tools.
- Exposes the same local workspace tools for file reads, file writes, directory listing, and shell commands inside the ticket worktree.
- Uses `OPENAI_API_KEY` as the provider fallback when `DISPATCH_AGENT_API_KEY` is unset.
- Requires an explicit `DISPATCH_AGENT_MODEL`; unlike `openai-responses`, Flywheel does not assume an OpenAI-hosted default model.
- Is currently a host-side backend: it operates in the local worktree rather than inside the Docker worker image.

## Built-in Drivers

Drivers apply to the `cli` and `docker` runners.

### `claude` (default)

The Claude Code driver. Invokes `claude --print --dangerously-skip-permissions --system-prompt <prompt> <task> --mcp-config <path>`.

- Resolves OAuth tokens from the macOS keychain for cost-free usage.
- Mounts persistent `.claude` data directory in Docker mode.
- Filters `CLAUDECODE=` and `ANTHROPIC_API_KEY=` from the parent environment.
- Sets `CLAUDE_CODE_ENTRYPOINT=warrant-dispatch`.

### `generic`

A generic driver that delivers prompts via environment variables. Works with any agent that reads from env vars.

Environment variables set for the agent:
- `FLYWHEEL_SYSTEM_PROMPT` — full system prompt text
- `FLYWHEEL_TASK_MESSAGE` — task instruction
- `FLYWHEEL_MCP_URL` — Flywheel streamable HTTP MCP endpoint (`/mcp`)
- `FLYWHEEL_MCP_SSE_URL` — Flywheel SSE MCP endpoint (`/sse`)
- `FLYWHEEL_MCP_HEADERS_JSON` — JSON map of auth headers
- `FLYWHEEL_MCP_CONFIG_PATH` — Claude-compatible MCP config JSON file path
- `FLYWHEEL_DISPATCH=true` — marker that the agent was spawned by warrant
- `WARRANT_SYSTEM_PROMPT` — full system prompt text
- `WARRANT_TASK_MESSAGE` — task instruction
- `WARRANT_MCP_URL` — Flywheel streamable HTTP MCP endpoint (`/mcp`)
- `WARRANT_MCP_SSE_URL` — Flywheel SSE MCP endpoint (`/sse`)
- `WARRANT_MCP_HEADERS_JSON` — JSON map of auth headers
- `WARRANT_MCP_CONFIG_PATH` — Claude-compatible MCP config JSON file path
- `WARRANT_DISPATCH=true` — marker that the agent was spawned by warrant

The agent is expected to:
1. Read its instructions from the environment variables above.
2. Connect to the MCP server using `*_MCP_URL` (or `*_MCP_SSE_URL` if needed).
3. Execute the work and produce output on stdout.

## Configuration

Set the runner first, then the driver when the runner is `cli` or `docker`:

```bash
# CLI or Docker harnesses (default: Claude)
DISPATCH_AGENT_RUNNER=cli
DISPATCH_AGENT_DRIVER=claude

# API-native OpenAI Responses backend
DISPATCH_AGENT_RUNNER=openai-responses
OPENAI_API_KEY=sk-...
DISPATCH_AGENT_MODEL=gpt-4o

# OpenAI-compatible backend (LiteLLM, vLLM, local gateways)
DISPATCH_AGENT_RUNNER=openai-compatible
DISPATCH_AGENT_API_BASE_URL=http://localhost:4000/v1
DISPATCH_AGENT_MODEL=qwen2.5-coder
OPENAI_API_KEY=sk-...

# Standard dispatch settings still apply
DISPATCH_ENABLED=true
DISPATCH_MAX_WORKERS=4
```

## Adding a New Driver

Only do this for a CLI or Docker harness. API-native backends should add a new runner instead.

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

func (d *MyAgentDriver) Executable() string { return d.CLIPath }

func (d *MyAgentDriver) BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) []string {
    return []string{
        "--prompt", systemPrompt,
        "--task", taskMessage,
        "--mcp", mcpConfigPath,
    }
}

func (d *MyAgentDriver) BuildDockerCmd(branch string, mcp mcpConnection) string {
    return fmt.Sprintf(`set -e
git clone /repo /workspace 2>/dev/null
cd /workspace
git checkout -b %s 2>/dev/null || git checkout %s
myagent --prompt "$(cat /tmp/system-prompt.txt)" --task "$(cat /tmp/task-prompt.txt)" --mcp /tmp/mcp-config.json`,
        branch, branch)
}

func (d *MyAgentDriver) DockerImage() string { return "myagent-worker" }

func (d *MyAgentDriver) FormatPrompt(systemPrompt string) string { return systemPrompt }

func (d *MyAgentDriver) Env(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) DriverEnv {
    return DriverEnv{
        FilterPrefixes: nil,
        Set:            map[string]string{"MYAGENT_MODE": "dispatch"},
    }
}

func (d *MyAgentDriver) ResolveCredential(staticKey string) string { return staticKey }

func (d *MyAgentDriver) CredentialEnvName() string { return "" }

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

## Driver Interface Reference

```go
type AgentDriver interface {
    // Name returns the driver identifier for logging and config.
    Name() string

    // Executable returns the agent binary path.
    Executable() string

    // BuildCLIArgs returns CLI arguments for host-mode execution.
    BuildCLIArgs(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) []string

    // BuildDockerCmd returns the shell command to run inside a Docker container.
    // Files are mounted at: /tmp/system-prompt.txt, /tmp/task-prompt.txt, /tmp/mcp-config.json
    BuildDockerCmd(branch string, mcp mcpConnection) string

    // DockerImage returns the preferred Docker image. Empty = use config default.
    DockerImage() string

    // FormatPrompt transforms the system prompt for agent-specific needs.
    FormatPrompt(systemPrompt string) string

    // Env returns environment configuration (filters and additions).
    Env(systemPrompt, taskMessage string, mcp mcpConnection, mcpConfigPath string) DriverEnv

    // ResolveCredential resolves the agent's API credential dynamically.
    ResolveCredential(staticKey string) string

    // CredentialEnvName returns the env var used for docker credentials.
    CredentialEnvName() string

    // DefaultAllowedHosts returns the default docker firewall allowlist.
    DefaultAllowedHosts() []string

    // ExtraDockerArgs returns additional docker run arguments.
    ExtraDockerArgs() []string
}
```

## Design Decisions

- **Prompt delivery is driver-specific.** Claude uses CLI flags; generic uses env vars. Each agent has its own way of receiving instructions.
- **MCP transport is shared, not the client config format.** Flywheel exposes both `/sse` and `/mcp`; drivers choose the transport and config shape their harness actually understands.
- **Docker resource limits are infrastructure.** Memory, CPU, PID limits, and firewall rules are configured globally, not per-driver.
- **Credentials are driver-resolved.** Claude can use OAuth tokens from the keychain; other agents may use API keys or different auth mechanisms.
- **The dispatcher remains agent-agnostic.** Event handling, capacity management, worktree creation, lease management, and failure recovery work the same regardless of which agent driver is selected.
