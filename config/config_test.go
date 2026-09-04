package config

import (
	"os/exec"
	"strings"
	"testing"
)

func TestValidate_ClaudePathNotFound(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:    true,
			ClaudePath: "nonexistent-binary-that-should-not-exist-on-path",
		},
	}
	warnings := cfg.Validate()

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "DISPATCH_CLAUDE_PATH") && strings.Contains(w, "not found on PATH") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about DISPATCH_CLAUDE_PATH not on PATH, got: %v", warnings)
	}
}

func TestValidate_ClaudePathFound(t *testing.T) {
	// "sh" should exist on any Unix-like system.
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH; cannot test positive case")
	}
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:    true,
			ClaudePath: "sh",
		},
	}
	warnings := cfg.Validate()

	for _, w := range warnings {
		if strings.Contains(w, "DISPATCH_CLAUDE_PATH") {
			t.Errorf("unexpected DISPATCH_CLAUDE_PATH warning: %s", w)
		}
	}
}

func TestValidate_GenericDriverPathFound(t *testing.T) {
	// "sh" should exist on any Unix-like system.
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH; cannot test positive case")
	}
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:      true,
			AgentDriver:  "generic",
			AgentCLIPath: "sh",
		},
	}
	warnings := cfg.Validate()

	for _, w := range warnings {
		if strings.Contains(w, "DISPATCH_AGENT_CMD") {
			t.Errorf("unexpected DISPATCH_AGENT_CMD warning: %s", w)
		}
	}
}

func TestValidate_InvalidDriver(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:     true,
			AgentRunner: dispatchRunnerCLIForTest(),
			AgentDriver: "not-a-real-driver",
		},
	}
	warnings := cfg.Validate()

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "DISPATCH_AGENT_DRIVER") && strings.Contains(w, "not-a-real-driver") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about invalid DISPATCH_AGENT_DRIVER, got: %v", warnings)
	}
}

func TestValidate_ClaudePathSkippedWhenDispatchDisabled(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:    false,
			ClaudePath: "nonexistent-binary-that-should-not-exist-on-path",
		},
	}
	warnings := cfg.Validate()

	for _, w := range warnings {
		if strings.Contains(w, "DISPATCH_CLAUDE_PATH") {
			t.Errorf("should not warn about DISPATCH_CLAUDE_PATH when dispatch disabled, got: %s", w)
		}
	}
}

func TestValidate_AutoApproveConflict(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			AutoApproveOnAcceptancePass: true,
		},
		RunAcceptanceTestOnSubmit: false,
	}
	warnings := cfg.Validate()

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "AUTO_APPROVE_ON_ACCEPTANCE_PASS") &&
			strings.Contains(w, "RUN_ACCEPTANCE_TEST_ON_SUBMIT") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about conflicting auto-approve settings, got: %v", warnings)
	}
}

func TestValidate_InvalidRunner(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			Enabled:     true,
			AgentRunner: "not-a-runner",
		},
	}
	warnings := cfg.Validate()

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "unknown agent runner") && strings.Contains(w, "not-a-runner") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about invalid DISPATCH_AGENT_RUNNER, got: %v", warnings)
	}
}

func TestResolveDispatchAgentAPIKey_ClaudeUsesAnthropicFallback(t *testing.T) {
	t.Setenv("DISPATCH_AGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ANTHROPIC_API_KEY", "sk-anthropic")

	got := resolveAgentAPIKey("DISPATCH_AGENT_API_KEY", "claude")
	if got != "sk-anthropic" {
		t.Fatalf("expected ANTHROPIC_API_KEY fallback for claude, got %q", got)
	}
}

func TestValidate_AutoApproveNoConflict(t *testing.T) {
	cfg := &Config{
		Dispatch: DispatchConfig{
			AutoApproveOnAcceptancePass: true,
		},
		RunAcceptanceTestOnSubmit: true,
	}
	warnings := cfg.Validate()

	for _, w := range warnings {
		if strings.Contains(w, "AUTO_APPROVE_ON_ACCEPTANCE_PASS") {
			t.Errorf("unexpected auto-approve warning when acceptance test enabled: %s", w)
		}
	}
}

func TestValidate_NoWarningsOnDefaults(t *testing.T) {
	cfg := &Config{}
	warnings := cfg.Validate()

	if len(warnings) != 0 {
		t.Errorf("expected no warnings on default config, got: %v", warnings)
	}
}

func dispatchRunnerCLIForTest() string { return "cli" }
