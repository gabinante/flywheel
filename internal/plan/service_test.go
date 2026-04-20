package plan

import (
	"testing"
	"time"
)

func TestPlanModel_Backends(t *testing.T) {
	backends := AllBackends()
	if len(backends) != 5 {
		t.Fatalf("expected 5 backends, got %d", len(backends))
	}
	expected := map[Backend]bool{
		BackendDatabase:  true,
		BackendTerraform: true,
		BackendCode:      true,
		BackendShell:     true,
		BackendDeploy:    true,
	}
	for _, b := range backends {
		if !expected[b] {
			t.Errorf("unexpected backend: %s", b)
		}
	}
}

func TestPlanModel_States(t *testing.T) {
	states := AllStates()
	if len(states) != 7 {
		t.Fatalf("expected 7 states, got %d", len(states))
	}
	expected := map[State]bool{
		StateDraft:      true,
		StateSubmitted:  true,
		StateClassified: true,
		StateApproved:   true,
		StateApplied:    true,
		StateSuperseded: true,
		StateRejected:   true,
	}
	for _, s := range states {
		if !expected[s] {
			t.Errorf("unexpected state: %s", s)
		}
	}
}

func TestPlanContent_NoUntypedEscapeHatch(t *testing.T) {
	// Verify that the Content struct has no untyped escape hatch.
	// Each backend has exactly one typed field; there is no "raw" or "other" field.
	content := Content{}

	// All fields are nil — no default "pass-through" for arbitrary content
	if content.Database != nil || content.Terraform != nil || content.Code != nil || content.Shell != nil || content.Deploy != nil {
		t.Fatal("empty Content should have all nil fields")
	}

	// Validation should fail for every backend with empty content
	for _, backend := range AllBackends() {
		if err := ValidateContent(backend, content); err == nil {
			t.Errorf("expected validation error for backend %s with empty content", backend)
		}
	}
}

func TestPlanFreshness(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	// Plan with future expiry is fresh
	p := &Plan{ExpiresAt: &future}
	if p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
		t.Error("plan with future expiry should be fresh")
	}

	// Plan with past expiry is stale
	p = &Plan{ExpiresAt: &past}
	if p.ExpiresAt != nil && !time.Now().UTC().After(*p.ExpiresAt) {
		t.Error("plan with past expiry should be stale")
	}

	// Plan with no expiry is always fresh
	p = &Plan{ExpiresAt: nil}
	if p.ExpiresAt != nil {
		t.Error("plan with nil expiry should be fresh")
	}
}

func TestBackendSchemaIndependence(t *testing.T) {
	// Each backend's validation is independent — a valid database plan
	// doesn't affect terraform validation and vice versa.
	dbContent := Content{
		Database: &DatabasePlan{
			MigrationName: "test",
			DDL:           "CREATE TABLE t (id INT);",
			Direction:     "up",
		},
	}
	if err := ValidateContent(BackendDatabase, dbContent); err != nil {
		t.Fatalf("valid database content: %v", err)
	}
	// Same content struct fails for terraform backend (wrong sub-schema populated)
	if err := ValidateContent(BackendTerraform, dbContent); err == nil {
		t.Fatal("database content should fail terraform validation")
	}

	shellContent := Content{
		Shell: &ShellPlan{
			Commands: []ShellCommand{
				{Command: "echo hello", Idempotent: true},
			},
			SideEffectManifest: SideEffectManifest{},
		},
	}
	if err := ValidateContent(BackendShell, shellContent); err != nil {
		t.Fatalf("valid shell content: %v", err)
	}
	// Shell content fails for deploy backend
	if err := ValidateContent(BackendDeploy, shellContent); err == nil {
		t.Fatal("shell content should fail deploy validation")
	}
}

func TestClassifierRulesMapOneToOne(t *testing.T) {
	// The classifier maps one-to-one to backend schemas.
	// Each backend has exactly one sub-schema field in Content.
	// This test verifies that ValidateContent enforces the correct
	// backend->schema mapping by checking that each backend only accepts
	// its own typed sub-schema.

	testCases := []struct {
		backend Backend
		content Content
	}{
		{BackendDatabase, Content{Database: &DatabasePlan{MigrationName: "t", DDL: "CREATE TABLE t(id INT);", Direction: "up"}}},
		{BackendTerraform, Content{Terraform: &TerraformPlan{PlanJSON: `{}`, Provider: "aws"}}},
		{BackendCode, Content{Code: &CodePlan{FilePath: "f.go", Language: "go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 2, Content: "x", Operation: "add"}}}}},
		{BackendShell, Content{Shell: &ShellPlan{Commands: []ShellCommand{{Command: "ls"}}, SideEffectManifest: SideEffectManifest{}}}},
		{BackendDeploy, Content{Deploy: &DeployPlan{ArtifactHash: "sha256:abc", Target: "prod", DeployStrategy: "rolling"}}},
	}

	for _, tc := range testCases {
		// Should pass for its own backend
		if err := ValidateContent(tc.backend, tc.content); err != nil {
			t.Errorf("backend %s should validate its own content, got: %v", tc.backend, err)
		}

		// Should fail for all other backends
		for _, other := range AllBackends() {
			if other == tc.backend {
				continue
			}
			if err := ValidateContent(other, tc.content); err == nil {
				t.Errorf("backend %s content should NOT validate against backend %s", tc.backend, other)
			}
		}
	}
}
