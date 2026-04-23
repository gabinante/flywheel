package dispatch

import (
	"strings"
	"testing"
)

func testMCPConnection() mcpConnection {
	return buildMCPConnection("http://localhost:8080", "test-api-key")
}

// --- ClaudeDriver Tests ---

func TestClaudeDriverName(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	if d.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", d.Name())
	}
}

func TestClaudeDriverExecutable(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{CLIPath: "/usr/local/bin/claude"})
	if d.Executable() != "/usr/local/bin/claude" {
		t.Errorf("expected executable '/usr/local/bin/claude', got %q", d.Executable())
	}
}

func TestClaudeDriverBuildCLIArgs(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	args := d.BuildCLIArgs("system prompt here", "do the task", testMCPConnection(), "/tmp/mcp.json")

	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--print") {
		t.Error("expected --print flag")
	}
	if !strings.Contains(argStr, "--dangerously-skip-permissions") {
		t.Error("expected --dangerously-skip-permissions flag")
	}
	if !strings.Contains(argStr, "--system-prompt") {
		t.Error("expected --system-prompt flag")
	}
	if !strings.Contains(argStr, "system prompt here") {
		t.Error("expected system prompt in args")
	}
	if !strings.Contains(argStr, "do the task") {
		t.Error("expected task message in args")
	}
	if !strings.Contains(argStr, "--mcp-config") {
		t.Error("expected --mcp-config flag")
	}
	if !strings.Contains(argStr, "/tmp/mcp.json") {
		t.Error("expected MCP config path in args")
	}
}

func TestClaudeDriverBuildDockerCmd(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	cmd := d.BuildDockerCmd("ticket/t-1", testMCPConnection())

	if !strings.Contains(cmd, "git clone /repo /workspace") {
		t.Error("expected git clone in docker cmd")
	}
	if !strings.Contains(cmd, "git checkout -b ticket/t-1") {
		t.Error("expected branch checkout in docker cmd")
	}
	if !strings.Contains(cmd, "claude --print --dangerously-skip-permissions") {
		t.Error("expected claude CLI invocation in docker cmd")
	}
	if !strings.Contains(cmd, "/tmp/system-prompt.txt") {
		t.Error("expected system prompt file reference")
	}
	if !strings.Contains(cmd, "/tmp/task-prompt.txt") {
		t.Error("expected task prompt file reference")
	}
	if !strings.Contains(cmd, "/tmp/mcp-config.json") {
		t.Error("expected MCP config file reference")
	}
}

func TestClaudeDriverDockerImage(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	if d.DockerImage() != "" {
		t.Errorf("expected empty docker image (use default), got %q", d.DockerImage())
	}
}

func TestClaudeDriverFormatPrompt(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	input := "# System Prompt\n\nBe helpful."
	output := d.FormatPrompt(input)
	if output != input {
		t.Error("ClaudeDriver should pass through prompt unchanged")
	}
}

func TestClaudeDriverEnv(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	env := d.Env("system", "task", testMCPConnection(), "/tmp/mcp.json")

	if len(env.FilterPrefixes) != 2 {
		t.Fatalf("expected 2 filter prefixes, got %d", len(env.FilterPrefixes))
	}
	found := map[string]bool{}
	for _, p := range env.FilterPrefixes {
		found[p] = true
	}
	if !found["CLAUDECODE="] {
		t.Error("expected CLAUDECODE= in filter prefixes")
	}
	if !found["ANTHROPIC_API_KEY="] {
		t.Error("expected ANTHROPIC_API_KEY= in filter prefixes")
	}
	if env.Set["CLAUDE_CODE_ENTRYPOINT"] != "warrant-dispatch" {
		t.Errorf("expected CLAUDE_CODE_ENTRYPOINT=warrant-dispatch, got %q", env.Set["CLAUDE_CODE_ENTRYPOINT"])
	}
}

func TestClaudeDriverResolveCredentialFallback(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	if got := d.ResolveCredential("sk-test-key"); got == "" {
		t.Error("expected non-empty credential")
	}
}

func TestClaudeDriverCredentialEnvName(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	if d.CredentialEnvName() != "ANTHROPIC_API_KEY" {
		t.Errorf("expected ANTHROPIC_API_KEY, got %q", d.CredentialEnvName())
	}
}

func TestClaudeDriverDefaultAllowedHosts(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	hosts := strings.Join(d.DefaultAllowedHosts(), ",")
	if !strings.Contains(hosts, "api.anthropic.com") {
		t.Errorf("expected Anthropic host allowlist, got %q", hosts)
	}
}

