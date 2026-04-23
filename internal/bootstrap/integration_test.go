package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/embedded"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// TestEndToEndBootstrap validates the complete bootstrap flow:
// 1. Create temp repo with representative files
// 2. Open embedded SQLite database
// 3. Run scanner against repo
// 4. Verify project map produced correctly
// 5. Create org, project, and agent programmatically (simulates wizard without stdin)
// 6. Verify all artifacts exist
func TestEndToEndBootstrap(t *testing.T) {
	ctx := context.Background()

	// 1. Create a mock repo structure.
	repoDir := t.TempDir()
	createMockRepo(t, repoDir)

	// 2. Open embedded database.
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "warrant.db")
	db, err := embedded.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// 3. Run bootstrap scanner.
	pm, err := ScanRepo(repoDir)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}

	// 4. Verify project map.
	if !pm.HasDocker {
		t.Error("expected HasDocker=true")
	}
	if !pm.HasCI {
		t.Error("expected HasCI=true")
	}
	if pm.CISystem != "GitHub Actions" {
		t.Errorf("expected GitHub Actions, got %q", pm.CISystem)
	}
	if !pm.HasTerraform {
		t.Error("expected HasTerraform=true")
	}
	if len(pm.Languages) == 0 {
		t.Fatal("expected at least one language detected")
	}
	if pm.Summary == "" {
		t.Error("expected non-empty summary")
	}

	// 5. Simulate wizard's org/project/agent creation using embedded stores.
	orgStore := embedded.NewOrgStore(db)
	orgSvc := org.NewService(orgStore)
	projStore := embedded.NewProjectStore(db)
	projSvc := project.NewService(projStore)
	agentStore := embedded.NewAgentStore(db)
	agentSvc := agent.NewService(agentStore)

	// Create org.
	newOrg, err := orgSvc.CreateOrg(ctx, "default", "default")
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if newOrg.ID == "" {
		t.Fatal("expected non-empty org ID")
	}

	// Add bootstrap user.
	bootstrapUserID := uuid.Must(uuid.NewV7()).String()
	err = orgSvc.AddMember(ctx, newOrg.ID, bootstrapUserID, org.RoleOwner)
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// Create project from scan results.
	projectName := filepath.Base(repoDir)
	newProj, err := projSvc.CreateProject(ctx, newOrg.ID, projectName, "", repoDir, pm.TechStack)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if newProj.ID == "" {
		t.Fatal("expected non-empty project ID")
	}

	// Update context pack with key files.
	if len(pm.KeyFiles) > 0 {
		var keyFileRefs []project.FileRef
		for _, kf := range pm.KeyFiles {
			keyFileRefs = append(keyFileRefs, project.FileRef{Path: kf})
		}
		pack := project.ContextPack{
			Conventions: "Auto-detected: " + pm.Summary,
			KeyFiles:    keyFileRefs,
		}
		err = projSvc.UpdateContextPack(ctx, newProj.ID, pack)
		if err != nil {
			t.Fatalf("UpdateContextPack: %v", err)
		}
	}

	// Register agent.
	newAgent, apiKey, err := agentSvc.RegisterAgent(ctx, "bootstrap-agent", agent.TypeClaude)
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if newAgent.ID == "" || apiKey == "" {
		t.Fatal("expected non-empty agent ID and API key")
	}

	// 6. Verify artifacts.
	// Verify org retrieval.
	gotOrg, err := orgStore.GetBySlug(ctx, "default")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if gotOrg.Name != "default" {
		t.Errorf("expected org name 'default', got %q", gotOrg.Name)
	}

	// Verify project retrieval.
	gotProj, err := projStore.GetByID(ctx, newProj.ID)
	if err != nil {
		t.Fatalf("GetByID project: %v", err)
	}
	if gotProj.RepoURL != repoDir {
		t.Errorf("expected repo_url %q, got %q", repoDir, gotProj.RepoURL)
	}
	if len(gotProj.TechStack) == 0 {
		t.Error("expected non-empty tech stack")
	}

	// Verify agent retrieval.
	gotAgent, err := agentStore.GetByAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("GetByAPIKey: %v", err)
	}
	if gotAgent.Name != "bootstrap-agent" {
		t.Errorf("expected agent name 'bootstrap-agent', got %q", gotAgent.Name)
	}

	t.Logf("Bootstrap complete: org=%s project=%s agent=%s", newOrg.ID, newProj.ID, newAgent.ID)
	t.Logf("Tech stack: %v", pm.TechStack)
	t.Logf("Summary: %s", pm.Summary)
}

