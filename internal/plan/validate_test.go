package plan

import (
	"testing"
)

func TestValidateContent_Database_Valid(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_users_table",
			DDL:           "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL);",
			Direction:     "up",
			RollbackDDL:   "DROP TABLE users;",
		},
	}
	if err := ValidateContent(BackendDatabase, content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateContent_Database_EmptyDDL(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_users_table",
			DDL:           "",
			Direction:     "up",
		},
	}
	err := ValidateContent(BackendDatabase, content)
	if err == nil {
		t.Fatal("expected error for empty DDL")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if ve.Field != "ddl" {
		t.Fatalf("expected field 'ddl', got: %s", ve.Field)
	}
}

func TestValidateContent_Database_InvalidDDL(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_users_table",
			DDL:           "This is just a prose description of what I want to do",
			Direction:     "up",
		},
	}
	err := ValidateContent(BackendDatabase, content)
	if err == nil {
		t.Fatal("expected error for prose DDL")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if ve.Field != "ddl" {
		t.Fatalf("expected field 'ddl', got: %s", ve.Field)
	}
}

func TestValidateContent_Database_InvalidDirection(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_users_table",
			DDL:           "CREATE TABLE users (id TEXT PRIMARY KEY);",
			Direction:     "sideways",
		},
	}
	err := ValidateContent(BackendDatabase, content)
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if ve.Field != "direction" {
		t.Fatalf("expected field 'direction', got: %s", ve.Field)
	}
}

func TestValidateContent_Database_Nil(t *testing.T) {
	content := Content{}
	err := ValidateContent(BackendDatabase, content)
	if err == nil {
		t.Fatal("expected error for nil database plan")
	}
}

func TestValidateContent_Database_ComplexDDL(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_function",
			DDL: `CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE users ADD COLUMN email TEXT;
CREATE INDEX users_email ON users(email);`,
			Direction: "up",
		},
	}
	if err := ValidateContent(BackendDatabase, content); err != nil {
		t.Fatalf("expected valid complex DDL, got: %v", err)
	}
}

func TestValidateContent_Database_CommentedDDL(t *testing.T) {
	content := Content{
		Database: &DatabasePlan{
			MigrationName: "add_column",
			DDL:           "-- Add email column\nALTER TABLE users ADD COLUMN email TEXT;",
			Direction:     "up",
		},
	}
	if err := ValidateContent(BackendDatabase, content); err != nil {
		t.Fatalf("expected valid commented DDL, got: %v", err)
	}
}

