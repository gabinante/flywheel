package catalog

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Scanner discovers entities from common project artifacts:
// Dockerfiles, docker-compose.yml, Kubernetes manifests, Terraform files,
// and varlock .env.schema files.
type Scanner struct{}

// NewScanner returns a new bootstrap scanner.
func NewScanner() *Scanner { return &Scanner{} }

// ScanRepo scans a repository root and returns discovered entities and edges.
// All results are marked as "observed" since they are system-derived.
func (s *Scanner) ScanRepo(repoPath string) (*ScanResult, error) {
	result := &ScanResult{}

	// Walk once, dispatch to scanners by filename.
	_ = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			// Skip hidden directories and common non-project dirs.
			if info != nil && info.IsDir() {
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(repoPath, path)
		name := info.Name()

		switch {
		case name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile."):
			s.scanDockerfile(rel, result)
		case name == "docker-compose.yml" || name == "docker-compose.yaml":
			s.scanDockerCompose(path, rel, result)
		case strings.HasSuffix(name, ".tf"):
			s.scanTerraform(path, rel, result)
		case name == ".env.schema":
			s.scanVarlockSchema(path, rel, result)
		case isKubernetesManifest(name, path):
			s.scanKubernetesManifest(path, rel, result)
		}
		return nil
	})

	return result, nil
}

// scanDockerfile discovers a service from a Dockerfile path.
func (s *Scanner) scanDockerfile(relPath string, result *ScanResult) {
	// Derive service name from directory (e.g. "services/api/Dockerfile" → "api").
	dir := filepath.Dir(relPath)
	name := filepath.Base(dir)
	if dir == "." {
		name = "main"
	}
	// Check for Dockerfile.suffix pattern.
	base := filepath.Base(relPath)
	if strings.HasPrefix(base, "Dockerfile.") {
		suffix := strings.TrimPrefix(base, "Dockerfile.")
		if suffix != "" {
			name = name + "-" + suffix
		}
	}

	result.Entities = append(result.Entities, Entity{
		Type:        EntityService,
		Name:        name,
		Description: "Service discovered from " + relPath,
		Labels:      map[string]string{"discovered_from": "dockerfile"},
		Metadata:    map[string]string{"dockerfile": relPath},
		Source:      SourceObserved,
	})
}

// scanDockerCompose discovers services and datastores from docker-compose files.
func (s *Scanner) scanDockerCompose(absPath, relPath string, result *ScanResult) {
	f, err := os.Open(absPath)
	if err != nil {
		result.Errors = append(result.Errors, "docker-compose open: "+err.Error())
		return
	}
	defer f.Close()

	// Simple line-based YAML scanning for service names under "services:".
	scanner := bufio.NewScanner(f)
	inServices := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect top-level "services:" key.
		if trimmed == "services:" {
			inServices = true
			continue
		}
		// Another top-level key ends the services block.
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' && trimmed != "" {
			inServices = false
			continue
		}

		if !inServices {
			continue
		}

		// Service names are indented exactly 2 spaces and end with ":".
		if len(line) > 2 && line[0] == ' ' && line[1] == ' ' && line[2] != ' ' && strings.HasSuffix(trimmed, ":") {
			svcName := strings.TrimSuffix(trimmed, ":")
			entityType := classifyComposeService(svcName)
			result.Entities = append(result.Entities, Entity{
				Type:        entityType,
				Name:        svcName,
				Description: "Discovered from " + relPath,
				Labels:      map[string]string{"discovered_from": "docker-compose"},
				Metadata:    map[string]string{"compose_file": relPath},
				Source:      SourceObserved,
			})
		}
	}
}

// classifyComposeService guesses entity type from common service naming patterns.
func classifyComposeService(name string) EntityType {
	lower := strings.ToLower(name)
	datastoreKeywords := []string{"postgres", "mysql", "redis", "mongo", "elasticsearch", "kafka", "rabbit", "memcached", "nats", "minio", "db", "database"}
	for _, kw := range datastoreKeywords {
		if strings.Contains(lower, kw) {
			return EntityDatastore
		}
	}
	infraKeywords := []string{"nginx", "traefik", "envoy", "haproxy", "vault", "consul"}
	for _, kw := range infraKeywords {
		if strings.Contains(lower, kw) {
			return EntityInfrastructure
		}
	}
	return EntityService
}

// scanTerraform discovers infrastructure entities from Terraform files.
func (s *Scanner) scanTerraform(absPath, relPath string, result *ScanResult) {
	f, err := os.Open(absPath)
	if err != nil {
		result.Errors = append(result.Errors, "terraform open: "+err.Error())
		return
	}
	defer f.Close()

	// Scan for resource blocks: resource "aws_instance" "name" {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "resource ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		resourceType := strings.Trim(parts[1], "\"")
		resourceName := strings.Trim(parts[2], "\"")

		result.Entities = append(result.Entities, Entity{
			Type:        EntityInfrastructure,
			Name:        resourceName,
			Description: "Terraform resource " + resourceType,
			Labels:      map[string]string{"discovered_from": "terraform", "resource_type": resourceType},
			Metadata:    map[string]string{"tf_file": relPath, "tf_resource_type": resourceType},
			Source:      SourceObserved,
		})
	}
}

