package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDockerfile(t *testing.T) {
	dir := t.TempDir()
	svcDir := filepath.Join(dir, "services", "api")
	os.MkdirAll(svcDir, 0o755)
	os.WriteFile(filepath.Join(svcDir, "Dockerfile"), []byte("FROM golang:1.22\nCOPY . .\nRUN go build\n"), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}
	e := result.Entities[0]
	if e.Type != EntityService || e.Name != "api" || e.Source != SourceObserved {
		t.Fatalf("unexpected entity: %+v", e)
	}
}

func TestScanDockerCompose(t *testing.T) {
	dir := t.TempDir()
	composeContent := `version: "3"
services:
  api:
    build: ./api
  worker:
    build: ./worker
  postgres:
    image: postgres:15
  redis:
    image: redis:7
volumes:
  data:
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeContent), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 4 {
		t.Fatalf("expected 4 entities, got %d: %+v", len(result.Entities), result.Entities)
	}

	// Check classification.
	typeMap := make(map[string]EntityType)
	for _, e := range result.Entities {
		typeMap[e.Name] = e.Type
	}
	if typeMap["api"] != EntityService {
		t.Errorf("api should be service, got %s", typeMap["api"])
	}
	if typeMap["postgres"] != EntityDatastore {
		t.Errorf("postgres should be datastore, got %s", typeMap["postgres"])
	}
	if typeMap["redis"] != EntityDatastore {
		t.Errorf("redis should be datastore, got %s", typeMap["redis"])
	}
}

func TestScanTerraform(t *testing.T) {
	dir := t.TempDir()
	tfContent := `resource "aws_instance" "web_server" {
  ami           = "ami-12345"
  instance_type = "t3.micro"
}

resource "aws_rds_instance" "main_db" {
  engine = "postgres"
}
`
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tfContent), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(result.Entities))
	}
	for _, e := range result.Entities {
		if e.Type != EntityInfrastructure {
			t.Errorf("expected infrastructure, got %s for %s", e.Type, e.Name)
		}
		if e.Source != SourceObserved {
			t.Errorf("expected observed source for %s", e.Name)
		}
	}
}

func TestScanVarlockSchema(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `DATABASE_URL=                     # @required
REDIS_URL=                        # @optional
ANTHROPIC_API_KEY=                # @optional @sensitive
STRIPE_SECRET=                    # @optional @sensitive
SLACK_TOKEN=                      # @optional @sensitive
LOG_LEVEL=                        # @optional
`
	os.WriteFile(filepath.Join(dir, ".env.schema"), []byte(schemaContent), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	// DATABASE_URL, REDIS_URL, ANTHROPIC_API_KEY, STRIPE_SECRET, SLACK_TOKEN match integration patterns.
	// LOG_LEVEL does not.
	if len(result.Entities) < 4 {
		t.Fatalf("expected at least 4 integration entities, got %d: %+v", len(result.Entities), result.Entities)
	}
	for _, e := range result.Entities {
		if e.Type != EntityIntegration {
			t.Errorf("expected integration, got %s for %s", e.Type, e.Name)
		}
	}
}

func TestScanKubernetesManifest(t *testing.T) {
	dir := t.TempDir()
	k8sDir := filepath.Join(dir, "k8s")
	os.MkdirAll(k8sDir, 0o755)
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
spec:
  replicas: 3
---
apiVersion: v1
kind: Service
metadata:
  name: api-service
`
	os.WriteFile(filepath.Join(k8sDir, "api.yaml"), []byte(manifest), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d: %+v", len(result.Entities), result.Entities)
	}
}

func TestScanSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".git")
	os.MkdirAll(hiddenDir, 0o755)
	os.WriteFile(filepath.Join(hiddenDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644)

	scanner := NewScanner()
	result, err := scanner.ScanRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 0 {
		t.Fatalf("expected 0 entities from hidden dirs, got %d", len(result.Entities))
	}
}

func TestValidEntityTypes(t *testing.T) {
	types := ValidEntityTypes()
	if len(types) != 6 {
		t.Fatalf("expected 6 entity types, got %d", len(types))
	}
	for _, et := range types {
		if !IsValidEntityType(string(et)) {
			t.Errorf("%s should be valid", et)
		}
	}
	if IsValidEntityType("bogus") {
		t.Error("bogus should not be valid")
	}
}

func TestValidEdgeTypes(t *testing.T) {
	types := ValidEdgeTypes()
	if len(types) != 7 {
		t.Fatalf("expected 7 edge types, got %d", len(types))
	}
	for _, et := range types {
		if !IsValidEdgeType(string(et)) {
			t.Errorf("%s should be valid", et)
		}
	}
	if IsValidEdgeType("bogus") {
		t.Error("bogus should not be valid")
	}
}