func TestValidateContent_Terraform_Valid(t *testing.T) {
	content := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{"format_version":"1.0"}`,
			Provider: "aws",
			ResourceChanges: []ResourceChange{
				{Address: "aws_instance.web", Type: "aws_instance", Name: "web", ChangeAction: "create"},
			},
		},
	}
	if err := ValidateContent(BackendTerraform, content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateContent_Terraform_InvalidJSON(t *testing.T) {
	content := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{not valid json`,
			Provider: "aws",
		},
	}
	err := ValidateContent(BackendTerraform, content)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateContent_Terraform_MissingProvider(t *testing.T) {
	content := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{"format_version":"1.0"}`,
			Provider: "",
		},
	}
	err := ValidateContent(BackendTerraform, content)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestValidateContent_Terraform_InvalidChangeAction(t *testing.T) {
	content := Content{
		Terraform: &TerraformPlan{
			PlanJSON: `{"format_version":"1.0"}`,
			Provider: "aws",
			ResourceChanges: []ResourceChange{
				{Address: "aws_instance.web", ChangeAction: "destroy"},
			},
		},
	}
	err := ValidateContent(BackendTerraform, content)
	if err == nil {
		t.Fatal("expected error for invalid change_action")
	}
}

func TestValidateContent_Code_Valid(t *testing.T) {
	content := Content{
		Code: &CodePlan{
			FilePath:   "internal/plan/model.go",
			Language:   "go",
			BeforeHash: "abc123",
			AfterHash:  "def456",
			Hunks: []CodeHunk{
				{StartLine: 10, EndLine: 15, Content: "func NewFeature() {}", Operation: "add"},
			},
		},
	}
	if err := ValidateContent(BackendCode, content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateContent_Code_NoHunks(t *testing.T) {
	content := Content{
		Code: &CodePlan{
			FilePath: "main.go",
			Language: "go",
			Hunks:    []CodeHunk{},
		},
	}
	err := ValidateContent(BackendCode, content)
	if err == nil {
		t.Fatal("expected error for no hunks")
	}
}

func TestValidateContent_Code_InvalidOperation(t *testing.T) {
	content := Content{
		Code: &CodePlan{
			FilePath: "main.go",
			Language: "go",
			Hunks: []CodeHunk{
				{StartLine: 1, EndLine: 5, Content: "x", Operation: "replace"},
			},
		},
	}
	err := ValidateContent(BackendCode, content)
	if err == nil {
		t.Fatal("expected error for invalid operation")
	}
}

func TestValidateContent_Shell_Valid(t *testing.T) {
	content := Content{
		Shell: &ShellPlan{
			Commands: []ShellCommand{
				{Command: "npm install", Description: "Install dependencies", Idempotent: true},
				{Command: "npm run build", Description: "Build project", Idempotent: true},
			},
			WorkingDir: "/app",
			SideEffectManifest: SideEffectManifest{
				FilesCreated:  []string{"dist/"},
				FilesModified: []string{"node_modules/"},
			},
		},
	}
	if err := ValidateContent(BackendShell, content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateContent_Shell_EmptyCommands(t *testing.T) {
	content := Content{
		Shell: &ShellPlan{
			Commands:           []ShellCommand{},
			SideEffectManifest: SideEffectManifest{},
		},
	}
	err := ValidateContent(BackendShell, content)
	if err == nil {
		t.Fatal("expected error for empty commands")
	}
}

func TestValidateContent_Shell_EmptyCommandString(t *testing.T) {
	content := Content{
		Shell: &ShellPlan{
			Commands: []ShellCommand{
				{Command: "", Description: "nothing"},
			},
			SideEffectManifest: SideEffectManifest{},
		},
	}
	err := ValidateContent(BackendShell, content)
	if err == nil {
		t.Fatal("expected error for empty command string")
	}
}

func TestValidateContent_Deploy_Valid(t *testing.T) {
	content := Content{
		Deploy: &DeployPlan{
			ArtifactHash:   "sha256:abc123def456",
			ArtifactURL:    "registry.example.com/app:v1.2.3",
			Target:         "production",
			DeployStrategy: "rolling",
			HealthCheckURL: "https://app.example.com/healthz",
			RollbackHash:   "sha256:prev789",
		},
	}
	if err := ValidateContent(BackendDeploy, content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateContent_Deploy_MissingArtifactHash(t *testing.T) {
	content := Content{
		Deploy: &DeployPlan{
			ArtifactHash:   "",
			Target:         "production",
			DeployStrategy: "rolling",
		},
	}
	err := ValidateContent(BackendDeploy, content)
	if err == nil {
		t.Fatal("expected error for missing artifact_hash")
	}
}

func TestValidateContent_Deploy_InvalidStrategy(t *testing.T) {
	content := Content{
		Deploy: &DeployPlan{
			ArtifactHash:   "sha256:abc",
			Target:         "production",
			DeployStrategy: "yolo",
		},
	}
	err := ValidateContent(BackendDeploy, content)
	if err == nil {
		t.Fatal("expected error for invalid deploy_strategy")
	}
}

func TestValidateContent_Deploy_AllStrategies(t *testing.T) {
	strategies := []string{"rolling", "blue-green", "canary", "recreate"}
	for _, strat := range strategies {
		content := Content{
			Deploy: &DeployPlan{
				ArtifactHash:   "sha256:abc",
				Target:         "production",
				DeployStrategy: strat,
			},
		}
		if err := ValidateContent(BackendDeploy, content); err != nil {
			t.Fatalf("strategy %s should be valid, got: %v", strat, err)
		}
	}
}

func TestValidateContent_UnknownBackend(t *testing.T) {
	err := ValidateContent(Backend("unknown"), Content{})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestIsValidBackend(t *testing.T) {
	tests := []struct {
		backend Backend
		valid   bool
	}{
		{BackendDatabase, true},
		{BackendTerraform, true},
		{BackendCode, true},
		{BackendShell, true},
		{BackendDeploy, true},
		{Backend("unknown"), false},
		{Backend(""), false},
	}
	for _, tt := range tests {
		if got := IsValidBackend(tt.backend); got != tt.valid {
			t.Errorf("IsValidBackend(%q) = %v, want %v", tt.backend, got, tt.valid)
		}
	}
}

func TestIsValidState(t *testing.T) {
	tests := []struct {
		state State
		valid bool
	}{
		{StateDraft, true},
		{StateSubmitted, true},
		{StateClassified, true},
		{StateApproved, true},
		{StateApplied, true},
		{StateSuperseded, true},
		{StateRejected, true},
		{State("unknown"), false},
		{State(""), false},
	}
	for _, tt := range tests {
		if got := IsValidState(tt.state); got != tt.valid {
			t.Errorf("IsValidState(%q) = %v, want %v", tt.state, got, tt.valid)
		}
	}
}

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected int
	}{
		{"single", "CREATE TABLE t (id INT);", 1},
		{"multiple", "CREATE TABLE t (id INT); ALTER TABLE t ADD col TEXT;", 2},
		{"quoted semicolon", "INSERT INTO t VALUES ('a;b');", 1},
		{"no trailing semicolon", "CREATE TABLE t (id INT)", 1},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := splitStatements(tt.sql)
			if len(stmts) != tt.expected {
				t.Errorf("splitStatements(%q) returned %d statements, want %d", tt.sql, len(stmts), tt.expected)
			}
		})
	}
}

func TestValidateDDLSyntax(t *testing.T) {
	tests := []struct {
		name    string
		ddl     string
		wantErr bool
	}{
		{"create table", "CREATE TABLE t (id INT)", false},
		{"alter table", "ALTER TABLE t ADD COLUMN x TEXT", false},
		{"drop table", "DROP TABLE t", false},
		{"select", "SELECT 1", false},
		{"comment", "-- this is a comment", false},
		{"prose", "Please add a column to the table", true},
		{"empty", "", true},
		{"mixed valid", "CREATE TABLE t (id INT); ALTER TABLE t ADD x TEXT", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDDLSyntax(tt.ddl)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDDLSyntax(%q): err=%v, wantErr=%v", tt.ddl, err, tt.wantErr)
			}
		})
	}
}
