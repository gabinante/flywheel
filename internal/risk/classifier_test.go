package risk

import (
	"testing"
)

// ============================================================
// Per-backend classifier tests
// ============================================================

func TestDatabaseClassifier(t *testing.T) {
	c := &DatabaseClassifier{}

	tests := []struct {
		name      string
		op        Operation
		wantLevel RiskLevel
		wantRule  string
	}{
		// Safe
		{
			name:      "SELECT is safe",
			op:        Operation{Backend: BackendDatabase, Action: "SELECT * FROM users", Target: "users"},
			wantLevel: RiskSafe,
			wantRule:  "db.select",
		},
		{
			name:      "EXPLAIN is safe",
			op:        Operation{Backend: BackendDatabase, Action: "EXPLAIN ANALYZE", Target: ""},
			wantLevel: RiskSafe,
			wantRule:  "db.explain",
		},
		// Low (additive)
		{
			name:      "CREATE TABLE is low",
			op:        Operation{Backend: BackendDatabase, Action: "CREATE TABLE users (id int)", Target: "users"},
			wantLevel: RiskLow,
			wantRule:  "db.create_table",
		},
		{
			name:      "CREATE VIEW is low",
			op:        Operation{Backend: BackendDatabase, Action: "CREATE VIEW active_users AS SELECT ...", Target: "active_users"},
			wantLevel: RiskLow,
			wantRule:  "db.create_view",
		},
		// Reversible
		{
			name:      "ADD COLUMN is reversible",
			op:        Operation{Backend: BackendDatabase, Action: "ALTER TABLE users ADD COLUMN email text", Target: "users"},
			wantLevel: RiskReversible,
			wantRule:  "db.add_column",
		},
		{
			name:      "CREATE INDEX is reversible",
			op:        Operation{Backend: BackendDatabase, Action: "CREATE INDEX idx_email ON users(email)", Target: "users"},
			wantLevel: RiskReversible,
			wantRule:  "db.create_index",
		},
		// High
		{
			name:      "ALTER COLUMN TYPE is high",
			op:        Operation{Backend: BackendDatabase, Action: "ALTER TABLE users ALTER COLUMN TYPE varchar(50)", Target: "users"},
			wantLevel: RiskHigh,
			wantRule:  "db.alter_column_type",
		},
		{
			name:      "RENAME TABLE is high",
			op:        Operation{Backend: BackendDatabase, Action: "RENAME TABLE users TO archived_users", Target: "users"},
			wantLevel: RiskHigh,
			wantRule:  "db.rename_table",
		},
		// Destructive
		{
			name:      "DROP TABLE is destructive",
			op:        Operation{Backend: BackendDatabase, Action: "DROP TABLE users", Target: "users"},
			wantLevel: RiskDestructive,
			wantRule:  "db.drop_table",
		},
		{
			name:      "TRUNCATE TABLE is destructive",
			op:        Operation{Backend: BackendDatabase, Action: "TRUNCATE TABLE sessions", Target: "sessions"},
			wantLevel: RiskDestructive,
			wantRule:  "db.truncate_table",
		},
		{
			name:      "TRUNCATE without TABLE is destructive",
			op:        Operation{Backend: BackendDatabase, Action: "TRUNCATE sessions", Target: "sessions"},
			wantLevel: RiskDestructive,
			wantRule:  "db.truncate",
		},
		{
			name:      "DROP DATABASE is destructive",
			op:        Operation{Backend: BackendDatabase, Action: "DROP DATABASE myapp", Target: "myapp"},
			wantLevel: RiskDestructive,
			wantRule:  "db.drop_database",
		},
		{
			name: "narrowing column size is destructive",
			op: Operation{
				Backend: BackendDatabase,
				Action:  "ALTER TABLE users ALTER COLUMN name",
				Target:  "users",
				Details: map[string]any{"old_size": 255, "new_size": 50},
			},
			wantLevel: RiskDestructive,
			wantRule:  "db.narrowing_alteration",
		},
		{
			name: "narrowing flag from lint tool",
			op: Operation{
				Backend: BackendDatabase,
				Action:  "ALTER TABLE users ALTER COLUMN data",
				Target:  "users",
				Details: map[string]any{"narrowing": true},
			},
			wantLevel: RiskDestructive,
			wantRule:  "db.narrowing_alteration",
		},
		// Unknown
		{
			name:      "unknown DB operation is destructive",
			op:        Operation{Backend: BackendDatabase, Action: "VACUUM FULL", Target: "users"},
			wantLevel: RiskDestructive,
			wantRule:  "db.unknown_operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citations := c.Classify(tt.op)
			if level != tt.wantLevel {
				t.Errorf("got level %v, want %v", level, tt.wantLevel)
			}
			if len(citations) == 0 {
				t.Fatal("expected at least one citation")
			}
			if citations[0].Rule != tt.wantRule {
				t.Errorf("got rule %q, want %q", citations[0].Rule, tt.wantRule)
			}
		})
	}
}

