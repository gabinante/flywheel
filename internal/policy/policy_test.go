package policy

import (
	"context"
	"testing"

	"github.com/gabinante/flywheel/events"
)

// --- Action Restrictiveness Tests ---

func TestActionRestrictiveness(t *testing.T) {
	ordered := AllActions()
	for i := 1; i < len(ordered); i++ {
		if ordered[i].Restrictiveness() <= ordered[i-1].Restrictiveness() {
			t.Errorf("expected %s (%d) > %s (%d)",
				ordered[i], ordered[i].Restrictiveness(),
				ordered[i-1], ordered[i-1].Restrictiveness())
		}
	}
}

func TestMoreRestrictive(t *testing.T) {
	tests := []struct {
		a, b     Action
		expected Action
	}{
		{ActionAuto, ActionApprove, ActionApprove},
		{ActionApprove, ActionAuto, ActionApprove},
		{ActionNotify, ActionNotify, ActionNotify},
		{ActionHumanRequired, ActionAuto, ActionHumanRequired},
		{ActionPlanOnly, ActionOpenPRStop, ActionOpenPRStop},
		{ActionTypedConfirm, ActionApprove, ActionTypedConfirm},
	}
	for _, tt := range tests {
		got := MoreRestrictive(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("MoreRestrictive(%s, %s) = %s, want %s", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestActionIsValid(t *testing.T) {
	for _, a := range AllActions() {
		if !a.IsValid() {
			t.Errorf("expected %s to be valid", a)
		}
	}
	if Action("unknown").IsValid() {
		t.Error("expected unknown action to be invalid")
	}
}

// --- Engine Evaluate Tests ---

func TestEngineEvaluate_NoRules_DefaultApprove(t *testing.T) {
	engine := NewEngine()
	decision := engine.Evaluate(nil, TransitionContext{})
	if decision.Action != ActionApprove {
		t.Errorf("got %s, want %s", decision.Action, ActionApprove)
	}
	if decision.EffectiveRule == nil || decision.EffectiveRule.RuleID != "_default" {
		t.Error("expected conservative default rule")
	}
}

func TestEngineEvaluate_SingleRule(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:      "r1",
			Name:    "auto-all",
			Action:  ActionAuto,
			Enabled: true,
		},
	}
	decision := engine.Evaluate(rules, TransitionContext{})
	if decision.Action != ActionAuto {
		t.Errorf("got %s, want %s", decision.Action, ActionAuto)
	}
	if len(decision.MatchedRules) != 1 {
		t.Errorf("expected 1 matched rule, got %d", len(decision.MatchedRules))
	}
}

func TestEngineEvaluate_MostRestrictiveWins(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:      "r1",
			Name:    "auto-all",
			Action:  ActionAuto,
			Enabled: true,
		},
		{
			ID:      "r2",
			Name:    "prod-approve",
			Action:  ActionApprove,
			Enabled: true,
			Predicates: []Predicate{
				{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
			},
		},
	}

	// Non-prod: only r1 matches → auto.
	decision := engine.Evaluate(rules, TransitionContext{Environment: "development"})
	if decision.Action != ActionAuto {
		t.Errorf("dev: got %s, want %s", decision.Action, ActionAuto)
	}

	// Prod: both match → most restrictive (approve) wins.
	decision = engine.Evaluate(rules, TransitionContext{Environment: "production"})
	if decision.Action != ActionApprove {
		t.Errorf("prod: got %s, want %s", decision.Action, ActionApprove)
	}
	if len(decision.MatchedRules) != 2 {
		t.Errorf("prod: expected 2 matched rules, got %d", len(decision.MatchedRules))
	}
}

func TestEngineEvaluate_DisabledRulesSkipped(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:      "r1",
			Name:    "approve-all",
			Action:  ActionApprove,
			Enabled: false, // disabled
		},
		{
			ID:      "r2",
			Name:    "auto-all",
			Action:  ActionAuto,
			Enabled: true,
		},
	}
	decision := engine.Evaluate(rules, TransitionContext{})
	if decision.Action != ActionAuto {
		t.Errorf("got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestEngineEvaluate_MultiPredicateAND(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "prod-deploy-confirm",
			Predicates: []Predicate{
				{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
				{Field: FieldTransition, Operator: OpEquals, Values: []string{"close"}},
			},
			Action:  ActionTypedConfirm,
			Enabled: true,
		},
	}

	// Only env matches → no match.
	decision := engine.Evaluate(rules, TransitionContext{
		Environment: "production",
		Transition:  "submit",
	})
	if decision.Action != ActionApprove { // falls to conservative default
		t.Errorf("got %s, want %s (conservative default)", decision.Action, ActionApprove)
	}

	// Both match.
	decision = engine.Evaluate(rules, TransitionContext{
		Environment: "production",
		Transition:  "close",
	})
	if decision.Action != ActionTypedConfirm {
		t.Errorf("got %s, want %s", decision.Action, ActionTypedConfirm)
	}
}

