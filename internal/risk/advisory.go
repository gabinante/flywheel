package risk

// LLMAdvisory represents an advisory risk adjustment from an LLM.
// The LLM role is advisory only and upward-only — it can raise severity but never lower it.
// This ensures the deterministic classifier remains the authority; the LLM can only add caution.
type LLMAdvisory struct {
	// SuggestedLevel is the risk level the LLM suggests.
	SuggestedLevel RiskLevel `json:"suggested_level"`
	// Reason is the LLM's explanation for the adjustment.
	Reason string `json:"reason"`
}

// ApplyAdvisory applies an LLM advisory to a risk level. The advisory can only
// raise the level, never lower it. If the suggested level is at or below the
// current level, the current level is returned unchanged.
//
// Returns the (potentially raised) level and a citation if the level was raised.
func ApplyAdvisory(level RiskLevel, advisory *LLMAdvisory) (RiskLevel, *RuleCitation) {
	if advisory == nil {
		return level, nil
	}

	// Upward-only: LLM can raise but never lower.
	if advisory.SuggestedLevel <= level {
		return level, nil
	}

	// Cap at maximum risk level.
	suggested := advisory.SuggestedLevel
	if suggested > RiskDestructive {
		suggested = RiskDestructive
	}

	return suggested, &RuleCitation{
		Rule:        "advisory.llm_raise",
		Backend:     "",
		Level:       suggested,
		Description: "LLM advisory raised risk from " + level.String() + " to " + suggested.String() + ": " + advisory.Reason,
	}
}