func TestTerraformClassifier(t *testing.T) {
	c := &TerraformClassifier{}

	tests := []struct {
		name      string
		op        Operation
		wantLevel RiskLevel
		wantRule  string
	}{
		// Safe
		{
			name:      "read data source is safe",
			op:        Operation{Backend: BackendTerraform, Action: "read", Target: "data.aws_ami.latest"},
			wantLevel: RiskSafe,
			wantRule:  "tf.read",
		},
		// Low
		{
			name:      "create new resource is low",
			op:        Operation{Backend: BackendTerraform, Action: "create", Target: "aws_iam_role.new_role"},
			wantLevel: RiskLow,
			wantRule:  "tf.create",
		},
		// Reversible
		{
			name:      "update in-place is reversible",
			op:        Operation{Backend: BackendTerraform, Action: "update", Target: "aws_iam_role.existing"},
			wantLevel: RiskReversible,
			wantRule:  "tf.update_in_place",
		},
		// High
		{
			name:      "destroy stateless resource is high",
			op:        Operation{Backend: BackendTerraform, Action: "destroy", Target: "aws_iam_role.old_role"},
			wantLevel: RiskHigh,
			wantRule:  "tf.destroy_stateless",
		},
		{
			name:      "replace stateless resource is high",
			op:        Operation{Backend: BackendTerraform, Action: "replace", Target: "aws_iam_role.role"},
			wantLevel: RiskHigh,
			wantRule:  "tf.replace_stateless",
		},
		// Destructive
		{
			name:      "destroy RDS instance is destructive",
			op:        Operation{Backend: BackendTerraform, Action: "destroy", Target: "aws_rds_instance.main"},
			wantLevel: RiskDestructive,
			wantRule:  "tf.destroy_stateful",
		},
		{
			name:      "destroy S3 bucket is destructive",
			op:        Operation{Backend: BackendTerraform, Action: "destroy", Target: "aws_s3_bucket.data"},
			wantLevel: RiskDestructive,
			wantRule:  "tf.destroy_stateful",
		},
		{
			name:      "replace DynamoDB table is destructive",
			op:        Operation{Backend: BackendTerraform, Action: "replace", Target: "aws_dynamodb_table.orders"},
			wantLevel: RiskDestructive,
			wantRule:  "tf.replace_stateful",
		},
		{
			name: "explicit resource_type in details",
			op: Operation{
				Backend: BackendTerraform, Action: "destroy", Target: "my_db",
				Details: map[string]any{"resource_type": "aws_rds_instance"},
			},
			wantLevel: RiskDestructive,
			wantRule:  "tf.destroy_stateful",
		},
		// Unknown action
		{
			name:      "unknown terraform action is destructive",
			op:        Operation{Backend: BackendTerraform, Action: "taint", Target: "aws_instance.web"},
			wantLevel: RiskDestructive,
			wantRule:  "tf.unknown_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citations := c.Classify(tt.op)
			if level != tt.wantLevel {
				t.Errorf("got level %v, want %v", level, tt.wantLevel)
			}
			if len(citations) == 0 {
				t.Fatal("expected at least one citation")
			}
			if citations[0].Rule != tt.wantRule {
				t.Errorf("got rule %q, want %q", citations[0].Rule, tt.wantRule)
			}
		})
	}
}

func TestCodeClassifier(t *testing.T) {
	c := &CodeClassifier{}

	tests := []struct {
		name      string
		op        Operation
		wantLevel RiskLevel
		wantRule  string
	}{
		// Safe
		{
			name:      "test file is safe",
			op:        Operation{Backend: BackendCode, Action: "edit", Target: "internal/risk/classifier_test.go"},
			wantLevel: RiskSafe,
			wantRule:  "code.test_or_docs",
		},
		{
			name:      "markdown doc is safe",
			op:        Operation{Backend: BackendCode, Action: "edit", Target: "docs/architecture.md"},
			wantLevel: RiskSafe,
			wantRule:  "code.test_or_docs",
		},
		{
			name: "comment scope is safe",
			op: Operation{
				Backend: BackendCode, Action: "edit", Target: "internal/server.go",
				Details: map[string]any{"scope": "comment"},
			},
			wantLevel: RiskSafe,
			wantRule:  "code.scope_comment",
		},
		{
			name: "test scope is safe",
			op: Operation{
				Backend: BackendCode, Action: "edit", Target: "internal/handler.go",
				Details: map[string]any{"scope": "test"},
			},
			wantLevel: RiskSafe,
			wantRule:  "code.scope_test",
		},
		// Low
		{
			name:      "add new file is low",
			op:        Operation{Backend: BackendCode, Action: "add", Target: "internal/risk/model.go"},
			wantLevel: RiskLow,
			wantRule:  "code.add_file",
		},
		{
			name: "internal scope is low",
			op: Operation{
				Backend: BackendCode, Action: "edit", Target: "internal/helper.go",
				Details: map[string]any{"scope": "internal"},
			},
			wantLevel: RiskLow,
			wantRule:  "code.scope_internal",
		},
		// Reversible
		{
			name:      "edit regular file is reversible",
			op:        Operation{Backend: BackendCode, Action: "edit", Target: "internal/risk/classifier.go"},
			wantLevel: RiskReversible,
			wantRule:  "code.edit_file",
		},
		// High
		{
			name:      "edit main.go is high",
			op:        Operation{Backend: BackendCode, Action: "edit", Target: "cmd/server/main.go"},
			wantLevel: RiskHigh,
			wantRule:  "code.startup_path",
		},
		{
			name: "public API scope is high",
			op: Operation{
				Backend: BackendCode, Action: "edit", Target: "api/handler.go",
				Details: map[string]any{"scope": "public_api"},
			},
			wantLevel: RiskHigh,
			wantRule:  "code.scope_public_api",
		},
		// Destructive
		{
			name:      "edit auth code is destructive",
			op:        Operation{Backend: BackendCode, Action: "edit", Target: "internal/auth/middleware.go"},
			wantLevel: RiskDestructive,
			wantRule:  "code.security_sensitive",
		},
		{
			name: "security scope is destructive",
			op: Operation{
				Backend: BackendCode, Action: "edit", Target: "internal/handler.go",
				Details: map[string]any{"scope": "security"},
			},
			wantLevel: RiskDestructive,
			wantRule:  "code.scope_security",
		},
		// Delete
		{
			name:      "delete test file is safe",
			op:        Operation{Backend: BackendCode, Action: "delete", Target: "internal/old_test.go"},
			wantLevel: RiskSafe,
			wantRule:  "code.delete_test_or_docs",
		},
		{
			name:      "delete startup file is high",
			op:        Operation{Backend: BackendCode, Action: "delete", Target: "cmd/server/main.go"},
			wantLevel: RiskHigh,
			wantRule:  "code.delete_startup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citations := c.Classify(tt.op)
			if level != tt.wantLevel {
				t.Errorf("got level %v, want %v", level, tt.wantLevel)
			}
			if len(citations) == 0 {
				t.Fatal("expected at least one citation")
			}
			if citations[0].Rule != tt.wantRule {
				t.Errorf("got rule %q, want %q", citations[0].Rule, tt.wantRule)
			}
		})
	}
}

