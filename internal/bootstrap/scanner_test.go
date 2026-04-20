package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRepo(t *testing.T) {
	// Create a temporary repo structure.
	tmp := t.TempDir()
	// Create files.
	writeFile(t, tmp, "go.mod", "module example.com/test\ngo 1.21\n")
	writeFile(t, tmp, "main.go", "package main\nfunc main() {}\n")
	writeFile(t, tmp, "Dockerfile", "FROM golang:1.21\n")
	writeFile(t, tmp, "Makefile", "build:\n\tgo build\n")
	writeFile(t, tmp, ".env.example", "PORT=8080\n")
	writeFile(t, tmp, "README.md", "# Test Project\n")

	// Create CI directory.
	os.MkdirAll(filepath.Join(tmp, ".github", "workflows"), 0o755)
	writeFile(t, tmp, ".github/workflows/ci.yml", "name: CI\non: push\n")

	// Create a package.json with React.
	writeFile(t, tmp, "web/package.json", `{"dependencies":{"react":"^18","vite":"^5","tailwindcss":"^3"}}`)
	writeFile(t, tmp, "web/src/App.tsx", "export default function App() { return <div/>; }")

	pm, err := ScanRepo(tmp)
	if err != nil {
		t.Fatalf("ScanRepo: %v", err)
	}

	// Verify detections.
	if !pm.HasDocker {
		t.Error("expected HasDocker=true")
	}
	if !pm.HasCI {
		t.Error("expected HasCI=true")
	}
	if pm.CISystem != "GitHub Actions" {
		t.Errorf("expected CISystem=GitHub Actions, got %q", pm.CISystem)
	}
	if !pm.HasEnvFile {
		t.Error("expected HasEnvFile=true")
	}
	if len(pm.Languages) == 0 {
		t.Error("expected at least one language")
	}

	// Check tech stack includes Go.
	hasGo := false
	for _, lang := range pm.Languages {
		if lang == "Go" {
			hasGo = true
		}
	}
	if !hasGo {
		t.Errorf("expected Go in languages, got %v", pm.Languages)
	}

	// Check frameworks detected.
	hasReact := false
	for _, fw := range pm.Frameworks {
		if fw == "React" {
			hasReact = true
		}
	}
	if !hasReact {
		t.Errorf("expected React in frameworks, got %v", pm.Frameworks)
	}

	// Check key files detected.
	if len(pm.KeyFiles) == 0 {
		t.Error("expected key files")
	}

	// Check summary is non-empty.
	if pm.Summary == "" {
		t.Error("expected non-empty summary")
	}

	t.Logf("ProjectMap: languages=%v frameworks=%v docker=%v k8s=%v terraform=%v ci=%s",
		pm.Languages, pm.Frameworks, pm.HasDocker, pm.HasK8s, pm.HasTerraform, pm.CISystem)
	t.Logf("Summary: %s", pm.Summary)
	t.Logf("TechStack: %v", pm.TechStack)
}

func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, rel)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
