package policy

import (
	"path/filepath"
	"strconv"
)

// Engine evaluates policy rules against transition contexts.
// It implements the "most restrictive applicable rule wins" composition model.
type Engine struct{}

// NewEngine returns a new policy evaluation engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Evaluate takes a set of rules and a transition context, returning the policy decision.
// If no rules match, the conservative default is ActionApprove (require human approval).
func (e *Engine) Evaluate(rules []Rule, tctx TransitionContext) PolicyDecision {
	var matched []MatchedRule

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if e.ruleMatches(rule, tctx) {
			matched = append(matched, MatchedRule{
				RuleID:   rule.ID,
				RuleName: rule.Name,
				Action:   rule.Action,
				Reason:   e.matchReason(rule, tctx),
			})
		}
	}

	if len(matched) == 0 {
		// Conservative default: require approval when no rules match.
		return PolicyDecision{
			Action:       ActionApprove,
			MatchedRules: nil,
			EffectiveRule: &MatchedRule{
				RuleID:   "_default",
				RuleName: "conservative-default",
				Action:   ActionApprove,
				Reason:   "no matching policy rules; conservative default requires approval",
			},
		}
	}

	// Find the most restrictive action among all matched rules.
	mostRestrictive := matched[0]
	for _, m := range matched[1:] {
		if m.Action.Restrictiveness() > mostRestrictive.Action.Restrictiveness() {
			mostRestrictive = m
		}
	}

	return PolicyDecision{
		Action:        mostRestrictive.Action,
		MatchedRules:  matched,
		EffectiveRule: &mostRestrictive,
	}
}

// EvaluateGates computes all policy gates a ticket will encounter on its remaining path.
// happyPath is a list of (trigger, fromState, toState) tuples for the remaining transitions.
func (e *Engine) EvaluateGates(rules []Rule, baseCtx TransitionContext, happyPath []PathStep) []PolicyGate {
	var gates []PolicyGate

	for _, step := range happyPath {
		stepCtx := baseCtx
		stepCtx.Transition = step.Trigger

		decision := e.Evaluate(rules, stepCtx)

		gates = append(gates, PolicyGate{
			Transition: step.Trigger,
			FromState:  step.FromState,
			ToState:    step.ToState,
			Action:     decision.Action,
			Rules:      decision.MatchedRules,
		})
	}

	return gates
}

// PathStep describes a single step in a ticket's remaining happy path.
type PathStep struct {
	Trigger   string
	FromState string
	ToState   string
}

// ruleMatches returns true if all predicates on the rule match the transition context.
func (e *Engine) ruleMatches(rule Rule, tctx TransitionContext) bool {
	if len(rule.Predicates) == 0 {
		// A rule with no predicates matches everything (catch-all).
		return true
	}

	for _, pred := range rule.Predicates {
		if !e.predicateMatches(pred, tctx) {
			return false
		}
	}
	return true
}

// predicateMatches evaluates a single predicate against the transition context.
func (e *Engine) predicateMatches(pred Predicate, tctx TransitionContext) bool {
	actual := e.resolveField(pred.Field, tctx)

	switch pred.Operator {
	case OpEquals:
		return len(pred.Values) > 0 && containsAny(actual, pred.Values)
	case OpNotEquals:
		return len(pred.Values) > 0 && !containsAny(actual, pred.Values)
	case OpIn:
		return containsAny(actual, pred.Values)
	case OpNotIn:
		return !containsAny(actual, pred.Values)
	case OpMatches:
		return matchesGlob(actual, pred.Values)
	default:
		return false
	}
}

// resolveField extracts the field value(s) from the transition context.
func (e *Engine) resolveField(field PredicateField, tctx TransitionContext) []string {
	switch field {
	case FieldEnvironment:
		if tctx.Environment != "" {
			return []string{tctx.Environment}
		}
		return nil
	case FieldTicketType:
		if tctx.TicketType != "" {
			return []string{tctx.TicketType}
		}
		return nil
	case FieldTicketPriority:
		return []string{strconv.Itoa(tctx.TicketPriority)}
	case FieldService:
		return tctx.Services
	case FieldTransition:
		if tctx.Transition != "" {
			return []string{tctx.Transition}
		}
		return nil
	case FieldRiskLevel:
		if tctx.RiskLevel != "" {
			return []string{tctx.RiskLevel}
		}
		return nil
	case FieldRiskReversible:
		if tctx.RiskReversible != "" {
			return []string{tctx.RiskReversible}
		}
		return nil
	default:
		return nil
	}
}

// matchReason builds a human-readable explanation of why the rule matched.
func (e *Engine) matchReason(rule Rule, tctx TransitionContext) string {
	if len(rule.Predicates) == 0 {
		return "catch-all rule (no predicates)"
	}
	reason := "matched: "
	for i, p := range rule.Predicates {
		if i > 0 {
			reason += " AND "
		}
		actual := e.resolveField(p.Field, tctx)
		reason += string(p.Field) + " " + string(p.Operator) + " " + joinValues(p.Values) + " (actual: " + joinValues(actual) + ")"
	}
	return reason
}

// containsAny returns true if any element in actual is found in values.
func containsAny(actual, values []string) bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	for _, a := range actual {
		if set[a] {
			return true
		}
	}
	return false
}

// matchesGlob returns true if any element in actual matches any of the glob patterns.
func matchesGlob(actual, patterns []string) bool {
	for _, a := range actual {
		for _, p := range patterns {
			if matched, _ := filepath.Match(p, a); matched {
				return true
			}
		}
	}
	return false
}

// joinValues returns a comma-separated string of values for display.
func joinValues(vals []string) string {
	if len(vals) == 0 {
		return "<empty>"
	}
	result := ""
	for i, v := range vals {
		if i > 0 {
			result += ", "
		}
		result += v
	}
	return result
}