func TestShellClassifier(t *testing.T) {
	c := &ShellClassifier{}

	tests := []struct {
		name      string
		op        Operation
		wantLevel RiskLevel
		wantRule  string
	}{
		// Safe
		{
			name: "read-only ls is safe",
			op: Operation{
				Backend: BackendShell, Action: "list files", Target: "/app",
				Details: map[string]any{"side_effects": []any{"ls /app"}},
			},
			wantLevel: RiskSafe,
			wantRule:  "shell.safe_effect",
		},
		// Low
		{
			name: "create file is low",
			op: Operation{
				Backend: BackendShell, Action: "create file", Target: "/app/config.yaml",
				Details: map[string]any{},
			},
			wantLevel: RiskLow,
			wantRule:  "shell.low_effect",
		},
		// Reversible
		{
			name: "restart service is reversible",
			op: Operation{
				Backend: BackendShell, Action: "restart service", Target: "nginx",
				Details: map[string]any{},
			},
			wantLevel: RiskReversible,
			wantRule:  "shell.reversible_effect",
		},
		// High
		{
			name: "install package is high",
			op: Operation{
				Backend: BackendShell, Action: "install package", Target: "nginx",
				Details: map[string]any{},
			},
			wantLevel: RiskHigh,
			wantRule:  "shell.high_effect",
		},
		{
			name: "sudo bumps to high",
			op: Operation{
				Backend: BackendShell, Action: "create file", Target: "/etc/config",
				Details: map[string]any{"requires_sudo": true},
			},
			wantLevel: RiskHigh,
			wantRule:  "shell.low_effect", // first citation is the base, sudo is additional
		},
		// Destructive
		{
			name: "rm -rf is destructive",
			op: Operation{
				Backend: BackendShell, Action: "rm -rf /data", Target: "/data",
				Details: map[string]any{},
			},
			wantLevel: RiskDestructive,
			wantRule:  "shell.destructive_effect",
		},
		// No manifest
		{
			name: "no manifest is destructive",
			op: Operation{
				Backend: BackendShell, Action: "", Target: "mystery_script.sh",
				Details: map[string]any{},
			},
			wantLevel: RiskDestructive,
			wantRule:  "shell.no_manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citations := c.Classify(tt.op)
			if level != tt.wantLevel {
				t.Errorf("got level %v, want %v", level, tt.wantLevel)
			}
			if len(citations) == 0 {
				t.Fatal("expected at least one citation")
			}
			if citations[0].Rule != tt.wantRule {
				t.Errorf("got rule %q, want %q", citations[0].Rule, tt.wantRule)
			}
		})
	}
}