func TestEngineEvaluate_InOperator(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "auto-dev-staging",
			Predicates: []Predicate{
				{Field: FieldEnvironment, Operator: OpIn, Values: []string{"development", "staging"}},
			},
			Action:  ActionAuto,
			Enabled: true,
		},
	}

	for _, env := range []string{"development", "staging"} {
		decision := engine.Evaluate(rules, TransitionContext{Environment: env})
		if decision.Action != ActionAuto {
			t.Errorf("%s: got %s, want %s", env, decision.Action, ActionAuto)
		}
	}

	decision := engine.Evaluate(rules, TransitionContext{Environment: "production"})
	if decision.Action != ActionApprove { // no match → conservative default
		t.Errorf("prod: got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestEngineEvaluate_NotInOperator(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "not-prod",
			Predicates: []Predicate{
				{Field: FieldEnvironment, Operator: OpNotIn, Values: []string{"production"}},
			},
			Action:  ActionAuto,
			Enabled: true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{Environment: "development"})
	if decision.Action != ActionAuto {
		t.Errorf("dev: got %s, want %s", decision.Action, ActionAuto)
	}

	decision = engine.Evaluate(rules, TransitionContext{Environment: "production"})
	if decision.Action != ActionApprove { // no match
		t.Errorf("prod: got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestEngineEvaluate_ServiceGlobMatching(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "payments-approve",
			Predicates: []Predicate{
				{Field: FieldService, Operator: OpMatches, Values: []string{"payments*", "auth*"}},
			},
			Action:  ActionApprove,
			Enabled: true,
		},
		{
			ID:     "r2",
			Name:   "default-auto",
			Action: ActionAuto,
			Enabled: true,
		},
	}

	// Matching service.
	decision := engine.Evaluate(rules, TransitionContext{Services: []string{"payments-api"}})
	if decision.Action != ActionApprove {
		t.Errorf("payments: got %s, want %s", decision.Action, ActionApprove)
	}

	// Non-matching service.
	decision = engine.Evaluate(rules, TransitionContext{Services: []string{"frontend"}})
	if decision.Action != ActionAuto {
		t.Errorf("frontend: got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestEngineEvaluate_RiskLevel(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "critical-confirm",
			Predicates: []Predicate{
				{Field: FieldRiskLevel, Operator: OpEquals, Values: []string{"critical"}},
			},
			Action:  ActionTypedConfirm,
			Enabled: true,
		},
		{
			ID:   "r2",
			Name: "low-auto",
			Predicates: []Predicate{
				{Field: FieldRiskLevel, Operator: OpIn, Values: []string{"low", "medium"}},
			},
			Action:  ActionAuto,
			Enabled: true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{RiskLevel: "critical"})
	if decision.Action != ActionTypedConfirm {
		t.Errorf("critical: got %s, want %s", decision.Action, ActionTypedConfirm)
	}

	decision = engine.Evaluate(rules, TransitionContext{RiskLevel: "low"})
	if decision.Action != ActionAuto {
		t.Errorf("low: got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestEngineEvaluate_TicketPriority(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:   "r1",
			Name: "p0-approve",
			Predicates: []Predicate{
				{Field: FieldTicketPriority, Operator: OpEquals, Values: []string{"0"}},
			},
			Action:  ActionApprove,
			Enabled: true,
		},
		{
			ID:     "r2",
			Name:   "default-auto",
			Action: ActionAuto,
			Enabled: true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{TicketPriority: 0})
	if decision.Action != ActionApprove {
		t.Errorf("P0: got %s, want %s", decision.Action, ActionApprove)
	}

	decision = engine.Evaluate(rules, TransitionContext{TicketPriority: 2})
	if decision.Action != ActionAuto {
		t.Errorf("P2: got %s, want %s", decision.Action, ActionAuto)
	}
}

// --- Engine Requirements Union Tests ---

func TestEngineEvaluate_RequirementsUnioned(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:           "r1",
			Name:         "auto-with-ci",
			Action:       ActionAuto,
			Requirements: []GateRequirement{{Type: RequireGitHubChecks}},
			Enabled:      true,
		},
		{
			ID:           "r2",
			Name:         "auto-with-human",
			Action:       ActionAuto,
			Requirements: []GateRequirement{{Type: RequireHumanApproval}},
			Enabled:      true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{})
	if len(decision.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(decision.Requirements))
	}
	types := map[GateRequirementType]bool{}
	for _, r := range decision.Requirements {
		types[r.Type] = true
	}
	if !types[RequireGitHubChecks] || !types[RequireHumanApproval] {
		t.Errorf("expected both github_checks and human_approval, got %v", decision.Requirements)
	}
}

