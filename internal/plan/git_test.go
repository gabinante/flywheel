package plan

import (
	"strings"
	"testing"
)

func TestDefaultGitPolicy(t *testing.T) {
	p := DefaultGitPolicy()
	if p.BranchPrefix != "ticket/" {
		t.Fatalf("expected branch prefix 'ticket/', got: %s", p.BranchPrefix)
	}
	if p.BaseBranch != "main" {
		t.Fatalf("expected base branch 'main', got: %s", p.BaseBranch)
	}
	if !p.RequirePR {
		t.Fatal("expected RequirePR to be true by default")
	}
	if !p.RequireReview {
		t.Fatal("expected RequireReview to be true by default")
	}
	if !p.RequireCI {
		t.Fatal("expected RequireCI to be true by default")
	}
	if p.AutoMerge {
		t.Fatal("expected AutoMerge to be false by default")
	}
}

func TestBranchNameForTicket(t *testing.T) {
	p := DefaultGitPolicy()
	if got := p.BranchNameForTicket("warrant-49"); got != "ticket/warrant-49" {
		t.Fatalf("expected 'ticket/warrant-49', got: %s", got)
	}

	p.BranchPrefix = "feature/"
	if got := p.BranchNameForTicket("warrant-49"); got != "feature/warrant-49" {
		t.Fatalf("expected 'feature/warrant-49', got: %s", got)
	}
}

func TestCommitTagForTicket(t *testing.T) {
	p := DefaultGitPolicy()
	if got := p.CommitTagForTicket("warrant-49"); got != "ticket/warrant-49" {
		t.Fatalf("expected 'ticket/warrant-49', got: %s", got)
	}
}

func TestFormatCommitMessage(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
		SymbolSnapshots: []SymbolSnapshot{
			{SymbolID: "plan.CodePlan", FilePath: "internal/plan/model.go", Kind: "type", Exported: true, BeforeSignature: "old", AfterSignature: "new"},
		},
	}

	msg := FormatCommitMessage(plan, "warrant-49", "Enhance CodePlan schema")

	if !strings.Contains(msg, "warrant-49") {
		t.Fatal("commit message should contain ticket ID")
	}
	if !strings.Contains(msg, "internal/plan/model.go") {
		t.Fatal("commit message should list affected files")
	}
	if !strings.Contains(msg, "plan.CodePlan") {
		t.Fatal("commit message should list affected symbols")
	}
	if !strings.Contains(msg, "Plan-Ticket:") {
		t.Fatal("commit message should contain Plan-Ticket trailer")
	}
}

func TestFormatPRBody(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "internal/plan/model.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "internal/plan/model.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
		SymbolSnapshots: []SymbolSnapshot{
			{SymbolID: "plan.CodePlan", FilePath: "internal/plan/model.go", Kind: "type", Exported: true, BeforeSignature: "old", AfterSignature: "new"},
		},
		TestExpectations: &TestExpectations{
			TestCommands: []string{"go test ./internal/plan/..."},
		},
		RollbackPlan: &CodeRollbackPlan{
			Strategy:        "git_revert",
			RevertCommitRef: "abc123",
		},
	}

	body := FormatPRBody(plan, "warrant-49", "Enhance CodePlan schema")

	if !strings.Contains(body, "## Summary") {
		t.Fatal("PR body should contain Summary section")
	}
	if !strings.Contains(body, "## Changes") {
		t.Fatal("PR body should contain Changes section")
	}
	if !strings.Contains(body, "exported symbol") {
		t.Fatal("PR body should warn about exported symbol changes")
	}
	if !strings.Contains(body, "## Test Plan") {
		t.Fatal("PR body should contain Test Plan section")
	}
	if !strings.Contains(body, "## Rollback") {
		t.Fatal("PR body should contain Rollback section")
	}
	if !strings.Contains(body, "Plan-Ticket: warrant-49") {
		t.Fatal("PR body should contain Plan-Ticket reference")
	}
}

func TestGenerateGitInstructions(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "main.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "main.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}

	instructions := GenerateGitInstructions(plan, "warrant-49", "Test change", nil)

	// Should have: create_branch, commit, push, create_pr (4 instructions).
	if len(instructions) != 4 {
		t.Fatalf("expected 4 git instructions, got: %d", len(instructions))
	}

	if instructions[0].Operation != "create_branch" {
		t.Fatalf("expected first instruction to be create_branch, got: %s", instructions[0].Operation)
	}
	if instructions[0].Args["branch"] != "ticket/warrant-49" {
		t.Fatalf("expected branch 'ticket/warrant-49', got: %s", instructions[0].Args["branch"])
	}

	if instructions[1].Operation != "commit" {
		t.Fatalf("expected second instruction to be commit, got: %s", instructions[1].Operation)
	}

	if instructions[2].Operation != "push" {
		t.Fatalf("expected third instruction to be push, got: %s", instructions[2].Operation)
	}

	if instructions[3].Operation != "create_pr" {
		t.Fatalf("expected fourth instruction to be create_pr, got: %s", instructions[3].Operation)
	}
}

func TestGenerateGitInstructions_ExistingBranch(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "main.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "main.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
		GitContext: &GitContext{
			Branch: "existing-branch",
		},
	}

	instructions := GenerateGitInstructions(plan, "warrant-49", "Test change", nil)

	// First instruction should be checkout (not create_branch).
	if instructions[0].Operation != "checkout" {
		t.Fatalf("expected checkout for existing branch, got: %s", instructions[0].Operation)
	}
	if instructions[0].Args["branch"] != "existing-branch" {
		t.Fatalf("expected branch 'existing-branch', got: %s", instructions[0].Args["branch"])
	}
}

func TestGenerateGitInstructions_NoPRPolicy(t *testing.T) {
	plan := &CodePlan{
		Language: "go",
		TargetEntities: []TargetEntity{
			{ID: "main.go", EntityType: "file", OperationType: "modify"},
		},
		Diffs: []CodeDiff{
			{FilePath: "main.go", Hunks: []CodeHunk{{StartLine: 1, EndLine: 5, Content: "x", Operation: "modify"}}},
		},
	}

	policy := DefaultGitPolicy()
	policy.RequirePR = false

	instructions := GenerateGitInstructions(plan, "warrant-49", "Test change", policy)

	// Should have: create_branch, commit, push (3 instructions, no create_pr).
	if len(instructions) != 3 {
		t.Fatalf("expected 3 git instructions (no PR), got: %d", len(instructions))
	}
	for _, inst := range instructions {
		if inst.Operation == "create_pr" {
			t.Fatal("should not have create_pr when RequirePR is false")
		}
	}
}
