package risk

// PolicyThreshold defines the maximum risk level allowed for automatic advancement
// at a given state transition. If the plan's classification exceeds the threshold,
// the transition is blocked and requires human approval.
type PolicyThreshold struct {
	// MaxAutoAdvance is the highest risk level that can auto-advance without human review.
	// Operations above this level require explicit human approval.
	MaxAutoAdvance RiskLevel
}

// DefaultPolicyThresholds returns conservative policy thresholds.
// Anything above "low" risk requires human approval by default.
func DefaultPolicyThresholds() *PolicyThreshold {
	return &PolicyThreshold{
		MaxAutoAdvance: RiskLow,
	}
}

// Allows returns true if the given classification is at or below the auto-advance threshold.
func (pt *PolicyThreshold) Allows(c Classification) bool {
	return c.FinalLevel <= pt.MaxAutoAdvance
}

// PolicyResult describes whether a plan can auto-advance and why.
type PolicyResult struct {
	// Allowed is true if the plan can auto-advance.
	Allowed bool `json:"allowed"`
	// Reason explains why the plan was allowed or blocked.
	Reason string `json:"reason"`
	// Classification is the full risk classification for audit.
	Classification Classification `json:"classification"`
}

// Evaluate runs the full classification pipeline and checks against the policy threshold.
// This is the main integration point for the advancement policy engine.
func Evaluate(classifier *Classifier, plan Plan, advisory *LLMAdvisory, threshold *PolicyThreshold) PolicyResult {
	if threshold == nil {
		threshold = DefaultPolicyThresholds()
	}

	classification := classifier.Classify(plan, advisory)

	if threshold.Allows(classification) {
		return PolicyResult{
			Allowed:        true,
			Reason:         "Plan risk level (" + classification.FinalLevel.String() + ") is within auto-advance threshold (" + threshold.MaxAutoAdvance.String() + ")",
			Classification: classification,
		}
	}

	return PolicyResult{
		Allowed:        false,
		Reason:         "Plan risk level (" + classification.FinalLevel.String() + ") exceeds auto-advance threshold (" + threshold.MaxAutoAdvance.String() + ") — requires human approval",
		Classification: classification,
	}
}