func TestEngineEvaluate_RequirementsDeduplication(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:           "r1",
			Name:         "rule-a",
			Action:       ActionAuto,
			Requirements: []GateRequirement{{Type: RequireGitHubChecks}},
			Enabled:      true,
		},
		{
			ID:           "r2",
			Name:         "rule-b",
			Action:       ActionNotify,
			Requirements: []GateRequirement{{Type: RequireGitHubChecks}},
			Enabled:      true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{})
	if len(decision.Requirements) != 1 {
		t.Fatalf("expected 1 deduplicated requirement, got %d", len(decision.Requirements))
	}
	if decision.Requirements[0].Type != RequireGitHubChecks {
		t.Errorf("expected github_checks, got %s", decision.Requirements[0].Type)
	}
}

func TestEngineEvaluate_NoRequirements(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:      "r1",
			Name:    "auto-all",
			Action:  ActionAuto,
			Enabled: true,
		},
	}

	decision := engine.Evaluate(rules, TransitionContext{})
	if len(decision.Requirements) != 0 {
		t.Errorf("expected no requirements, got %d", len(decision.Requirements))
	}
}

func TestEngineEvaluateGates_RequirementsPropagated(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:      "r1",
			Name:    "auto-all",
			Action:  ActionAuto,
			Enabled: true,
		},
		{
			ID:   "r2",
			Name: "approve-with-ci",
			Predicates: []Predicate{
				{Field: FieldTransition, Operator: OpEquals, Values: []string{"approve"}},
			},
			Action:       ActionApprove,
			Requirements: []GateRequirement{{Type: RequireGitHubChecks}},
			Enabled:      true,
		},
	}

	path := []PathStep{
		{Trigger: "submit", FromState: "executing", ToState: "awaiting_validation"},
		{Trigger: "approve", FromState: "awaiting_validation", ToState: "validated"},
	}

	gates := engine.EvaluateGates(rules, TransitionContext{}, path)
	if len(gates) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(gates))
	}

	// submit gate: no requirements (r2 doesn't match)
	if len(gates[0].Requirements) != 0 {
		t.Errorf("submit gate: expected 0 requirements, got %d", len(gates[0].Requirements))
	}

	// approve gate: should have github_checks requirement
	if len(gates[1].Requirements) != 1 {
		t.Fatalf("approve gate: expected 1 requirement, got %d", len(gates[1].Requirements))
	}
	if gates[1].Requirements[0].Type != RequireGitHubChecks {
		t.Errorf("approve gate: expected github_checks, got %s", gates[1].Requirements[0].Type)
	}
}

