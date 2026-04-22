package plan

import (
	"testing"

	"github.com/gabinante/flywheel/internal/risk"
)

func TestClassifyCodePlan_SafeTestChange(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model_test.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/model_test.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "func TestNewFeature(t *testing.T) {}", Operation: "add", SymbolScope: "test:TestNewFeature"},
				},
			},
		},
	}

	classification := ClassifyCodePlan(plan, "development", nil)
	if classification.FinalLevel > risk.RiskLow {
		t.Fatalf("expected safe/low classification for test file, got: %s", classification.FinalLevel)
	}
}

func TestClassifyCodePlan_SecuritySensitive(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/auth/handler.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/auth/handler.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "func Authenticate() {}", Operation: "modify"},
				},
			},
		},
	}

	classification := ClassifyCodePlan(plan, "production", nil)
	// Security-sensitive code should be classified as destructive.
	if classification.FinalLevel < risk.RiskDestructive {
		t.Fatalf("expected destructive classification for auth code, got: %s", classification.FinalLevel)
	}
}

func TestClassifyCodePlan_ExportedSymbolChange(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/service.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/service.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "func (s *Service) NewMethod() {}", Operation: "add"},
				},
			},
		},
		SymbolSnapshots: []SymbolSnapshot{
			{
				SymbolID:        "plan.Service.CreatePlan",
				FilePath:        "internal/plan/service.go",
				Kind:            "method",
				Exported:        true,
				BeforeSignature: "func (s *Service) CreatePlan(ctx context.Context, ticketID string) (*Plan, error)",
				AfterSignature:  "func (s *Service) CreatePlan(ctx context.Context, ticketID string, opts ...Option) (*Plan, error)",
				Callers:         10,
			},
		},
	}

	classification := ClassifyCodePlan(plan, "production", nil)
	// Exported symbol signature change with many callers + prod should be high risk.
	if classification.FinalLevel < risk.RiskHigh {
		t.Fatalf("expected at least high classification for exported symbol change in prod, got: %s", classification.FinalLevel)
	}
}

func TestClassifyCodePlan_NewInternalFunction(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/helper.go", EntityType: "file", OperationType: "add"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/helper.go",
				Hunks: []CodeHunk{
					{StartLine: 1, EndLine: 10, Content: "func helper() {}", Operation: "add"},
				},
			},
		},
	}

	classification := ClassifyCodePlan(plan, "development", nil)
	// New internal file in dev should be low risk.
	if classification.FinalLevel > risk.RiskReversible {
		t.Fatalf("expected low/reversible classification for new internal file in dev, got: %s", classification.FinalLevel)
	}
}

func TestClassifyCodePlan_EnvironmentMultiplier(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/service.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/service.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "// updated", Operation: "modify"},
				},
			},
		},
	}

	devClass := ClassifyCodePlan(plan, "development", nil)
	prodClass := ClassifyCodePlan(plan, "production", nil)

	// Production should be at least as high risk as development.
	if prodClass.FinalLevel < devClass.FinalLevel {
		t.Fatalf("expected production risk >= development risk, got prod=%s dev=%s", prodClass.FinalLevel, devClass.FinalLevel)
	}
}

func TestClassifyCodePlan_ServiceSensitivity(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/service.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/service.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "// update", Operation: "modify"},
				},
			},
		},
	}

	withTags := ClassifyCodePlan(plan, "production", []string{"pii", "financial"})
	withoutTags := ClassifyCodePlan(plan, "production", nil)

	// PII/financial tags should increase classification.
	if withTags.FinalLevel < withoutTags.FinalLevel {
		t.Fatalf("expected sensitivity tags to increase risk, got with=%s without=%s", withTags.FinalLevel, withoutTags.FinalLevel)
	}
}

func TestIsTreeSitterSupported(t *testing.T) {
	tests := []struct {
		language  string
		supported bool
	}{
		{"go", true},
		{"typescript", true},
		{"python", true},
		{"rust", true},
		{"java", true},
		{"cobol", false},
		{"fortran", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsTreeSitterSupported(tt.language); got != tt.supported {
			t.Errorf("IsTreeSitterSupported(%q) = %v, want %v", tt.language, got, tt.supported)
		}
	}
}

func TestCodeAutoAdvanceEligible_SafeChange(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model_test.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/plan/model_test.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "// new test", Operation: "add", SymbolScope: "test:TestNew"},
				},
			},
		},
	}

	eligible := CodeAutoAdvanceEligible(plan, "development", nil)
	if !eligible {
		t.Fatal("expected safe test change in dev to be auto-advance eligible")
	}
}

func TestCodeAutoAdvanceEligible_DestructiveChange(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/auth/handler.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{
				FilePath: "internal/auth/handler.go",
				Hunks: []CodeHunk{
					{StartLine: 10, EndLine: 20, Content: "func Authenticate() {}", Operation: "modify"},
				},
			},
		},
	}

	eligible := CodeAutoAdvanceEligible(plan, "production", []string{"pii"})
	if eligible {
		t.Fatal("expected destructive auth change in prod to NOT be auto-advance eligible")
	}
}

func TestInferFileScope(t *testing.T) {
	tests := []struct {
		filePath string
		expected string
	}{
		{"internal/plan/model_test.go", "test"},
		{"README.md", "documentation"},
		{"internal/auth/handler.go", "security"},
		{"cmd/server/main.go", "startup"},
		{"internal/plan/service.go", "internal"},
		{"some/random/file.go", ""},
	}

	for _, tt := range tests {
		got := inferFileScope(tt.filePath)
		if got != tt.expected {
			t.Errorf("inferFileScope(%q) = %q, want %q", tt.filePath, got, tt.expected)
		}
	}
}

func TestInferSymbolScope(t *testing.T) {
	tests := []struct {
		name     string
		snapshot SymbolSnapshot
		expected string
	}{
		{
			"exported with signature change and many callers",
			SymbolSnapshot{SymbolID: "Service.Create", Exported: true, BeforeSignature: "func() error", AfterSignature: "func(ctx) error", Callers: 10},
			"public_api",
		},
		{
			"exported without change",
			SymbolSnapshot{SymbolID: "Service.Create", Exported: true, BeforeSignature: "func()", AfterSignature: "func()"},
			"exported",
		},
		{
			"private symbol",
			SymbolSnapshot{SymbolID: "helper", Exported: false, BeforeSignature: "func()", AfterSignature: "func()"},
			"internal",
		},
		{
			"security-related",
			SymbolSnapshot{SymbolID: "AuthMiddleware", Exported: false, BeforeSignature: "func()", AfterSignature: "func()"},
			"security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferSymbolScope(tt.snapshot)
			if got != tt.expected {
				t.Errorf("inferSymbolScope() = %q, want %q", got, tt.expected)
			}
		})
	}
}
