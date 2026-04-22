package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/embedded"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

func TestHeadlessConfigFromEnv(t *testing.T) {
	// Set test env vars.
	t.Setenv("WARRANT_REPO_PATH", "/test/repo")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("AUTONOMY_MODE", "supervised")
	t.Setenv("WARRANT_DATA_DIR", "/test/data")

	cfg := HeadlessConfigFromEnv()

	if cfg.RepoPath != "/test/repo" {
		t.Errorf("expected /test/repo, got %q", cfg.RepoPath)
	}
	if cfg.AnthropicKey != "sk-test" {
		t.Errorf("expected sk-test, got %q", cfg.AnthropicKey)
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

func TestRunHeadless(t *testing.T) {
	ctx := context.Background()

	// Create a mock repo.
	repoDir := t.TempDir()
	createMockRepo(t, repoDir)

	// Create embedded database.
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "warrant.db")
	db, err := embedded.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// Create services.
	orgStore := embedded.NewOrgStore(db)
	orgSvc := org.NewService(orgStore)
	projStore := embedded.NewProjectStore(db)
	projSvc := project.NewService(projStore)
	agentStore := embedded.NewAgentStore(db)
	agentSvc := agent.NewService(agentStore)

	// Run headless bootstrap.
	cfg := &HeadlessConfig{
		RepoPath:     repoDir,
		AnthropicKey: "sk-test-key",
		AutonomyMode: "sandbox",
		DataDir:      dataDir,
	}

	result, err := RunHeadless(ctx, cfg, orgSvc, projSvc, agentSvc)
	if err != nil {
		t.Fatalf("RunHeadless: %v", err)
	}

	// Verify result.
	if result.OrgID == "" {
		t.Error("expected non-empty OrgID")
	}
	if result.ProjectID == "" {
		t.Error("expected non-empty ProjectID")
	}
	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	if result.AgentAPIKey == "" {
		t.Error("expected non-empty AgentAPIKey")
	}
	if result.JWTSecret == "" {
		t.Error("expected non-empty JWTSecret")
	}
	if result.ProjectMap == nil {
		t.Fatal("expected non-nil ProjectMap")
	}
	if !result.ProjectMap.HasDocker {
		t.Error("expected HasDocker=true in project map")
	}

	// Verify config file was written.
	configPath := filepath.Join(dataDir, "config.env")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected config.env to be written")
	}

	// Verify project map JSON was written.
	pmPath := filepath.Join(dataDir, "project-map.json")
	if _, err := os.Stat(pmPath); os.IsNotExist(err) {
		t.Error("expected project-map.json to be written")
	}

	t.Logf("Headless bootstrap complete: org=%s project=%s", result.OrgID, result.ProjectID)
}