// --- EvaluateGates Tests ---

func TestEngineEvaluateGates(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{
			ID:     "r1",
			Name:   "auto-all",
			Action: ActionAuto,
			Enabled: true,
		},
		{
			ID:   "r2",
			Name: "deploy-approve",
			Predicates: []Predicate{
				{Field: FieldTransition, Operator: OpEquals, Values: []string{"close"}},
			},
			Action:  ActionApprove,
			Enabled: true,
		},
	}

	baseCtx := TransitionContext{Environment: "production"}
	path := []PathStep{
		{Trigger: "submit", FromState: "executing", ToState: "awaiting_validation"},
		{Trigger: "approve", FromState: "awaiting_validation", ToState: "validated"},
		{Trigger: "close", FromState: "validated", ToState: "closed"},
	}

	gates := engine.EvaluateGates(rules, baseCtx, path)
	if len(gates) != 3 {
		t.Fatalf("expected 3 gates, got %d", len(gates))
	}

	// submit: auto (only r1 matches)
	if gates[0].Action != ActionAuto {
		t.Errorf("submit gate: got %s, want %s", gates[0].Action, ActionAuto)
	}
	// deploy: approve (r1 + r2 match, approve is more restrictive)
	if gates[2].Action != ActionApprove {
		t.Errorf("deploy gate: got %s, want %s", gates[2].Action, ActionApprove)
	}
}

// --- Happy Path Tests ---

func TestRemainingHappyPath(t *testing.T) {
	remaining := RemainingHappyPath("executing")
	if len(remaining) == 0 {
		t.Fatal("expected non-empty remaining path from executing")
	}
	if remaining[0].Trigger != "submit" {
		t.Errorf("first remaining trigger = %s, want submit", remaining[0].Trigger)
	}

	// From closed: no remaining path.
	remaining = RemainingHappyPath("closed")
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining path from closed, got %d", len(remaining))
	}
}

func TestFullHappyPath(t *testing.T) {
	path := FullHappyPath()
	if len(path) != 5 {
		t.Errorf("expected 5 steps, got %d", len(path))
	}
	if path[0].Trigger != "claim" || path[0].FromState != "draft" {
		t.Errorf("first step: %+v", path[0])
	}
	if path[4].Trigger != "close" || path[4].ToState != "closed" {
		t.Errorf("last step: %+v", path[4])
	}
}

// --- Posture Tests ---

func TestAllPostures(t *testing.T) {
	postures := AllPostures()
	if len(postures) != 5 {
		t.Fatalf("expected 5 postures, got %d", len(postures))
	}

	names := map[string]bool{}
	for _, p := range postures {
		names[p.Name] = true
		if p.DisplayName == "" {
			t.Errorf("posture %s has empty display name", p.Name)
		}
		if p.Description == "" {
			t.Errorf("posture %s has empty description", p.Name)
		}
		if len(p.Rules) == 0 {
			t.Errorf("posture %s has no rules", p.Name)
		}
		// Validate all rules in each posture.
		for _, r := range p.Rules {
			if err := r.Validate(); err != nil {
				t.Errorf("posture %s rule %q: %v", p.Name, r.Name, err)
			}
		}
	}

	required := []string{"plan-only", "sandbox", "prod-gate", "graduated-risk", "paranoid-service"}
	for _, name := range required {
		if !names[name] {
			t.Errorf("missing required posture: %s", name)
		}
	}
}

func TestGetPosture(t *testing.T) {
	p := GetPosture("plan-only")
	if p == nil {
		t.Fatal("expected plan-only posture")
	}
	if !p.IsDefault {
		t.Error("plan-only should be the default posture")
	}

	p = GetPosture("nonexistent")
	if p != nil {
		t.Error("expected nil for nonexistent posture")
	}
}

