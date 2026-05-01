package bootstrap

import (
	"os"
	"testing"
)

func TestHeadlessConfigFromEnv(t *testing.T) {
	// Set test env vars.
	t.Setenv("WARRANT_REPO_PATH", "/test/repo")
	t.Setenv("DISPATCH_AGENT_API_KEY", "sk-test")
	t.Setenv("AUTONOMY_MODE", "supervised")
	t.Setenv("WARRANT_DATA_DIR", "/test/data")

	cfg := HeadlessConfigFromEnv()

	if cfg.RepoPath != "/test/repo" {
		t.Errorf("expected /test/repo, got %q", cfg.RepoPath)
	}
	if cfg.DispatchCredential != "sk-test" {
		t.Errorf("expected sk-test, got %q", cfg.DispatchCredential)
	}
	if cfg.AutonomyMode != "supervised" {
		t.Errorf("expected supervised, got %q", cfg.AutonomyMode)
	}
	if cfg.DataDir != "/test/data" {
		t.Errorf("expected /test/data, got %q", cfg.DataDir)
	}
}

func TestHeadlessConfigDefaults(t *testing.T) {
	// Clear relevant env vars.
	t.Setenv("WARRANT_REPO_PATH", "")
	t.Setenv("DISPATCH_AGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("AUTONOMY_MODE", "")
	t.Setenv("WARRANT_DATA_DIR", "")

	cfg := HeadlessConfigFromEnv()

	cwd, _ := os.Getwd()
	if cfg.RepoPath != cwd {
		t.Errorf("expected cwd %q, got %q", cwd, cfg.RepoPath)
	}
	if cfg.AutonomyMode != "sandbox" {
		t.Errorf("expected sandbox default, got %q", cfg.AutonomyMode)
	}
}

func TestHeadlessConfigFromEnvOpenAIKeyFallback(t *testing.T) {
	t.Setenv("WARRANT_REPO_PATH", "")
	t.Setenv("DISPATCH_AGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ANTHROPIC_API_KEY", "sk-anthropic")
	t.Setenv("AUTONOMY_MODE", "")
	t.Setenv("WARRANT_DATA_DIR", "")

	cfg := HeadlessConfigFromEnv()
	if cfg.DispatchCredential != "sk-openai" {
		t.Fatalf("expected OPENAI_API_KEY fallback, got %q", cfg.DispatchCredential)
	}
}