func TestExternalAPIClassifier(t *testing.T) {
	c := NewExternalAPIClassifier()

	tests := []struct {
		name      string
		op        Operation
		wantLevel RiskLevel
		wantRule  string
	}{
		// Safe
		{
			name: "Stripe list_charges is safe",
			op: Operation{
				Backend: BackendExternalAPI, Action: "GET", Target: "/v1/charges",
				Details: map[string]any{"integration": "stripe", "operation": "list_charges"},
			},
			wantLevel: RiskSafe,
			wantRule:  "api.stripe.list_charges",
		},
		{
			name: "HTTP GET is safe",
			op: Operation{
				Backend: BackendExternalAPI, Action: "GET", Target: "https://api.example.com/status",
				Details: map[string]any{},
			},
			wantLevel: RiskSafe,
			wantRule:  "api.http_read",
		},
		// Low
		{
			name: "GitHub create_pr is low",
			op: Operation{
				Backend: BackendExternalAPI, Action: "POST", Target: "/repos/owner/repo/pulls",
				Details: map[string]any{"integration": "github", "operation": "create_pr"},
			},
			wantLevel: RiskLow,
			wantRule:  "api.github.create_pr",
		},
		{
			name: "HTTP POST to unknown API is low",
			op: Operation{
				Backend: BackendExternalAPI, Action: "POST", Target: "https://api.example.com/resources",
				Details: map[string]any{},
			},
			wantLevel: RiskLow,
			wantRule:  "api.http_post",
		},
		// Reversible
		{
			name: "GitHub merge_pr is reversible",
			op: Operation{
				Backend: BackendExternalAPI, Action: "PUT", Target: "/repos/owner/repo/pulls/1/merge",
				Details: map[string]any{"integration": "github", "operation": "merge_pr"},
			},
			wantLevel: RiskReversible,
			wantRule:  "api.github.merge_pr",
		},
		// High
		{
			name: "Stripe create_charge is high",
			op: Operation{
				Backend: BackendExternalAPI, Action: "POST", Target: "/v1/charges",
				Details: map[string]any{"integration": "stripe", "operation": "create_charge"},
			},
			wantLevel: RiskHigh,
			wantRule:  "api.stripe.create_charge",
		},
		{
			name: "PagerDuty create_incident is high",
			op: Operation{
				Backend: BackendExternalAPI, Action: "POST", Target: "/incidents",
				Details: map[string]any{"integration": "pagerduty", "operation": "create_incident"},
			},
			wantLevel: RiskHigh,
			wantRule:  "api.pagerduty.create_incident",
		},
		{
			name: "HTTP DELETE is high",
			op: Operation{
				Backend: BackendExternalAPI, Action: "DELETE", Target: "https://api.example.com/resources/1",
				Details: map[string]any{},
			},
			wantLevel: RiskHigh,
			wantRule:  "api.http_delete",
		},
		// Destructive
		{
			name: "GitHub delete_repo is destructive",
			op: Operation{
				Backend: BackendExternalAPI, Action: "DELETE", Target: "/repos/owner/repo",
				Details: map[string]any{"integration": "github", "operation": "delete_repo"},
			},
			wantLevel: RiskDestructive,
			wantRule:  "api.github.delete_repo",
		},
		{
			name: "Stripe delete_customer is destructive",
			op: Operation{
				Backend: BackendExternalAPI, Action: "DELETE", Target: "/v1/customers/cus_123",
				Details: map[string]any{"integration": "stripe", "operation": "delete_customer"},
			},
			wantLevel: RiskDestructive,
			wantRule:  "api.stripe.delete_customer",
		},
		// Unknown integration + unknown operation
		{
			name: "unknown operation on known integration is high",
			op: Operation{
				Backend: BackendExternalAPI, Action: "POST", Target: "/v1/something",
				Details: map[string]any{"integration": "stripe", "operation": "unknown_thing"},
			},
			wantLevel: RiskHigh,
			wantRule:  "api.stripe.unknown_operation",
		},
		{
			name: "unknown method is destructive",
			op: Operation{
				Backend: BackendExternalAPI, Action: "PURGE", Target: "https://cdn.example.com/cache",
				Details: map[string]any{},
			},
			wantLevel: RiskDestructive,
			wantRule:  "api.unknown_method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citations := c.Classify(tt.op)
			if level != tt.wantLevel {
				t.Errorf("got level %v, want %v", level, tt.wantLevel)
			}
			if len(citations) == 0 {
				t.Fatal("expected at least one citation")
			}
			if citations[0].Rule != tt.wantRule {
				t.Errorf("got rule %q, want %q", citations[0].Rule, tt.wantRule)
			}
		})
	}
}

// ============================================================
// Environment multiplier tests
// ============================================================

func TestEnvironmentMultiplier(t *testing.T) {
	em := DefaultEnvironmentMultipliers()

	tests := []struct {
		name      string
		level     RiskLevel
		env       string
		wantLevel RiskLevel
		wantCite  bool
	}{
		{"dev reduces risk", RiskReversible, "development", RiskLow, true},
		{"dev reduces reversible to low", RiskReversible, "dev", RiskLow, true},
		{"dev safe stays safe", RiskSafe, "dev", RiskSafe, false},
		{"staging no change", RiskReversible, "staging", RiskReversible, false},
		{"prod increases risk", RiskReversible, "production", RiskHigh, true},
		{"prod increases to high", RiskHigh, "prod", RiskDestructive, true},
		{"prod destructive stays destructive", RiskDestructive, "production", RiskDestructive, false},
		{"empty env no change", RiskReversible, "", RiskReversible, false},
		{"unknown env treated as prod", RiskReversible, "canary", RiskHigh, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citation := em.Apply(tt.level, tt.env)
			if level != tt.wantLevel {
				t.Errorf("got %v, want %v", level, tt.wantLevel)
			}
			hasCite := citation != nil
			if hasCite != tt.wantCite {
				t.Errorf("citation present = %v, want %v", hasCite, tt.wantCite)
			}
		})
	}
}

// ============================================================
// Sensitivity classifier tests
// ============================================================

