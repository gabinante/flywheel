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

func TestClaudeDriverBuildCLIArgsStreamsStructuredOutput(t *testing.T) {
	d := NewClaudeDriver(DriverConfig{})
	args := d.BuildCLIArgs("system prompt here", "do the task", testMCPConnection(), "/tmp/mcp.json")

	idx := func(want string) int {
		for i, a := range args {
			if a == want {
				return i
			}
		}
		t.Fatalf("expected %q in args %q", want, args)
		return -1
	}
	if i := idx("--output-format"); args[i+1] != "stream-json" {
		t.Fatalf("expected --output-format stream-json, got %q", args[i+1])
	}
	idx("--verbose")
	if idx("do the task") > idx("--mcp-config") {
		t.Fatal("task message must precede --mcp-config, which is variadic and would swallow it")
	}
	if _, ok := AgentDriver(d).(OutputParsingDriver); !ok {
		t.Fatal("ClaudeDriver should provide an OutputParser for its stream-json output")
	}
}
