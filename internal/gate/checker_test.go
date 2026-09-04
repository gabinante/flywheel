package gate

import (
	"context"
	"fmt"
	"testing"
)

// --- CheckerRegistry Tests ---

func TestCheckerRegistry_RoutesToCorrectChecker(t *testing.T) {
	registry := NewCheckerRegistry()

	called := map[GateRequirementType]bool{}
	registry.Register(RequireGitHubChecks, &stubChecker{satisfied: true, typ: RequireGitHubChecks, called: &called})
	registry.Register(RequireHumanApproval, &stubChecker{satisfied: false, typ: RequireHumanApproval, called: &called})

	reqs := []GateRequirement{
		{Type: RequireGitHubChecks},
		{Type: RequireHumanApproval},
	}

	statuses := registry.CheckAll(context.Background(), reqs, CheckContext{TicketID: "t-1"})
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	if !statuses[0].Satisfied {
		t.Error("github_checks should be satisfied")
	}
	if statuses[1].Satisfied {
		t.Error("human_approval should not be satisfied")
	}
	if !called[RequireGitHubChecks] || !called[RequireHumanApproval] {
		t.Error("expected both checkers to be called")
	}
}

func TestCheckerRegistry_UnregisteredType(t *testing.T) {
	registry := NewCheckerRegistry()
	reqs := []GateRequirement{{Type: "unknown_type"}}

	statuses := registry.CheckAll(context.Background(), reqs, CheckContext{})
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Satisfied {
		t.Error("unregistered type should not be satisfied")
	}
}

func TestCheckerRegistry_EmptyRequirements(t *testing.T) {
	registry := NewCheckerRegistry()
	statuses := registry.CheckAll(context.Background(), nil, CheckContext{})
	if statuses != nil {
		t.Errorf("expected nil for empty requirements, got %v", statuses)
	}
}

// --- Unsatisfied helper ---

func TestUnsatisfied(t *testing.T) {
	statuses := []GateRequirementStatus{
		{Requirement: GateRequirement{Type: RequireGitHubChecks}, Satisfied: true},
		{Requirement: GateRequirement{Type: RequireHumanApproval}, Satisfied: false, Reason: "no review"},
	}
	unsatisfied := Unsatisfied(statuses)
	if len(unsatisfied) != 1 {
		t.Fatalf("expected 1 unsatisfied, got %d", len(unsatisfied))
	}
	if unsatisfied[0].Requirement.Type != RequireHumanApproval {
		t.Errorf("expected human_approval, got %s", unsatisfied[0].Requirement.Type)
	}
}

// --- HumanApprovalChecker Tests ---

func TestHumanApprovalChecker_Approved(t *testing.T) {
	checker := &HumanApprovalChecker{
		Reviews: &mockReviewGetter{approved: true},
	}

	status := checker.Check(context.Background(), GateRequirement{Type: RequireHumanApproval}, CheckContext{TicketID: "t-1"})
	if !status.Satisfied {
		t.Error("expected satisfied when review is approved")
	}
}

func TestHumanApprovalChecker_NotApproved(t *testing.T) {
	checker := &HumanApprovalChecker{
		Reviews: &mockReviewGetter{approved: false},
	}

	status := checker.Check(context.Background(), GateRequirement{Type: RequireHumanApproval}, CheckContext{TicketID: "t-1"})
	if status.Satisfied {
		t.Error("expected not satisfied when no approved review")
	}
}

func TestHumanApprovalChecker_Error(t *testing.T) {
	checker := &HumanApprovalChecker{
		Reviews: &mockReviewGetter{err: fmt.Errorf("db error")},
	}

	status := checker.Check(context.Background(), GateRequirement{Type: RequireHumanApproval}, CheckContext{TicketID: "t-1"})
	if status.Satisfied {
		t.Error("expected not satisfied on error")
	}
}

func TestHumanApprovalChecker_NilReviews(t *testing.T) {
	checker := &HumanApprovalChecker{}

	status := checker.Check(context.Background(), GateRequirement{Type: RequireHumanApproval}, CheckContext{TicketID: "t-1"})
	if status.Satisfied {
		t.Error("expected not satisfied with nil reviews")
	}
}

// --- GitHubChecksChecker Tests ---

func TestGitHubChecksChecker_NoPRURL(t *testing.T) {
	checker := &GitHubChecksChecker{}
	status := checker.Check(context.Background(), GateRequirement{Type: RequireGitHubChecks}, CheckContext{})
	if status.Satisfied {
		t.Error("expected not satisfied when no PR URL")
	}
	if status.Reason != "no PR URL available" {
		t.Errorf("unexpected reason: %s", status.Reason)
	}
}

// --- Test helpers ---

type stubChecker struct {
	satisfied bool
	typ       GateRequirementType
	called    *map[GateRequirementType]bool
}

func (c *stubChecker) Check(_ context.Context, req GateRequirement, _ CheckContext) GateRequirementStatus {
	(*c.called)[c.typ] = true
	return GateRequirementStatus{
		Requirement: req,
		Satisfied:   c.satisfied,
		Reason:      "stub",
	}
}

type mockReviewGetter struct {
	approved bool
	err      error
}

func (m *mockReviewGetter) HasApprovedReview(_ context.Context, _ string) (bool, error) {
	return m.approved, m.err
}