func TestSensitivityClassifier(t *testing.T) {
	sc := DefaultSensitivityClassifier()

	tests := []struct {
		name      string
		level     RiskLevel
		tags      []string
		wantLevel RiskLevel
	}{
		{"no tags no change", RiskLow, nil, RiskLow},
		{"pii raises to high", RiskLow, []string{"pii"}, RiskHigh},
		{"financial raises to high", RiskLow, []string{"financial"}, RiskHigh},
		{"hipaa raises to destructive", RiskLow, []string{"hipaa"}, RiskDestructive},
		{"sox raises to destructive", RiskHigh, []string{"sox"}, RiskDestructive},
		{"customer-facing raises to reversible", RiskLow, []string{"customer-facing"}, RiskReversible},
		{"internal-only no raise from high", RiskHigh, []string{"internal-only"}, RiskHigh},
		{"already above floor", RiskDestructive, []string{"pii"}, RiskDestructive},
		{"multiple tags highest wins", RiskLow, []string{"customer-facing", "pii", "hipaa"}, RiskDestructive},
		{"unknown tag raises to high", RiskLow, []string{"mystery-tag"}, RiskHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, _ := sc.Apply(tt.level, tt.tags)
			if level != tt.wantLevel {
				t.Errorf("got %v, want %v", level, tt.wantLevel)
			}
		})
	}
}

// ============================================================
// LLM advisory tests
// ============================================================

func TestLLMAdvisory(t *testing.T) {
	tests := []struct {
		name      string
		level     RiskLevel
		advisory  *LLMAdvisory
		wantLevel RiskLevel
		wantCite  bool
	}{
		{"nil advisory no change", RiskLow, nil, RiskLow, false},
		{"raise from low to high", RiskLow, &LLMAdvisory{SuggestedLevel: RiskHigh, Reason: "complex migration"}, RiskHigh, true},
		{"cannot lower from high to low", RiskHigh, &LLMAdvisory{SuggestedLevel: RiskLow, Reason: "seems safe"}, RiskHigh, false},
		{"equal level no change", RiskReversible, &LLMAdvisory{SuggestedLevel: RiskReversible, Reason: "agree"}, RiskReversible, false},
		{"raise to destructive", RiskSafe, &LLMAdvisory{SuggestedLevel: RiskDestructive, Reason: "critical system"}, RiskDestructive, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, citation := ApplyAdvisory(tt.level, tt.advisory)
			if level != tt.wantLevel {
				t.Errorf("got %v, want %v", level, tt.wantLevel)
			}
			hasCite := citation != nil
			if hasCite != tt.wantCite {
				t.Errorf("citation present = %v, want %v", hasCite, tt.wantCite)
			}
		})
	}
}

// ============================================================
// Full pipeline (Classifier) tests
// ============================================================

func TestClassifier_Deterministic(t *testing.T) {
	c := NewClassifier()

	plan := Plan{
		Operations: []Operation{
			{Backend: BackendDatabase, Action: "CREATE TABLE users (id int)", Target: "users"},
			{Backend: BackendCode, Action: "add", Target: "internal/user/model.go"},
		},
		Environment: "staging",
	}

	// Classify twice — must be identical.
	r1 := c.Classify(plan, nil)
	r2 := c.Classify(plan, nil)

	if r1.FinalLevel != r2.FinalLevel {
		t.Errorf("non-deterministic: first=%v, second=%v", r1.FinalLevel, r2.FinalLevel)
	}
	if len(r1.Citations) != len(r2.Citations) {
		t.Errorf("citation count mismatch: first=%d, second=%d", len(r1.Citations), len(r2.Citations))
	}
	for i := range r1.Citations {
		if r1.Citations[i].Rule != r2.Citations[i].Rule {
			t.Errorf("citation[%d] rule mismatch: %q vs %q", i, r1.Citations[i].Rule, r2.Citations[i].Rule)
		}
	}
}

func TestClassifier_EmptyPlan(t *testing.T) {
	c := NewClassifier()
	result := c.Classify(Plan{}, nil)

	if result.FinalLevel != RiskSafe {
		t.Errorf("empty plan: got %v, want safe", result.FinalLevel)
	}
	if len(result.Citations) == 0 {
		t.Error("expected at least one citation for empty plan")
	}
	if result.Citations[0].Rule != "plan.empty" {
		t.Errorf("expected plan.empty rule, got %q", result.Citations[0].Rule)
	}
}

func TestClassifier_HighestOperationWins(t *testing.T) {
	c := NewClassifier()

	plan := Plan{
		Operations: []Operation{
			{Backend: BackendDatabase, Action: "SELECT * FROM users", Target: "users"},         // safe
			{Backend: BackendDatabase, Action: "CREATE TABLE logs (id int)", Target: "logs"},    // low
			{Backend: BackendDatabase, Action: "DROP TABLE sessions", Target: "sessions"},       // destructive
		},
		Environment: "staging",
	}

	result := c.Classify(plan, nil)
	if result.BaseLevel != RiskDestructive {
		t.Errorf("expected destructive base, got %v", result.BaseLevel)
	}
}

