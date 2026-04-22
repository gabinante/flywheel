package dispatch

import (
	"strings"
	"testing"
)

// --- ClaudeDriver Tests ---

func TestClaudeDriverName(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	if d.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", d.Name())
	}
}

func TestClaudeDriverBuildCLIArgs(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{CLIPath: "/usr/local/bin/claude"})

	exe, args := d.BuildCLIArgs("system prompt here", "do the task", "/tmp/mcp.json")

	if exe != "/usr/local/bin/claude" {
		t.Errorf("expected exe '/usr/local/bin/claude', got %q", exe)
	}

	// Verify expected flags are present.
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

func TestClaudeDriverBuildCLIArgsDefaultPath(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{}) // empty CLIPath

	exe, _ := d.BuildCLIArgs("", "", "")
	if exe != "claude" {
		t.Errorf("expected default exe 'claude', got %q", exe)
	}
}

func TestClaudeDriverBuildDockerCmd(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})

	cmd := d.BuildDockerCmd("ticket/t-1")

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
	env := d.Env()

	// Should filter CLAUDECODE and ANTHROPIC_API_KEY.
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

	// Should set CLAUDE_CODE_ENTRYPOINT.
	if env.Set["CLAUDE_CODE_ENTRYPOINT"] != "warrant-dispatch" {
		t.Errorf("expected CLAUDE_CODE_ENTRYPOINT=warrant-dispatch, got %q", env.Set["CLAUDE_CODE_ENTRYPOINT"])
	}
}

func TestClaudeDriverResolveCredentialFallback(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	// On non-macOS or without keychain, should fall back to static key.
	result := d.ResolveCredential("sk-test-key")
	// We can't guarantee OAuth will work in test, but it should at least return the static key.
	if result == "" {
		t.Error("expected non-empty credential (at least the static key)")
	}
}

func TestClaudeDriverExtraDockerArgs(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	args := d.ExtraDockerArgs()

	// Should contain the .claude volume mount.
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

// --- GenericDriver Tests ---

func TestGenericDriverName(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	if d.Name() != "generic" {
		t.Errorf("expected name 'generic', got %q", d.Name())
	}
}

func TestGenericDriverBuildCLIArgs(t *testing.T) {
	d := NewGenericDriver(DriverConfig{
		CLIPath:   "/usr/bin/opencode",
		ExtraArgs: []string{"--mode", "headless"},
	})

	exe, args := d.BuildCLIArgs("system prompt", "task message", "/tmp/mcp.json")

	if exe != "/usr/bin/opencode" {
		t.Errorf("expected exe '/usr/bin/opencode', got %q", exe)
	}
	// Generic driver only passes extra args, no agent-specific flags.
	if len(args) != 2 {
		t.Fatalf("expected 2 args (extra args only), got %d: %v", len(args), args)
	}
	if args[0] != "--mode" || args[1] != "headless" {
		t.Errorf("expected extra args [--mode headless], got %v", args)
	}
}

func TestGenericDriverBuildCLIArgsDefault(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})

	exe, args := d.BuildCLIArgs("", "", "")
	if exe != "agent" {
		t.Errorf("expected default exe 'agent', got %q", exe)
	}
	if len(args) != 0 {
		t.Errorf("expected 0 args with no extra args, got %d", len(args))
	}
}

func TestGenericDriverBuildDockerCmd(t *testing.T) {
	d := NewGenericDriver(DriverConfig{CLIPath: "opencode"})

	cmd := d.BuildDockerCmd("ticket/t-2")

	if !strings.Contains(cmd, "git clone /repo /workspace") {
		t.Error("expected git clone in docker cmd")
	}
	if !strings.Contains(cmd, "git checkout -b ticket/t-2") {
		t.Error("expected branch checkout in docker cmd")
	}
	if !strings.Contains(cmd, "WARRANT_SYSTEM_PROMPT") {
		t.Error("expected WARRANT_SYSTEM_PROMPT export in docker cmd")
	}
	if !strings.Contains(cmd, "WARRANT_TASK_MESSAGE") {
		t.Error("expected WARRANT_TASK_MESSAGE export in docker cmd")
	}
	if !strings.Contains(cmd, "WARRANT_MCP_CONFIG_PATH") {
		t.Error("expected WARRANT_MCP_CONFIG_PATH export in docker cmd")
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

	cmd := d.BuildDockerCmd("ticket/t-3")
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
	env := d.Env()

	// Generic driver should not filter any parent env vars.
	if len(env.FilterPrefixes) != 0 {
		t.Errorf("expected 0 filter prefixes, got %d", len(env.FilterPrefixes))
	}

	// Should set WARRANT_DISPATCH marker.
	if env.Set["WARRANT_DISPATCH"] != "true" {
		t.Errorf("expected WARRANT_DISPATCH=true, got %q", env.Set["WARRANT_DISPATCH"])
	}
}

func TestGenericDriverResolveCredential(t *testing.T) {
	d := NewGenericDriver(DriverConfig{})
	// Should pass through the static key unchanged.
	if d.ResolveCredential("my-key") != "my-key" {
		t.Error("GenericDriver should return static key unchanged")
	}
	if d.ResolveCredential("") != "" {
		t.Error("GenericDriver should return empty when no key provided")
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
}

func TestRegisterDriver(t *testing.T) {
	// Register a custom driver.
	RegisterDriver("custom-test", func(cfg DriverConfig) AgentDriver {
		return NewGenericDriver(cfg) // reuse generic as a stand-in
	})
	defer delete(driverRegistry, "custom-test") // cleanup

	driver, err := LookupDriver("custom-test", DriverConfig{CLIPath: "custom-bin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
}
