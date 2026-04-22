// Package bootstrap provides the first-run experience: repository scanning,
// project map generation, and interactive setup wizard.
package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectMap is the result of scanning a repository. It provides a draft
// overview of the project's technology stack and structure, which the operator
// reviews and accepts during first-run setup.
type ProjectMap struct {
	RepoPath    string            `json:"repo_path"`
	Languages   []string          `json:"languages,omitempty"`
	Frameworks  []string          `json:"frameworks,omitempty"`
	HasDocker   bool              `json:"has_docker"`
	HasK8s      bool              `json:"has_k8s"`
	HasTerraform bool             `json:"has_terraform"`
	HasCI       bool              `json:"has_ci"`
	CISystem    string            `json:"ci_system,omitempty"`
	HasEnvFile  bool              `json:"has_env_file"`
	KeyFiles    []string          `json:"key_files,omitempty"`
	TechStack   []string          `json:"tech_stack,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Detections  map[string]string `json:"detections,omitempty"`
}

// ScanRepo walks the given directory and produces a draft ProjectMap by
// detecting Dockerfiles, Kubernetes manifests, Terraform configs, CI configs,
// language files, and other common patterns.
func ScanRepo(repoPath string) (*ProjectMap, error) {
	pm := &ProjectMap{
		RepoPath:   repoPath,
		Detections: make(map[string]string),
	}

	langSet := make(map[string]bool)
	frameworkSet := make(map[string]bool)

	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}

		// Skip common non-source directories.
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "node_modules", "vendor", ".terraform", "__pycache__", ".venv", "venv", "dist", "build", "target":
				return filepath.SkipDir
			}
			// Kubernetes directory patterns.
			switch name {
			case "k8s", "kubernetes", "deploy", "manifests":
				pm.HasK8s = true
				pm.Detections["k8s_dir"] = relPath(repoPath, path)
			}
			return nil
		}

		rel := relPath(repoPath, path)

		// Docker detection.
		if name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile.") || name == "docker-compose.yml" || name == "docker-compose.yaml" || name == "compose.yml" || name == "compose.yaml" {
			pm.HasDocker = true
			pm.Detections["docker"] = rel
		}

		// Kubernetes detection.
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			if isK8sManifest(path) {
				pm.HasK8s = true
				pm.Detections["k8s_manifest"] = rel
			}
		}

		// Terraform detection.
		if strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json") {
			pm.HasTerraform = true
			pm.Detections["terraform"] = rel
		}

		// CI detection.
		switch {
		case rel == ".github/workflows" || strings.HasPrefix(rel, ".github/workflows/"):
			pm.HasCI = true
			pm.CISystem = "GitHub Actions"
			pm.Detections["ci"] = rel
		case rel == ".gitlab-ci.yml":
			pm.HasCI = true
			pm.CISystem = "GitLab CI"
			pm.Detections["ci"] = rel
		case rel == "Jenkinsfile":
			pm.HasCI = true
			pm.CISystem = "Jenkins"
			pm.Detections["ci"] = rel
		case rel == ".circleci/config.yml":
			pm.HasCI = true
			pm.CISystem = "CircleCI"
			pm.Detections["ci"] = rel
		case rel == ".travis.yml":
			pm.HasCI = true
			pm.CISystem = "Travis CI"
			pm.Detections["ci"] = rel
		}

		// .env detection.
		if name == ".env" || name == ".env.example" || name == ".env.local" {
			pm.HasEnvFile = true
			pm.Detections["env_file"] = rel
		}

		// Language and framework detection.
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".go":
			langSet["Go"] = true
		case ".py":
			langSet["Python"] = true
		case ".js":
			langSet["JavaScript"] = true
		case ".ts", ".tsx":
			langSet["TypeScript"] = true
		case ".rs":
			langSet["Rust"] = true
		case ".java":
			langSet["Java"] = true
		case ".rb":
			langSet["Ruby"] = true
		case ".cs":
			langSet["C#"] = true
		case ".php":
			langSet["PHP"] = true
		case ".swift":
			langSet["Swift"] = true
		case ".kt":
			langSet["Kotlin"] = true
		}

		// Framework / tool detection by filename.
		switch name {
		case "go.mod":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "package.json":
			langSet["JavaScript"] = true
			pm.KeyFiles = append(pm.KeyFiles, rel)
			detectNodeFrameworks(path, frameworkSet)
		case "requirements.txt", "setup.py", "pyproject.toml", "Pipfile":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "Cargo.toml":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "pom.xml", "build.gradle", "build.gradle.kts":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "Gemfile":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "Makefile":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		case "CLAUDE.md", "README.md":
			pm.KeyFiles = append(pm.KeyFiles, rel)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Collect languages.
	for lang := range langSet {
		pm.Languages = append(pm.Languages, lang)
	}
	for fw := range frameworkSet {
		pm.Frameworks = append(pm.Frameworks, fw)
	}

	// Build tech stack summary.
	pm.TechStack = buildTechStack(pm)
	pm.Summary = buildSummary(pm)

	return pm, nil
}

// relPath returns the relative path from base to path.
func relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// isK8sManifest does a quick heuristic check for Kubernetes manifests.
func isK8sManifest(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 { // skip files > 1MB
		return false
	}
	content := string(data)
	// Look for common K8s resource markers.
	return (strings.Contains(content, "apiVersion:") && strings.Contains(content, "kind:")) ||
		strings.Contains(content, "apiVersion: apps/v1") ||
		strings.Contains(content, "kind: Deployment") ||
		strings.Contains(content, "kind: Service") ||
		strings.Contains(content, "kind: StatefulSet")
}

// detectNodeFrameworks reads package.json dependencies to detect frameworks.
func detectNodeFrameworks(pkgPath string, frameworks map[string]bool) {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "\"react\"") {
		frameworks["React"] = true
	}
	if strings.Contains(content, "\"next\"") {
		frameworks["Next.js"] = true
	}
	if strings.Contains(content, "\"vue\"") {
		frameworks["Vue"] = true
	}
	if strings.Contains(content, "\"express\"") {
		frameworks["Express"] = true
	}
	if strings.Contains(content, "\"fastify\"") {
		frameworks["Fastify"] = true
	}
	if strings.Contains(content, "\"vite\"") {
		frameworks["Vite"] = true
	}
	if strings.Contains(content, "\"tailwindcss\"") {
		frameworks["Tailwind CSS"] = true
	}
}

// buildTechStack produces a flat list for project.TechStack.
func buildTechStack(pm *ProjectMap) []string {
	var stack []string
	stack = append(stack, pm.Languages...)
	stack = append(stack, pm.Frameworks...)
	if pm.HasDocker {
		stack = append(stack, "Docker")
	}
	if pm.HasK8s {
		stack = append(stack, "Kubernetes")
	}
	if pm.HasTerraform {
		stack = append(stack, "Terraform")
	}
	if pm.HasCI && pm.CISystem != "" {
		stack = append(stack, pm.CISystem)
	}
	return stack
}

// buildSummary produces a one-line project description.
func buildSummary(pm *ProjectMap) string {
	var parts []string
	if len(pm.Languages) > 0 {
		parts = append(parts, strings.Join(pm.Languages, "/")+" project")
	}
	if len(pm.Frameworks) > 0 {
		parts = append(parts, "using "+strings.Join(pm.Frameworks, ", "))
	}
	if pm.HasDocker {
		parts = append(parts, "with Docker")
	}
	if pm.HasK8s {
		parts = append(parts, "with Kubernetes")
	}
	if pm.HasTerraform {
		parts = append(parts, "with Terraform")
	}
	if len(parts) == 0 {
		return "project at " + pm.RepoPath
	}
	return strings.Join(parts, " ")
}