func TestClassifier_EnvironmentMultiplier(t *testing.T) {
	c := NewClassifier()

	plan := Plan{
		Operations: []Operation{
			{Backend: BackendCode, Action: "edit", Target: "internal/risk/classifier.go"},
		},
	}

	// Dev reduces risk.
	plan.Environment = "development"
	devResult := c.Classify(plan, nil)

	// Prod increases risk.
	plan.Environment = "production"
	prodResult := c.Classify(plan, nil)

	if devResult.FinalLevel >= prodResult.FinalLevel {
		t.Errorf("dev (%v) should be lower than prod (%v)", devResult.FinalLevel, prodResult.FinalLevel)
	}
}

func TestClassifier_SensitivityTags(t *testing.T) {
	c := NewClassifier()

	// Simple additive change (low risk normally).
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendCode, Action: "add", Target: "internal/handler.go"},
		},
		Environment: "staging",
		ServiceTags: []string{"hipaa"},
	}

	result := c.Classify(plan, nil)
	if result.FinalLevel != RiskDestructive {
		t.Errorf("HIPAA tag should raise to destructive, got %v", result.FinalLevel)
	}
}

func TestClassifier_LLMAdvisoryUpwardOnly(t *testing.T) {
	c := NewClassifier()

	plan := Plan{
		Operations: []Operation{
			{Backend: BackendCode, Action: "add", Target: "internal/helper.go"},
		},
		Environment: "staging",
	}

	// Without advisory.
	baseResult := c.Classify(plan, nil)

	// With advisory raising.
	raiseAdvisory := &LLMAdvisory{SuggestedLevel: RiskDestructive, Reason: "this is a critical system"}
	raisedResult := c.Classify(plan, raiseAdvisory)

	if raisedResult.FinalLevel <= baseResult.FinalLevel {
		t.Errorf("advisory should raise: base=%v, raised=%v", baseResult.FinalLevel, raisedResult.FinalLevel)
	}

	// With advisory trying to lower (should be ignored).
	lowerAdvisory := &LLMAdvisory{SuggestedLevel: RiskSafe, Reason: "looks harmless"}
	loweredResult := c.Classify(plan, lowerAdvisory)

	if loweredResult.FinalLevel != baseResult.FinalLevel {
		t.Errorf("advisory should not lower: base=%v, lowered=%v", baseResult.FinalLevel, loweredResult.FinalLevel)
	}
}

func TestClassifier_UnknownBackend(t *testing.T) {
	c := NewClassifier()

	plan := Plan{
		Operations: []Operation{
			{Backend: "quantum_computer", Action: "entangle", Target: "qubit_array"},
		},
		Environment: "staging",
	}

	result := c.Classify(plan, nil)
	if result.FinalLevel != RiskDestructive {
		t.Errorf("unknown backend should default to destructive, got %v", result.FinalLevel)
	}
}

func TestClassifier_UnknownOperationDefaultsMaximum(t *testing.T) {
	c := NewClassifier()

	// Unknown DB operation.
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendDatabase, Action: "CLUSTER ON idx_name", Target: "users"},
		},
		Environment: "staging",
	}

	result := c.Classify(plan, nil)
	if result.FinalLevel != RiskDestructive {
		t.Errorf("unknown operation should default to destructive, got %v", result.FinalLevel)
	}
}

func TestClassifier_CitationsAlwaysPresent(t *testing.T) {
	c := NewClassifier()

	plans := []Plan{
		{}, // empty
		{Operations: []Operation{{Backend: BackendCode, Action: "add", Target: "new.go"}}},
		{Operations: []Operation{{Backend: BackendDatabase, Action: "DROP TABLE x", Target: "x"}}},
	}

	for i, plan := range plans {
		result := c.Classify(plan, nil)
		if len(result.Citations) == 0 {
			t.Errorf("plan[%d]: expected at least one citation, got none", i)
		}
	}
}

// ============================================================
// End-to-end scenarios: example plan per backend in each risk category
// ============================================================

func TestE2E_DatabaseRiskCategories(t *testing.T) {
	c := NewClassifier()

	categories := []struct {
		name  string
		plan  Plan
		level RiskLevel
	}{
		{"safe: select", Plan{Operations: []Operation{{Backend: BackendDatabase, Action: "SELECT 1", Target: ""}}}, RiskSafe},
		{"low: create table", Plan{Operations: []Operation{{Backend: BackendDatabase, Action: "CREATE TABLE t (id int)", Target: "t"}}}, RiskLow},
		{"reversible: add column", Plan{Operations: []Operation{{Backend: BackendDatabase, Action: "ALTER TABLE t ADD COLUMN x text", Target: "t"}}}, RiskReversible},
		{"high: alter column type", Plan{Operations: []Operation{{Backend: BackendDatabase, Action: "ALTER TABLE t ALTER COLUMN TYPE int", Target: "t"}}}, RiskHigh},
		{"destructive: drop table", Plan{Operations: []Operation{{Backend: BackendDatabase, Action: "DROP TABLE t", Target: "t"}}}, RiskDestructive},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.plan, nil)
			if result.BaseLevel != tt.level {
				t.Errorf("got base %v, want %v", result.BaseLevel, tt.level)
			}
		})
	}
}

