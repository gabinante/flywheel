package claims

import (
	"testing"

	"github.com/gabinante/flywheel/internal/plan"
)

func TestExtractTouches_NilPlan(t *testing.T) {
	touches := ExtractTouches(nil, "dev")
	if len(touches) != 0 {
		t.Errorf("expected 0 touches for nil plan, got %d", len(touches))
	}
}

func TestExtractTouches_CodePlan(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendCode,
		Content: plan.Content{
			Code: &plan.CodePlan{
				Diffs: []plan.CodeDiff{{
					FilePath:   "src/main.go",
					Language:   "go",
					BeforeHash: "abc123",
					AfterHash:  "def456",
					Hunks: []plan.CodeHunk{
						{StartLine: 10, EndLine: 20, Content: "new code", Operation: "modify"},
					},
				}},
			},
		},
	}

	touches := ExtractTouches(p, "development")
	if len(touches) != 1 {
		t.Fatalf("expected 1 touch, got %d", len(touches))
	}
	if touches[0].EntityID != "src/main.go" {
		t.Errorf("expected entity_id 'src/main.go', got %s", touches[0].EntityID)
	}
	if touches[0].Environment != "development" {
		t.Errorf("expected environment 'development', got %s", touches[0].Environment)
	}
	if touches[0].ClaimType != ClaimFileWrite {
		t.Errorf("expected claim_type file_write, got %s", touches[0].ClaimType)
	}
	if touches[0].Metadata["language"] != "go" {
		t.Errorf("expected metadata language 'go', got %v", touches[0].Metadata["language"])
	}
}

func TestExtractTouches_DatabasePlan(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendDatabase,
		Content: plan.Content{
			Database: &plan.DatabasePlan{
				MigrationName: "add_users_table",
				DDL:           "CREATE TABLE users (...)",
				Direction:     "up",
				DatabaseName:  "mydb",
				SchemaVersion: "v3",
			},
		},
	}

	touches := ExtractTouches(p, "production")
	if len(touches) != 1 {
		t.Fatalf("expected 1 touch, got %d", len(touches))
	}
	if touches[0].EntityID != "mydb:add_users_table" {
		t.Errorf("expected entity_id 'mydb:add_users_table', got %s", touches[0].EntityID)
	}
	if touches[0].ClaimType != ClaimSchema {
		t.Errorf("expected claim_type schema, got %s", touches[0].ClaimType)
	}
	if touches[0].Environment != "production" {
		t.Errorf("expected environment 'production', got %s", touches[0].Environment)
	}
}

func TestExtractTouches_DatabasePlan_NoDatabaseName(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendDatabase,
		Content: plan.Content{
			Database: &plan.DatabasePlan{
				MigrationName: "add_index",
				DDL:           "CREATE INDEX ...",
				Direction:     "up",
			},
		},
	}

	touches := ExtractTouches(p, "dev")
	if len(touches) != 1 {
		t.Fatalf("expected 1 touch, got %d", len(touches))
	}
	if touches[0].EntityID != "add_index" {
		t.Errorf("expected entity_id 'add_index', got %s", touches[0].EntityID)
	}
}

func TestExtractTouches_TerraformPlan(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendTerraform,
		Content: plan.Content{
			Terraform: &plan.TerraformPlan{
				PlanJSON: "{}",
				Provider: "aws",
				ResourceChanges: []plan.ResourceChange{
					{Address: "aws_s3_bucket.data", Type: "aws_s3_bucket", Name: "data", ChangeAction: "create"},
					{Address: "aws_iam_role.worker", Type: "aws_iam_role", Name: "worker", ChangeAction: "update"},
				},
			},
		},
	}

	touches := ExtractTouches(p, "staging")
	if len(touches) != 2 {
		t.Fatalf("expected 2 touches, got %d", len(touches))
	}
	if touches[0].EntityID != "aws_s3_bucket.data" {
		t.Errorf("expected entity_id 'aws_s3_bucket.data', got %s", touches[0].EntityID)
	}
	if touches[0].ClaimType != ClaimResource {
		t.Errorf("expected claim_type resource, got %s", touches[0].ClaimType)
	}
	if touches[1].EntityID != "aws_iam_role.worker" {
		t.Errorf("expected entity_id 'aws_iam_role.worker', got %s", touches[1].EntityID)
	}
}