func TestDefaultPostureName(t *testing.T) {
	name := DefaultPostureName()
	p := GetPosture(name)
	if p == nil {
		t.Errorf("default posture %q not found", name)
	}
}

// --- Posture Behavior Tests ---

func TestPosturePlanOnly_AllTransitionsPlanOnly(t *testing.T) {
	engine := NewEngine()
	posture := PosturePlanOnly()

	for _, step := range FullHappyPath() {
		decision := engine.Evaluate(posture.Rules, TransitionContext{
			Transition:  step.Trigger,
			Environment: "production",
		})
		if decision.Action != ActionPlanOnly {
			t.Errorf("plan-only posture, trigger %s: got %s, want %s",
				step.Trigger, decision.Action, ActionPlanOnly)
		}
	}
}

func TestPostureProdGate_DevAutomatic(t *testing.T) {
	engine := NewEngine()
	posture := PostureProdGate()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Environment: "development",
		Transition:  "close",
	})
	if decision.Action != ActionAuto {
		t.Errorf("prod-gate dev deploy: got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestPostureProdGate_ProdApproval(t *testing.T) {
	engine := NewEngine()
	posture := PostureProdGate()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Environment: "production",
		Transition:  "submit",
	})
	if decision.Action != ActionApprove {
		t.Errorf("prod-gate prod submit: got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestPostureProdGate_ProdApproveWithCI(t *testing.T) {
	engine := NewEngine()
	posture := PostureProdGate()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Environment: "production",
		Transition:  "approve",
	})
	if decision.Action != ActionApprove {
		t.Errorf("prod-gate prod approve: got %s, want %s", decision.Action, ActionApprove)
	}
	// The prod-approve-with-ci rule should add a github_checks requirement.
	hasCI := false
	for _, r := range decision.Requirements {
		if r.Type == RequireGitHubChecks {
			hasCI = true
		}
	}
	if !hasCI {
		t.Error("prod-gate prod approve: expected github_checks requirement")
	}
}

func TestPostureProdGate_ProdDeployTypedConfirm(t *testing.T) {
	engine := NewEngine()
	posture := PostureProdGate()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Environment: "production",
		Transition:  "close",
	})
	// Both prod-approve and prod-deploy-typed-confirm match.
	// typed-confirm is more restrictive than approve.
	if decision.Action != ActionTypedConfirm {
		t.Errorf("prod-gate prod deploy: got %s, want %s", decision.Action, ActionTypedConfirm)
	}
}

func TestPostureSandbox_CloseApprove(t *testing.T) {
	engine := NewEngine()
	posture := PostureSandbox()

	decision := engine.Evaluate(posture.Rules, TransitionContext{Transition: "close"})
	if decision.Action != ActionApprove {
		t.Errorf("sandbox close: got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestPostureSandbox_SubmitAuto(t *testing.T) {
	engine := NewEngine()
	posture := PostureSandbox()

	decision := engine.Evaluate(posture.Rules, TransitionContext{Transition: "submit"})
	if decision.Action != ActionAuto {
		t.Errorf("sandbox submit: got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestPostureGraduatedRisk_LowReversibleAuto(t *testing.T) {
	engine := NewEngine()
	posture := PostureGraduatedRisk()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		RiskLevel:      "low",
		RiskReversible: "true",
	})
	if decision.Action != ActionAuto {
		t.Errorf("graduated-risk low/reversible: got %s, want %s", decision.Action, ActionAuto)
	}
}

func TestPostureGraduatedRisk_CriticalConfirm(t *testing.T) {
	engine := NewEngine()
	posture := PostureGraduatedRisk()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		RiskLevel: "critical",
	})
	if decision.Action != ActionTypedConfirm {
		t.Errorf("graduated-risk critical: got %s, want %s", decision.Action, ActionTypedConfirm)
	}
}