func TestClaudeDriverExtraDockerArgs(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	args := d.ExtraDockerArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 extra docker args (-v and path), got %d", len(args))
	}
	if args[0] != "-v" {
		t.Errorf("expected first arg to be '-v', got %q", args[0])
	}
	if !strings.Contains(args[1], ".claude:delegated") {
		t.Errorf("expected .claude volume mount, got %q", args[1])
	}
}

// --- CodexDriver Tests ---

func TestCodexDriverName(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	if d.Name() != "codex" {
		t.Errorf("expected name 'codex', got %q", d.Name())
	}
}

func TestCodexDriverExecutable(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	if d.Executable() != "codex" {
		t.Errorf("expected default executable 'codex', got %q", d.Executable())
	}
}

func TestCodexDriverBuildCLIArgs(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	args := d.BuildCLIArgs("be careful", "fix the bug", testMCPConnection(), "/tmp/mcp.json")
	argStr := strings.Join(args, " ")

	if !strings.Contains(argStr, "exec") {
		t.Error("expected codex exec command")
	}
	if !strings.Contains(argStr, "--ask-for-approval never") {
		t.Error("expected non-interactive approval policy")
	}
	if !strings.Contains(argStr, "--sandbox workspace-write") {
		t.Error("expected workspace-write sandbox")
	}
	if !strings.Contains(argStr, `mcp_servers.flywheel.url="http://localhost:8080/mcp"`) {
		t.Error("expected Codex to target the /mcp endpoint")
	}
	if !strings.Contains(argStr, `env_http_headers={"X-API-Key"="FLYWHEEL_MCP_API_KEY"}`) {
		t.Error("expected API key header to flow through env_http_headers")
	}
	if !strings.Contains(argStr, "## System Instructions") || !strings.Contains(argStr, "## Task") {
		t.Error("expected combined system/task prompt")
	}
}

func TestCodexDriverBuildDockerCmd(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	cmd := d.BuildDockerCmd("ticket/t-9", testMCPConnection())

	if !strings.Contains(cmd, "codex exec") {
		t.Error("expected codex exec invocation in docker cmd")
	}
	if !strings.Contains(cmd, "--dangerously-bypass-approvals-and-sandbox") {
		t.Error("expected sandbox bypass inside externally sandboxed Docker worker")
	}
	if !strings.Contains(cmd, "FLYWHEEL_MCP_API_KEY") {
		t.Error("expected exported MCP API key env var")
	}
}

func TestCodexDriverCredentialEnvName(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	if d.CredentialEnvName() != "OPENAI_API_KEY" {
		t.Errorf("expected OPENAI_API_KEY, got %q", d.CredentialEnvName())
	}
}

func TestCodexDriverDefaultAllowedHosts(t *testing.T) {
	d := NewCodexDriver(DriverConfig{})
	hosts := strings.Join(d.DefaultAllowedHosts(), ",")
	if !strings.Contains(hosts, "api.openai.com") {
		t.Errorf("expected OpenAI host allowlist, got %q", hosts)
	}
}

// --- GenericDriver Tests ---

func TestGenericDriverName(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if d.Name() != "generic" {
		t.Errorf("expected name 'generic', got %q", d.Name())
	}
}

func TestGenericDriverExecutable(t *testing.T) {
	d := NewGenericDriver(DriverConfig{CLIPath: "/usr/bin/opencode"})
	if d.Executable() != "/usr/bin/opencode" {
		t.Errorf("expected executable '/usr/bin/opencode', got %q", d.Executable())
	}
}

func TestGenericDriverBuildCLIArgs(t *testing.T) {
	d := NewGenericDriver(DriverConfig{
		CLIPath:   "/usr/bin/opencode",
		ExtraArgs: []string{"--mode", "headless"},
	})
	args := d.BuildCLIArgs("system prompt", "task message", testMCPConnection(), "/tmp/mcp.json")

	if len(args) != 2 {
		t.Fatalf("expected 2 args (extra args only), got %d: %v", len(args), args)
	}
	if args[0] != "--mode" || args[1] != "headless" {
		t.Errorf("expected extra args [--mode headless], got %v", args)
	}
}

func TestGenericDriverBuildDockerCmd(t *testing.T) {
	d := NewGenericDriver(DriverConfig{CLIPath: "opencode"})
	cmd := d.BuildDockerCmd("ticket/t-2", testMCPConnection())

	if !strings.Contains(cmd, "git clone /repo /workspace") {
		t.Error("expected git clone in docker cmd")
	}
	if !strings.Contains(cmd, "git checkout -b ticket/t-2") {
		t.Error("expected branch checkout in docker cmd")
	}
	if !strings.Contains(cmd, "FLYWHEEL_SYSTEM_PROMPT") {
		t.Error("expected FLYWHEEL_SYSTEM_PROMPT export in docker cmd")
	}
	if !strings.Contains(cmd, "WARRANT_SYSTEM_PROMPT") {
		t.Error("expected WARRANT_SYSTEM_PROMPT export in docker cmd")
	}
	if !strings.Contains(cmd, "FLYWHEEL_MCP_URL") {
		t.Error("expected FLYWHEEL_MCP_URL export in docker cmd")
	}
	if !strings.Contains(cmd, "opencode") {
		t.Error("expected agent command in docker cmd")
	}
}