func TestExtractTouches_TerraformPlan_NoChanges_FallsBackToWorkspace(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendTerraform,
		Content: plan.Content{
			Terraform: &plan.TerraformPlan{
				PlanJSON:        "{}",
				Provider:        "gcp",
				Workspace:       "prod-us-east1",
				ResourceChanges: []plan.ResourceChange{},
			},
		},
	}

	touches := ExtractTouches(p, "prod")
	if len(touches) != 1 {
		t.Fatalf("expected 1 workspace-level touch, got %d", len(touches))
	}
	if touches[0].EntityID != "prod-us-east1" {
		t.Errorf("expected entity_id 'prod-us-east1', got %s", touches[0].EntityID)
	}
}

func TestExtractTouches_ShellPlan(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendShell,
		Content: plan.Content{
			Shell: &plan.ShellPlan{
				Commands: []plan.ShellCommand{{Command: "npm build"}},
				SideEffectManifest: plan.SideEffectManifest{
					FileOps: []plan.FileOp{
						{Action: "write", Path: "/app/dist/**"},
						{Action: "read", Path: "/app/src/**"},     // reads are NOT claimed
						{Action: "delete", Path: "/tmp/*.cache"},
					},
					NetworkOps: []plan.NetworkOp{
						{Endpoint: "registry.npmjs.org:443", Method: "GET", Idempotent: true},
					},
				},
			},
		},
	}

	touches := ExtractTouches(p, "dev")
	// 2 file_write (write + delete) + 1 service (network) = 3
	if len(touches) != 3 {
		t.Fatalf("expected 3 touches, got %d", len(touches))
	}

	// First: file write
	if touches[0].EntityID != "/app/dist/**" {
		t.Errorf("expected entity_id '/app/dist/**', got %s", touches[0].EntityID)
	}
	if touches[0].ClaimType != ClaimFileWrite {
		t.Errorf("expected file_write, got %s", touches[0].ClaimType)
	}

	// Second: file delete (still a file_write claim)
	if touches[1].EntityID != "/tmp/*.cache" {
		t.Errorf("expected entity_id '/tmp/*.cache', got %s", touches[1].EntityID)
	}
	if touches[1].ClaimType != ClaimFileWrite {
		t.Errorf("expected file_write for delete, got %s", touches[1].ClaimType)
	}

	// Third: network → service
	if touches[2].EntityID != "registry.npmjs.org:443" {
		t.Errorf("expected entity_id 'registry.npmjs.org:443', got %s", touches[2].EntityID)
	}
	if touches[2].ClaimType != ClaimService {
		t.Errorf("expected service, got %s", touches[2].ClaimType)
	}
}

func TestExtractTouches_ShellPlan_EmptyManifest(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendShell,
		Content: plan.Content{
			Shell: &plan.ShellPlan{
				Commands:           []plan.ShellCommand{{Command: "echo hello"}},
				SideEffectManifest: plan.SideEffectManifest{},
			},
		},
	}

	touches := ExtractTouches(p, "dev")
	if len(touches) != 0 {
		t.Errorf("expected 0 touches for empty manifest, got %d", len(touches))
	}
}

func TestExtractTouches_DeployPlan(t *testing.T) {
	p := &plan.Plan{
		Backend: plan.BackendDeploy,
		Content: plan.Content{
			Deploy: &plan.DeployPlan{
				ArtifactHash:   "sha256:abc123",
				Target:         "prod-cluster-us-east-1",
				DeployStrategy: "blue-green",
				Replicas:       3,
			},
		},
	}

	touches := ExtractTouches(p, "production")
	if len(touches) != 1 {
		t.Fatalf("expected 1 touch, got %d", len(touches))
	}
	if touches[0].EntityID != "prod-cluster-us-east-1" {
		t.Errorf("expected entity_id 'prod-cluster-us-east-1', got %s", touches[0].EntityID)
	}
	if touches[0].ClaimType != ClaimDeployTarget {
		t.Errorf("expected deploy_target, got %s", touches[0].ClaimType)
	}
	if touches[0].Environment != "production" {
		t.Errorf("expected environment 'production', got %s", touches[0].Environment)
	}
}

func TestExtractTouches_NilContent(t *testing.T) {
	// Plan with backend but nil content (e.g., empty code plan)
	p := &plan.Plan{
		Backend: plan.BackendCode,
		Content: plan.Content{},
	}

	touches := ExtractTouches(p, "dev")
	if len(touches) != 0 {
		t.Errorf("expected 0 touches for nil content, got %d", len(touches))
	}
}