func TestPostureParanoidService_CriticalServiceApprove(t *testing.T) {
	engine := NewEngine()
	posture := PostureParanoidService()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Services:    []string{"payments-api"},
		Environment: "development",
		Transition:  "start",
	})
	// Both critical-services-approve and dev-staging-auto match.
	// approve is more restrictive than auto.
	if decision.Action != ActionApprove {
		t.Errorf("paranoid-service payments dev: got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestPostureParanoidService_NonCriticalDevAuto(t *testing.T) {
	engine := NewEngine()
	posture := PostureParanoidService()

	decision := engine.Evaluate(posture.Rules, TransitionContext{
		Services:    []string{"frontend-app"},
		Environment: "development",
		Transition:  "start",
	})
	if decision.Action != ActionAuto {
		t.Errorf("paranoid-service frontend dev: got %s, want %s", decision.Action, ActionAuto)
	}
}

// --- Predicate Validation Tests ---

func TestPredicateValidation(t *testing.T) {
	valid := Predicate{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid predicate failed: %v", err)
	}

	// Bad field.
	bad := Predicate{Field: "unknown", Operator: OpEquals, Values: []string{"x"}}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for unknown field")
	}

	// Bad operator.
	bad = Predicate{Field: FieldEnvironment, Operator: "like", Values: []string{"x"}}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for unknown operator")
	}

	// Empty values.
	bad = Predicate{Field: FieldEnvironment, Operator: OpEquals, Values: nil}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for empty values")
	}
}

func TestRuleValidation(t *testing.T) {
	valid := Rule{
		Name:   "test",
		Action: ActionAuto,
		Predicates: []Predicate{
			{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"dev"}},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid rule failed: %v", err)
	}

	// Empty name.
	bad := Rule{Action: ActionAuto}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for empty name")
	}

	// Bad action.
	bad = Rule{Name: "test", Action: "unknown"}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for unknown action")
	}

	// Valid requirements.
	withReqs := Rule{
		Name:         "with-reqs",
		Action:       ActionAuto,
		Requirements: []GateRequirement{{Type: RequireGitHubChecks}},
	}
	if err := withReqs.Validate(); err != nil {
		t.Errorf("valid rule with requirements failed: %v", err)
	}

	// Bad requirement type.
	badReq := Rule{
		Name:         "bad-req",
		Action:       ActionAuto,
		Requirements: []GateRequirement{{Type: "nonexistent"}},
	}
	if err := badReq.Validate(); err == nil {
		t.Error("expected error for unknown requirement type")
	}
}

// --- Service Tests (with MemoryStore) ---

func TestServiceCreatePolicySetFromPosture(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps, err := svc.CreatePolicySet(ctx, "proj-1", "My Policy", "", "plan-only", "user-1")
	if err != nil {
		t.Fatalf("CreatePolicySet: %v", err)
	}
	if ps.Posture != "plan-only" {
		t.Errorf("posture = %s, want plan-only", ps.Posture)
	}
	if len(ps.Rules) == 0 {
		t.Error("expected rules from posture")
	}
	if ps.CredentialScopes.DeployCreds != CredNone {
		t.Errorf("plan-only deploy creds = %s, want none", ps.CredentialScopes.DeployCreds)
	}
}

func TestServiceCreatePolicySetUnknownPosture(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	_, err := svc.CreatePolicySet(ctx, "proj-1", "My Policy", "", "nonexistent", "user-1")
	if err == nil {
		t.Error("expected error for unknown posture")
	}
}

func TestServiceActivatePolicySet(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps, _ := svc.CreatePolicySet(ctx, "proj-1", "Policy A", "", "sandbox", "user-1")
	if err := svc.ActivatePolicySet(ctx, ps.ID, "user-1"); err != nil {
		t.Fatalf("ActivatePolicySet: %v", err)
	}

	active, err := svc.GetActivePolicySet(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetActivePolicySet: %v", err)
	}
	if active == nil || active.ID != ps.ID {
		t.Error("expected activated policy set")
	}
}