// TestIsFirstRun verifies first-run detection logic.
func TestIsFirstRun(t *testing.T) {
	// Empty directory = first run.
	emptyDir := t.TempDir()
	if !IsFirstRun(emptyDir) {
		t.Error("expected IsFirstRun=true for empty dir")
	}

	// Non-existent directory = first run.
	if !IsFirstRun("/tmp/nonexistent-warrant-test-" + uuid.New().String()) {
		t.Error("expected IsFirstRun=true for nonexistent dir")
	}

	// Directory with warrant.db = not first run.
	withDB := t.TempDir()
	dbPath := filepath.Join(withDB, "warrant.db")
	if err := os.WriteFile(dbPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsFirstRun(withDB) {
		t.Error("expected IsFirstRun=false when warrant.db exists")
	}
}

// TestWriteConfigFile verifies config file generation.
func TestWriteConfigFile(t *testing.T) {
	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "config.env")

	result := &WizardResult{
		RepoPath:           "/home/user/project",
		DispatchCredential: "sk-ant-test123",
		AutonomyMode:       "sandbox",
		ProjectMap:         &ProjectMap{Summary: "Go project with Docker"},
		OrgID:              "org-123",
		ProjectID:          "proj-456",
		AgentID:            "agent-789",
		AgentAPIKey:        "wf_testapikey",
		JWTSecret:          "secret123",
		DataDir:            dataDir,
	}

	err := writeConfigFile(configPath, result)
	if err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	// Verify config file exists and has expected content.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	expectations := []string{
		"STORAGE_MODE=embedded",
		"JWT_SECRET=secret123",
		"DISPATCH_PROJECT_ID=proj-456",
		"DISPATCH_API_KEY=wf_testapikey",
		"DISPATCH_AGENT_API_KEY=sk-ant-test123",
		"AUTONOMY_MODE=sandbox",
	}
	for _, exp := range expectations {
		if !contains(content, exp) {
			t.Errorf("config missing %q", exp)
		}
	}

	// Verify project map JSON was also written.
	pmPath := filepath.Join(dataDir, "project-map.json")
	if _, err := os.Stat(pmPath); os.IsNotExist(err) {
		t.Error("expected project-map.json to be written")
	}
}

// TestScanRepoKubernetes verifies K8s manifest detection.
func TestScanRepoKubernetes(t *testing.T) {
	tmp := t.TempDir()
	// Create a K8s manifest.
	k8sDir := filepath.Join(tmp, "k8s")
	os.MkdirAll(k8sDir, 0o755)
	writeFile(t, tmp, "k8s/deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
`)

	pm, err := ScanRepo(tmp)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	if !pm.HasK8s {
		t.Error("expected HasK8s=true")
	}
}

// TestScanRepoTerraform verifies Terraform detection.
func TestScanRepoTerraform(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "infra/main.tf", `provider "aws" { region = "us-east-1" }`)

	pm, err := ScanRepo(tmp)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	if !pm.HasTerraform {
		t.Error("expected HasTerraform=true")
	}
}

// TestBootstrapTiming verifies that the bootstrap scan completes quickly
// (targeting well under the 10-minute total budget).
func TestBootstrapTiming(t *testing.T) {
	// Create a moderately complex repo (simulates a real project).
	tmp := t.TempDir()
	createMockRepo(t, tmp)
	// Add some extra depth to simulate larger repos.
	for i := 0; i < 10; i++ {
		dir := filepath.Join(tmp, "pkg", string(rune('a'+i)))
		os.MkdirAll(dir, 0o755)
		writeFile(t, tmp, filepath.Join("pkg", string(rune('a'+i)), "main.go"), "package "+string(rune('a'+i)))
	}

	start := time.Now()
	pm, err := ScanRepo(tmp)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}
	if pm == nil {
		t.Fatal("expected non-nil project map")
	}

	// Scan should complete in well under 5 seconds even for moderate repos.
	if elapsed > 5*time.Second {
		t.Errorf("scan took %v, expected < 5s", elapsed)
	}
	t.Logf("scan completed in %v", elapsed)
}

// createMockRepo builds a realistic test repository structure.
func createMockRepo(t *testing.T, dir string) {
	t.Helper()
	// Go backend
	writeFile(t, dir, "go.mod", "module example.com/myapp\ngo 1.21\n")
	writeFile(t, dir, "cmd/server/main.go", "package main\nfunc main() {}\n")
	writeFile(t, dir, "internal/handler/handler.go", "package handler\n")
	writeFile(t, dir, "Makefile", "build:\n\tgo build ./cmd/server\n")

	// TypeScript frontend
	writeFile(t, dir, "web/package.json", `{
		"name": "myapp-web",
		"dependencies": {"react": "^18", "vite": "^5", "tailwindcss": "^3"}
	}`)
	writeFile(t, dir, "web/src/App.tsx", "export default function App() { return <div/>; }")

	// Docker
	writeFile(t, dir, "Dockerfile", "FROM golang:1.21\nCOPY . .\nRUN go build ./cmd/server\n")
	writeFile(t, dir, "docker-compose.yml", "services:\n  app:\n    build: .\n")

	// CI
	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	writeFile(t, dir, ".github/workflows/ci.yml", "name: CI\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n")

	// Terraform
	os.MkdirAll(filepath.Join(dir, "infra"), 0o755)
	writeFile(t, dir, "infra/main.tf", `provider "aws" { region = "us-east-1" }`)

	// .env
	writeFile(t, dir, ".env.example", "PORT=8080\nDATABASE_URL=postgres://...\n")

	// README
	writeFile(t, dir, "README.md", "# My App\n")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