func TestE2E_TerraformRiskCategories(t *testing.T) {
	c := NewClassifier()

	categories := []struct {
		name  string
		plan  Plan
		level RiskLevel
	}{
		{"safe: read", Plan{Operations: []Operation{{Backend: BackendTerraform, Action: "read", Target: "data.aws_ami.latest"}}}, RiskSafe},
		{"low: create", Plan{Operations: []Operation{{Backend: BackendTerraform, Action: "create", Target: "aws_iam_role.new"}}}, RiskLow},
		{"reversible: update", Plan{Operations: []Operation{{Backend: BackendTerraform, Action: "update", Target: "aws_iam_role.existing"}}}, RiskReversible},
		{"high: destroy stateless", Plan{Operations: []Operation{{Backend: BackendTerraform, Action: "destroy", Target: "aws_iam_role.old"}}}, RiskHigh},
		{"destructive: destroy stateful", Plan{Operations: []Operation{{Backend: BackendTerraform, Action: "destroy", Target: "aws_rds_instance.prod"}}}, RiskDestructive},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.plan, nil)
			if result.BaseLevel != tt.level {
				t.Errorf("got base %v, want %v", result.BaseLevel, tt.level)
			}
		})
	}
}

func TestE2E_CodeRiskCategories(t *testing.T) {
	c := NewClassifier()

	categories := []struct {
		name  string
		plan  Plan
		level RiskLevel
	}{
		{"safe: test file", Plan{Operations: []Operation{{Backend: BackendCode, Action: "edit", Target: "pkg/foo_test.go"}}}, RiskSafe},
		{"low: add file", Plan{Operations: []Operation{{Backend: BackendCode, Action: "add", Target: "internal/new.go"}}}, RiskLow},
		{"reversible: edit file", Plan{Operations: []Operation{{Backend: BackendCode, Action: "edit", Target: "internal/service.go"}}}, RiskReversible},
		{"high: startup path", Plan{Operations: []Operation{{Backend: BackendCode, Action: "edit", Target: "cmd/server/main.go"}}}, RiskHigh},
		{"destructive: security", Plan{Operations: []Operation{{Backend: BackendCode, Action: "edit", Target: "internal/auth/handler.go"}}}, RiskDestructive},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.plan, nil)
			if result.BaseLevel != tt.level {
				t.Errorf("got base %v, want %v", result.BaseLevel, tt.level)
			}
		})
	}
}

func TestE2E_ShellRiskCategories(t *testing.T) {
	c := NewClassifier()

	categories := []struct {
		name  string
		plan  Plan
		level RiskLevel
	}{
		{"safe: read-only", Plan{Operations: []Operation{{Backend: BackendShell, Action: "ls /app", Target: "/app", Details: map[string]any{}}}}, RiskSafe},
		{"low: create file", Plan{Operations: []Operation{{Backend: BackendShell, Action: "create file", Target: "/app/f", Details: map[string]any{}}}}, RiskLow},
		{"reversible: modify file", Plan{Operations: []Operation{{Backend: BackendShell, Action: "modify file", Target: "/app/f", Details: map[string]any{}}}}, RiskReversible},
		{"high: install package", Plan{Operations: []Operation{{Backend: BackendShell, Action: "install package", Target: "nginx", Details: map[string]any{}}}}, RiskHigh},
		{"destructive: rm -rf", Plan{Operations: []Operation{{Backend: BackendShell, Action: "rm -rf /data", Target: "/data", Details: map[string]any{}}}}, RiskDestructive},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.plan, nil)
			if result.BaseLevel != tt.level {
				t.Errorf("got base %v, want %v", result.BaseLevel, tt.level)
			}
		})
	}
}

func TestE2E_ExternalAPIRiskCategories(t *testing.T) {
	c := NewClassifier()

	categories := []struct {
		name  string
		plan  Plan
		level RiskLevel
	}{
		{"safe: GET", Plan{Operations: []Operation{{Backend: BackendExternalAPI, Action: "GET", Target: "/status", Details: map[string]any{}}}}, RiskSafe},
		{"low: POST create", Plan{Operations: []Operation{{Backend: BackendExternalAPI, Action: "POST", Target: "/resources", Details: map[string]any{}}}}, RiskLow},
		{"reversible: merge PR", Plan{Operations: []Operation{{Backend: BackendExternalAPI, Action: "PUT", Target: "/merge", Details: map[string]any{"integration": "github", "operation": "merge_pr"}}}}, RiskReversible},
		{"high: Stripe charge", Plan{Operations: []Operation{{Backend: BackendExternalAPI, Action: "POST", Target: "/charges", Details: map[string]any{"integration": "stripe", "operation": "create_charge"}}}}, RiskHigh},
		{"destructive: delete repo", Plan{Operations: []Operation{{Backend: BackendExternalAPI, Action: "DELETE", Target: "/repos/x/y", Details: map[string]any{"integration": "github", "operation": "delete_repo"}}}}, RiskDestructive},
	}

	for _, tt := range categories {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.plan, nil)
			if result.BaseLevel != tt.level {
				t.Errorf("got base %v, want %v", result.BaseLevel, tt.level)
			}
		})
	}
}