func TestServiceActivatePolicySet_DeactivatesPrevious(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps1, _ := svc.CreatePolicySet(ctx, "proj-1", "A", "", "sandbox", "user-1")
	_ = svc.ActivatePolicySet(ctx, ps1.ID, "user-1")

	ps2, _ := svc.CreatePolicySet(ctx, "proj-1", "B", "", "prod-gate", "user-1")
	_ = svc.ActivatePolicySet(ctx, ps2.ID, "user-1")

	active, _ := svc.GetActivePolicySet(ctx, "proj-1")
	if active == nil || active.ID != ps2.ID {
		t.Error("expected ps2 to be active")
	}

	ps1Again, _ := svc.GetPolicySet(ctx, ps1.ID)
	if ps1Again.IsActive {
		t.Error("expected ps1 to be deactivated")
	}
}

func TestServiceApplyPosture(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps, err := svc.ApplyPosture(ctx, "proj-1", "prod-gate", "user-1")
	if err != nil {
		t.Fatalf("ApplyPosture: %v", err)
	}
	if ps.Posture != "prod-gate" {
		t.Errorf("posture = %s, want prod-gate", ps.Posture)
	}
	if !ps.IsActive {
		// Check via store.
		active, _ := svc.GetActivePolicySet(ctx, "proj-1")
		if active == nil || active.ID != ps.ID {
			t.Error("expected posture to be activated")
		}
	}
}

func TestServiceEvaluateTransition_NoPolicy(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	decision, err := svc.EvaluateTransition(ctx, "proj-1", TransitionContext{})
	if err != nil {
		t.Fatalf("EvaluateTransition: %v", err)
	}
	if decision.Action != ActionApprove {
		t.Errorf("got %s, want %s (conservative default)", decision.Action, ActionApprove)
	}
}

func TestServiceEvaluateTransition_WithPolicy(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	_, _ = svc.ApplyPosture(ctx, "proj-1", "sandbox", "user-1")

	decision, err := svc.EvaluateTransition(ctx, "proj-1", TransitionContext{Transition: "close"})
	if err != nil {
		t.Fatalf("EvaluateTransition: %v", err)
	}
	if decision.Action != ActionApprove {
		t.Errorf("got %s, want %s", decision.Action, ActionApprove)
	}
}

func TestServiceGetEffectivePolicy(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	_, _ = svc.ApplyPosture(ctx, "proj-1", "prod-gate", "user-1")

	ep, err := svc.GetEffectivePolicy(ctx, "proj-1", TicketInfo{
		ID:          "ticket-1",
		ProjectID:   "proj-1",
		State:       "executing",
		Environment: "production",
		Type:        "task",
		Priority:    1,
	})
	if err != nil {
		t.Fatalf("GetEffectivePolicy: %v", err)
	}
	if ep.TicketID != "ticket-1" {
		t.Errorf("ticket_id = %s, want ticket-1", ep.TicketID)
	}
	if ep.PostureName != "prod-gate" {
		t.Errorf("posture = %s, want prod-gate", ep.PostureName)
	}
	if len(ep.Gates) == 0 {
		t.Fatal("expected gates on remaining path")
	}
	if ep.CurrentGate == nil {
		t.Fatal("expected current gate")
	}
	// In prod-gate posture, production transitions require approval.
	if ep.CurrentGate.Action != ActionApprove {
		t.Errorf("current gate action = %s, want %s", ep.CurrentGate.Action, ActionApprove)
	}
}

func TestServiceUpdateRules(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps, _ := svc.CreatePolicySet(ctx, "proj-1", "Custom", "", "", "user-1")

	newRules := []Rule{
		{
			ID:     "custom-1",
			Name:   "auto-all",
			Action: ActionAuto,
			Enabled: true,
		},
	}
	if err := svc.UpdateRules(ctx, ps.ID, newRules, "user-1"); err != nil {
		t.Fatalf("UpdateRules: %v", err)
	}

	updated, _ := svc.GetPolicySet(ctx, ps.ID)
	if len(updated.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(updated.Rules))
	}
}

