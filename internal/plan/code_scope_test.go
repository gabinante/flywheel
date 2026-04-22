package plan

import (
	"testing"
)

func TestScopeChecker_AllowedFile(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
			{ID: "internal/plan/service.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	if v := sc.CheckFileAccess("internal/plan/model.go"); v != nil {
		t.Fatalf("expected no violation for declared file, got: %v", v)
	}
}

func TestScopeChecker_DisallowedFile(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	v := sc.CheckFileAccess("internal/risk/classifier.go")
	if v == nil {
		t.Fatal("expected violation for undeclared file")
	}
	if v.Type != "file" {
		t.Fatalf("expected violation type 'file', got: %s", v.Type)
	}
}

func TestScopeChecker_OperationMismatch(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	// "remove" on a file declared as "modify" is a violation.
	v := sc.CheckOperationType("internal/plan/model.go", "remove")
	if v == nil {
		t.Fatal("expected violation for operation mismatch")
	}
	if v.Type != "operation" {
		t.Fatalf("expected violation type 'operation', got: %s", v.Type)
	}
}

func TestScopeChecker_OperationCompatible(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	// "add" is compatible with "modify" (adding content within the same concept).
	if v := sc.CheckOperationType("internal/plan/model.go", "add"); v != nil {
		t.Fatalf("expected no violation for compatible operation, got: %v", v)
	}
}

func TestScopeChecker_RenameAllowsAddRemove(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "old_file.go", EntityType: "file", OperationType: "rename", NewID: "new_file.go"},
		},
		Diffs: []CodeDiff{
			{FilePath: "old_file.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "remove"}}},
		},
	}
	sc := NewScopeChecker(plan)

	// Rename allows "add" and "remove".
	if v := sc.CheckOperationType("old_file.go", "remove"); v != nil {
		t.Fatalf("expected no violation for remove on rename, got: %v", v)
	}
	if v := sc.CheckOperationType("new_file.go", "add"); v != nil {
		t.Fatalf("expected no violation for add on renamed target, got: %v", v)
	}
}

func TestScopeChecker_SymbolAccess(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "plan.Service.CreatePlan", EntityType: "symbol", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/service.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	if v := sc.CheckSymbolAccess("plan.Service.CreatePlan"); v != nil {
		t.Fatalf("expected no violation for declared symbol, got: %v", v)
	}

	v := sc.CheckSymbolAccess("plan.Service.DeletePlan")
	if v == nil {
		t.Fatal("expected violation for undeclared symbol")
	}
}

func TestScopeChecker_AllViolations(t *testing.T) {
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}
	sc := NewScopeChecker(plan)

	touchedFiles := map[string]string{
		"internal/plan/model.go":     "modify", // OK
		"internal/risk/classifier.go": "modify", // Violation: not declared
		"internal/plan/service.go":    "modify", // Violation: not declared
	}

	violations := sc.AllViolations(touchedFiles)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got: %d", len(violations))
	}
}

func TestScopeChecker_DiffFileAllowed(t *testing.T) {
	// Files in diffs are allowed even if not in target entities.
	plan := &CodePlan{
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
			{FilePath: "internal/plan/extra.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "add"}}},
		},
	}
	sc := NewScopeChecker(plan)

	// The diff file should be allowed.
	if v := sc.CheckFileAccess("internal/plan/extra.go"); v != nil {
		t.Fatalf("expected no violation for file in diffs, got: %v", v)
	}
}

func TestOperationCompatible(t *testing.T) {
	tests := []struct {
		declared   string
		actual     string
		compatible bool
	}{
		{"modify", "modify", true},
		{"modify", "add", true},
		{"modify", "remove", false},
		{"add", "add", true},
		{"add", "modify", false},
		{"remove", "remove", true},
		{"remove", "add", false},
		{"rename", "add", true},
		{"rename", "remove", true},
		{"rename", "modify", true},
	}

	for _, tt := range tests {
		got := operationCompatible(tt.declared, tt.actual)
		if got != tt.compatible {
			t.Errorf("operationCompatible(%q, %q) = %v, want %v", tt.declared, tt.actual, got, tt.compatible)
		}
	}
}
