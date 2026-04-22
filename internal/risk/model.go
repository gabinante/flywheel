package risk

import "fmt"

// RiskLevel represents the severity of a plan's risk.
// Ordered from lowest to highest; comparisons use integer values.
type RiskLevel int

const (
	// RiskSafe indicates no risk — read-only operations, comments, tests, docs.
	RiskSafe RiskLevel = 0
	// RiskLow indicates low risk — additive, non-breaking changes.
	RiskLow RiskLevel = 1
	// RiskReversible indicates reversible changes that can be rolled back.
	RiskReversible RiskLevel = 2
	// RiskHigh indicates high risk — potentially destructive but recoverable.
	RiskHigh RiskLevel = 3
	// RiskDestructive indicates destructive operations — data loss, breaking changes.
	RiskDestructive RiskLevel = 4
)

// String returns the human-readable name of the risk level.
func (r RiskLevel) String() string {
	switch r {
	case RiskSafe:
		return "safe"
	case RiskLow:
		return "low"
	case RiskReversible:
		return "reversible"
	case RiskHigh:
		return "high"
	case RiskDestructive:
		return "destructive"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// ParseRiskLevel converts a string to a RiskLevel. Returns RiskDestructive for unknown inputs.
func ParseRiskLevel(s string) RiskLevel {
	switch s {
	case "safe":
		return RiskSafe
	case "low":
		return RiskLow
	case "reversible":
		return RiskReversible
	case "high":
		return RiskHigh
	case "destructive":
		return RiskDestructive
	default:
		return RiskDestructive // unknown defaults to maximum
	}
}

// Backend identifies the type of system a plan operation targets.
type Backend string

const (
	BackendDatabase    Backend = "database"
	BackendTerraform   Backend = "terraform"
	BackendCode        Backend = "code"
	BackendShell       Backend = "shell"
	BackendExternalAPI Backend = "external_api"
)

// Operation is a single unit of work within a plan. Backend-specific details
// are carried in the Details map — each per-backend classifier knows which
// keys to inspect.
type Operation struct {
	// Backend identifies which system this operation targets.
	Backend Backend `json:"backend"`
	// Action is the backend-specific action (e.g. "CREATE TABLE", "destroy", "edit", "curl", "POST /api/v1/deploy").
	Action string `json:"action"`
	// Target is the resource being acted upon (e.g. table name, resource address, file path).
	Target string `json:"target"`
	// Details carries backend-specific metadata the classifier inspects.
	Details map[string]any `json:"details,omitempty"`
}

// Plan is a typed, structured representation of what an agent intends to do.
// Classification is a deterministic function of Plan contents — same Plan always
// produces the same Classification.
type Plan struct {
	// Operations is the ordered list of operations the plan will perform.
	Operations []Operation `json:"operations"`
	// Environment is the deployment target (development, staging, production).
	Environment string `json:"environment,omitempty"`
	// ServiceTags are sensitivity labels from the project/service map layer.
	// Tags like "pii", "financial", "critical-path" cause upward reclassification.
	ServiceTags []string `json:"service_tags,omitempty"`
}

// RuleCitation records which classification rule fired and why.
// Every classification must cite at least one rule so operators can audit.
type RuleCitation struct {
	// Rule is the machine-readable rule identifier (e.g. "db.drop_table", "tf.resource_replacement").
	Rule string `json:"rule"`
	// Backend is which backend classifier produced this citation.
	Backend Backend `json:"backend"`
	// Level is the risk level this rule assigned.
	Level RiskLevel `json:"level"`
	// Description is a human-readable explanation of why this rule fired.
	Description string `json:"description"`
}

// Classification is the output of the risk classifier. It is deterministic —
// the same Plan always produces the same Classification.
type Classification struct {
	// BaseLevel is the highest risk level from per-backend classifiers before adjustments.
	BaseLevel RiskLevel `json:"base_level"`
	// EnvironmentMultiplied is the level after applying the environment multiplier.
	EnvironmentMultiplied RiskLevel `json:"environment_multiplied"`
	// SensitivityAdjusted is the level after applying service sensitivity tags.
	SensitivityAdjusted RiskLevel `json:"sensitivity_adjusted"`
	// AdvisoryAdjusted is the final level after LLM advisory (upward-only). This is the authoritative level.
	AdvisoryAdjusted RiskLevel `json:"advisory_adjusted"`
	// FinalLevel is the authoritative risk level (alias for AdvisoryAdjusted for convenience).
	FinalLevel RiskLevel `json:"final_level"`
	// Citations lists every rule that fired, in order. At least one citation is always present.
	Citations []RuleCitation `json:"citations"`
}