func TestGenericDriverBuildDockerCmdWithArgs(t *testing.T) {
	d := NewGenericDriver(DriverConfig{
		CLIPath:   "aider",
		ExtraArgs: []string{"--yes", "--no-git"},
	})

	cmd := d.BuildDockerCmd("ticket/t-3", testMCPConnection())
	if !strings.Contains(cmd, "aider --yes --no-git") {
		t.Errorf("expected 'aider --yes --no-git' in docker cmd, got:\n%s", cmd)
	}
}

func TestGenericDriverDockerImage(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if d.DockerImage() != "" {
		t.Errorf("expected empty docker image (use default), got %q", d.DockerImage())
	}
}

func TestGenericDriverFormatPrompt(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	input := "# Prompt\n\nDo things."
	if d.FormatPrompt(input) != input {
		t.Error("GenericDriver should pass through prompt unchanged")
	}
}

func TestGenericDriverEnv(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	env := d.Env("system prompt", "task message", testMCPConnection(), "/tmp/mcp.json")

	if len(env.FilterPrefixes) != 0 {
		t.Errorf("expected 0 filter prefixes, got %d", len(env.FilterPrefixes))
	}
	if env.Set["WARRANT_DISPATCH"] != "true" {
		t.Errorf("expected WARRANT_DISPATCH=true, got %q", env.Set["WARRANT_DISPATCH"])
	}
	if env.Set["FLYWHEEL_SYSTEM_PROMPT"] != "system prompt" {
		t.Errorf("expected system prompt to be exported, got %q", env.Set["FLYWHEEL_SYSTEM_PROMPT"])
	}
	if env.Set["FLYWHEEL_MCP_URL"] != "http://localhost:8080/mcp" {
		t.Errorf("expected /mcp URL export, got %q", env.Set["FLYWHEEL_MCP_URL"])
	}
}

func TestGenericDriverResolveCredential(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if d.ResolveCredential("my-key") != "my-key" {
		t.Error("GenericDriver should return static key unchanged")
	}
	if d.ResolveCredential("") != "" {
		t.Error("GenericDriver should return empty when no key provided")
	}
}

func TestGenericDriverCredentialEnvName(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if d.CredentialEnvName() != "" {
		t.Errorf("expected empty credential env name, got %q", d.CredentialEnvName())
	}
}

func TestGenericDriverDefaultAllowedHosts(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if len(d.DefaultAllowedHosts()) != 0 {
		t.Errorf("expected no default allowed hosts, got %v", d.DefaultAllowedHosts())
	}
}

func TestGenericDriverExtraDockerArgs(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if len(d.ExtraDockerArgs()) != 0 {
		t.Error("GenericDriver should return no extra docker args")
	}
}

// --- LookupDriver Tests ---

func TestLookupDriverClaude(t *testing.T) {
	driver, err := LookupDriver("claude", DriverConfig{CLIPath: "/bin/claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver.Name() != "claude" {
		t.Errorf("expected driver name 'claude', got %q", driver.Name())
	}
}

func TestLookupDriverCodex(t *testing.T) {
	driver, err := LookupDriver("codex", DriverConfig{CLIPath: "/opt/homebrew/bin/codex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver.Name() != "codex" {
		t.Errorf("expected driver name 'codex', got %q", driver.Name())
	}
}

func TestLookupDriverGeneric(t *testing.T) {
	driver, err := LookupDriver("generic", DriverConfig{CLIPath: "aider"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver.Name() != "generic" {
		t.Errorf("expected driver name 'generic', got %q", driver.Name())
	}
}

func TestLookupDriverUnknown(t *testing.T) {
	_, err := LookupDriver("nonexistent", DriverConfig{})
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the driver name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "claude, codex, generic") {
		t.Errorf("error should list available drivers, got: %v", err)
	}
}

func TestRegisterDriver(t *testing.T) {
	RegisterDriver("custom-test", func(cfg DriverConfig) AgentDriver {
		return NewGenericDriver(cfg)
	})
	defer delete(driverRegistry, "custom-test")

	driver, err := LookupDriver("custom-test", DriverConfig{CLIPath: "custom-bin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
}