func TestE2E_FullPipeline(t *testing.T) {
	c := NewClassifier()

	// A mixed plan in production with PII service tag and LLM advisory.
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendDatabase, Action: "ALTER TABLE users ADD COLUMN phone text", Target: "users"},  // reversible
			{Backend: BackendCode, Action: "edit", Target: "internal/user/handler.go"},                       // reversible
			{Backend: BackendTerraform, Action: "update", Target: "aws_iam_policy.user_access"},             // reversible
		},
		Environment: "production",
		ServiceTags: []string{"pii"},
	}

	advisory := &LLMAdvisory{
		SuggestedLevel: RiskDestructive,
		Reason:         "Adding phone field to PII table in production requires extra caution",
	}

	result := c.Classify(plan, advisory)

	// Base should be reversible (highest of the three operations).
	if result.BaseLevel != RiskReversible {
		t.Errorf("base: got %v, want reversible", result.BaseLevel)
	}

	// After prod multiplier: reversible+1 = high.
	if result.EnvironmentMultiplied != RiskHigh {
		t.Errorf("env multiplied: got %v, want high", result.EnvironmentMultiplied)
	}

	// After PII tag: already high from env, PII floor is high → stays high.
	if result.SensitivityAdjusted != RiskHigh {
		t.Errorf("sensitivity: got %v, want high", result.SensitivityAdjusted)
	}

	// After LLM advisory: raised to destructive.
	if result.FinalLevel != RiskDestructive {
		t.Errorf("final: got %v, want destructive", result.FinalLevel)
	}

	// Must have citations.
	if len(result.Citations) == 0 {
		t.Error("expected citations")
	}
}

// ============================================================
// Model tests
// ============================================================

func TestRiskLevel_String(t *testing.T) {
	tests := []struct {
		level RiskLevel
		want  string
	}{
		{RiskSafe, "safe"},
		{RiskLow, "low"},
		{RiskReversible, "reversible"},
		{RiskHigh, "high"},
		{RiskDestructive, "destructive"},
		{RiskLevel(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("RiskLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestParseRiskLevel(t *testing.T) {
	tests := []struct {
		input string
		want  RiskLevel
	}{
		{"safe", RiskSafe},
		{"low", RiskLow},
		{"reversible", RiskReversible},
		{"high", RiskHigh},
		{"destructive", RiskDestructive},
		{"unknown_string", RiskDestructive}, // unknown defaults to max
	}

	for _, tt := range tests {
		if got := ParseRiskLevel(tt.input); got != tt.want {
			t.Errorf("ParseRiskLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ============================================================
// Policy evaluation tests (advancement policy engine integration)
// ============================================================

func TestPolicyThreshold_Allows(t *testing.T) {
	threshold := DefaultPolicyThresholds() // MaxAutoAdvance = RiskLow

	tests := []struct {
		name    string
		level   RiskLevel
		allowed bool
	}{
		{"safe allowed", RiskSafe, true},
		{"low allowed", RiskLow, true},
		{"reversible blocked", RiskReversible, false},
		{"high blocked", RiskHigh, false},
		{"destructive blocked", RiskDestructive, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Classification{FinalLevel: tt.level}
			if got := threshold.Allows(c); got != tt.allowed {
				t.Errorf("got %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestPolicyThreshold_CustomThreshold(t *testing.T) {
	// Allow up to reversible without human review.
	threshold := &PolicyThreshold{MaxAutoAdvance: RiskReversible}

	tests := []struct {
		name    string
		level   RiskLevel
		allowed bool
	}{
		{"safe allowed", RiskSafe, true},
		{"low allowed", RiskLow, true},
		{"reversible allowed", RiskReversible, true},
		{"high blocked", RiskHigh, false},
		{"destructive blocked", RiskDestructive, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Classification{FinalLevel: tt.level}
			if got := threshold.Allows(c); got != tt.allowed {
				t.Errorf("got %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestEvaluate_AllowsLowRisk(t *testing.T) {
	classifier := NewClassifier()
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendCode, Action: "add", Target: "internal/new.go"},
		},
		Environment: "staging",
	}

	result := Evaluate(classifier, plan, nil, nil)

	if !result.Allowed {
		t.Errorf("expected low-risk plan to be allowed, got blocked: %s", result.Reason)
	}
}

func TestEvaluate_BlocksHighRisk(t *testing.T) {
	classifier := NewClassifier()
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendDatabase, Action: "DROP TABLE users", Target: "users"},
		},
		Environment: "production",
	}

	result := Evaluate(classifier, plan, nil, nil)

	if result.Allowed {
		t.Errorf("expected destructive plan to be blocked, got allowed")
	}
	if result.Classification.FinalLevel < RiskHigh {
		t.Errorf("expected high+ risk, got %v", result.Classification.FinalLevel)
	}
}

func TestEvaluate_IncludesClassification(t *testing.T) {
	classifier := NewClassifier()
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendTerraform, Action: "create", Target: "aws_iam_role.new"},
		},
	}

	result := Evaluate(classifier, plan, nil, nil)

	if len(result.Classification.Citations) == 0 {
		t.Error("expected classification to include citations")
	}
}

func TestEvaluate_WithAdvisory(t *testing.T) {
	classifier := NewClassifier()
	plan := Plan{
		Operations: []Operation{
			{Backend: BackendCode, Action: "add", Target: "internal/new.go"},
		},
		Environment: "staging",
	}

	// LLM raises to high — should block with default threshold.
	advisory := &LLMAdvisory{SuggestedLevel: RiskHigh, Reason: "complex interaction"}
	result := Evaluate(classifier, plan, advisory, nil)

	if result.Allowed {
		t.Errorf("expected advisory-raised plan to be blocked")
	}
}