// scanVarlockSchema discovers integrations from varlock .env.schema files.
func (s *Scanner) scanVarlockSchema(absPath, relPath string, result *ScanResult) {
	f, err := os.Open(absPath)
	if err != nil {
		result.Errors = append(result.Errors, "varlock schema open: "+err.Error())
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse "VAR_NAME= # @optional @sensitive Description"
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 1 {
			continue
		}
		varName := strings.TrimSpace(parts[0])
		// Look for patterns indicating external service integrations.
		if isIntegrationVar(varName) {
			integrationName := guessIntegrationName(varName)
			result.Entities = append(result.Entities, Entity{
				Type:        EntityIntegration,
				Name:        integrationName,
				Description: "Integration discovered from " + relPath + " (" + varName + ")",
				Labels:      map[string]string{"discovered_from": "varlock", "env_var": varName},
				Metadata:    map[string]string{"schema_file": relPath, "env_var": varName},
				Source:      SourceObserved,
			})
		}
	}
}

// isIntegrationVar checks if an env var name suggests an external integration.
func isIntegrationVar(name string) bool {
	keywords := []string{"_API_KEY", "_TOKEN", "_SECRET", "_URL", "_ENDPOINT", "_DSN", "_CONNECTION"}
	upper := strings.ToUpper(name)
	for _, kw := range keywords {
		if strings.HasSuffix(upper, kw) {
			return true
		}
	}
	return false
}

// guessIntegrationName extracts a human-readable integration name from an env var.
func guessIntegrationName(varName string) string {
	// Strip common suffixes.
	suffixes := []string{"_API_KEY", "_TOKEN", "_SECRET", "_URL", "_ENDPOINT", "_DSN", "_CONNECTION_STRING", "_CONNECTION"}
	name := strings.ToUpper(varName)
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			name = strings.TrimSuffix(name, s)
			break
		}
	}
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// isKubernetesManifest checks if a file is likely a Kubernetes manifest.
func isKubernetesManifest(name string, path string) bool {
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return false
	}
	// Check for k8s-related directory names.
	dir := filepath.Dir(path)
	lower := strings.ToLower(dir)
	return strings.Contains(lower, "k8s") || strings.Contains(lower, "kubernetes") ||
		strings.Contains(lower, "manifests") || strings.Contains(lower, "deploy")
}

// scanKubernetesManifest discovers services and infrastructure from K8s manifests.
func (s *Scanner) scanKubernetesManifest(absPath, relPath string, result *ScanResult) {
	f, err := os.Open(absPath)
	if err != nil {
		result.Errors = append(result.Errors, "k8s manifest open: "+err.Error())
		return
	}
	defer f.Close()

	// Simple line-based scan for "kind:" and "name:" in metadata.
	scanner := bufio.NewScanner(f)
	var kind, name string
	inMetadata := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "kind:") {
			kind = strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
		}
		if trimmed == "metadata:" {
			inMetadata = true
			continue
		}
		if inMetadata && strings.HasPrefix(trimmed, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			inMetadata = false
		}
		// Document separator — emit entity if we have kind+name.
		if trimmed == "---" || trimmed == "" {
			if kind != "" && name != "" {
				entityType := classifyK8sKind(kind)
				result.Entities = append(result.Entities, Entity{
					Type:        entityType,
					Name:        name,
					Description: "K8s " + kind + " from " + relPath,
					Labels:      map[string]string{"discovered_from": "kubernetes", "k8s_kind": kind},
					Metadata:    map[string]string{"manifest": relPath, "kind": kind},
					Source:      SourceObserved,
				})
			}
			kind, name = "", ""
		}
	}
	// Emit last entity if file doesn't end with separator.
	if kind != "" && name != "" {
		entityType := classifyK8sKind(kind)
		result.Entities = append(result.Entities, Entity{
			Type:        entityType,
			Name:        name,
			Description: "K8s " + kind + " from " + relPath,
			Labels:      map[string]string{"discovered_from": "kubernetes", "k8s_kind": kind},
			Metadata:    map[string]string{"manifest": relPath, "kind": kind},
			Source:      SourceObserved,
		})
	}
}

// classifyK8sKind maps a Kubernetes resource kind to an entity type.
func classifyK8sKind(kind string) EntityType {
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset", "pod", "job", "cronjob":
		return EntityService
	case "service", "ingress", "gateway", "virtualservice":
		return EntityInfrastructure
	case "configmap", "secret":
		return EntityInfrastructure
	case "persistentvolumeclaim", "persistentvolume":
		return EntityDatastore
	case "namespace":
		return EntityEnvironment
	default:
		return EntityInfrastructure
	}
}