func TestServiceDeletePolicySet_ActiveFails(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	ps, _ := svc.ApplyPosture(ctx, "proj-1", "sandbox", "user-1")
	err := svc.DeletePolicySet(ctx, ps.ID, "user-1")
	if err == nil {
		t.Error("expected error deleting active policy set")
	}
}

// --- Policy Change Event Tests ---

func TestPolicyChangeEvents(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	var receivedEvents []events.Event
	bus.Subscribe(events.EventPolicyChanged, func(_ context.Context, ev events.Event) {
		receivedEvents = append(receivedEvents, ev)
	})

	// Create → emits event.
	ps, _ := svc.CreatePolicySet(ctx, "proj-1", "Test", "", "sandbox", "user-1")
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event after create, got %d", len(receivedEvents))
	}
	if receivedEvents[0].Payload["change_type"] != string(ChangeCreated) {
		t.Errorf("change_type = %s, want created", receivedEvents[0].Payload["change_type"])
	}

	// Activate → emits event.
	_ = svc.ActivatePolicySet(ctx, ps.ID, "user-1")
	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 events after activate, got %d", len(receivedEvents))
	}
	if receivedEvents[1].Payload["change_type"] != string(ChangeActivated) {
		t.Errorf("change_type = %s, want activated", receivedEvents[1].Payload["change_type"])
	}
}

// --- Preview Tests ---

type mockTicketLister struct {
	tickets []TicketInfo
}

func (m *mockTicketLister) ListRecentTickets(_ context.Context, _ string, _ int) ([]TicketInfo, error) {
	return m.tickets, nil
}

func TestServicePreviewPolicyChange(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	store := NewMemoryStore()
	svc := NewPostureService(store, bus)

	// Set up current policy (plan-only).
	_, _ = svc.ApplyPosture(ctx, "proj-1", "plan-only", "user-1")

	// Mock tickets.
	lister := &mockTicketLister{
		tickets: []TicketInfo{
			{ID: "t-1", ProjectID: "proj-1", Environment: "development", Type: "task", Priority: 1},
			{ID: "t-2", ProjectID: "proj-1", Environment: "production", Type: "task", Priority: 0},
		},
	}

	// Preview switching to prod-gate.
	candidatePosture := PostureProdGate()
	results, err := svc.PreviewPolicyChange(ctx, "proj-1", candidatePosture.Rules, lister, 30)
	if err != nil {
		t.Fatalf("PreviewPolicyChange: %v", err)
	}

	// Should have diffs (plan-only → prod-gate changes actions for many transitions).
	if len(results) == 0 {
		t.Error("expected preview results showing differences")
	}

	// Check that dev transitions get looser (plan-only → auto).
	foundLooser := false
	for _, r := range results {
		if r.TicketID == "t-1" && r.Delta == "looser" {
			foundLooser = true
			break
		}
	}
	if !foundLooser {
		t.Error("expected at least one 'looser' delta for dev ticket")
	}
}

// --- Credential Scope Tests ---

func TestCredentialScopesPerPosture(t *testing.T) {
	tests := []struct {
		posture     string
		deployCreds CredentialLevel
		codeAccess  CredentialLevel
	}{
		{"plan-only", CredNone, CredReadOnly},
		{"sandbox", CredNone, CredReadWrite},
		{"prod-gate", CredReadWrite, CredReadWrite},
		{"graduated-risk", CredReadWrite, CredReadWrite},
		{"paranoid-service", CredReadWrite, CredReadWrite},
	}

	for _, tt := range tests {
		p := GetPosture(tt.posture)
		if p == nil {
			t.Fatalf("posture %s not found", tt.posture)
		}
		if p.CredentialScopes.DeployCreds != tt.deployCreds {
			t.Errorf("%s deploy_creds = %s, want %s", tt.posture, p.CredentialScopes.DeployCreds, tt.deployCreds)
		}
		if p.CredentialScopes.CodeAccess != tt.codeAccess {
			t.Errorf("%s code_access = %s, want %s", tt.posture, p.CredentialScopes.CodeAccess, tt.codeAccess)
		}
	}
}
