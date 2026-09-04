package gate

import "time"

// GateRequirementType identifies an automated condition that must be satisfied.
type GateRequirementType string

const (
	// RequireGitHubChecks requires all GitHub CI checks on the PR to pass.
	RequireGitHubChecks GateRequirementType = "github_checks"
	// RequireHumanApproval requires an approved review on the ticket.
	RequireHumanApproval GateRequirementType = "human_approval"
	// RequireWebhook requires an external system to POST to a callback URL.
	RequireWebhook GateRequirementType = "webhook"
	// RequireHTTPCheck requires an HTTP endpoint to return the expected status.
	RequireHTTPCheck GateRequirementType = "http_check"
)

// validGateRequirementTypes is the set of recognized requirement types.
var validGateRequirementTypes = map[GateRequirementType]bool{
	RequireGitHubChecks:  true,
	RequireHumanApproval: true,
	RequireWebhook:       true,
	RequireHTTPCheck:     true,
}

// GateRequirement is an automated condition that must be satisfied for a gate to clear.
// Requirements are orthogonal to actions — a rule can require both CI passing AND human approval.
type GateRequirement struct {
	Type   GateRequirementType `json:"type"`
	Config map[string]any      `json:"config,omitempty"` // type-specific (future use)
}

// GateRequirementStatus reports whether a single requirement is currently satisfied.
type GateRequirementStatus struct {
	Requirement GateRequirement `json:"requirement"`
	Satisfied   bool            `json:"satisfied"`
	Reason      string          `json:"reason,omitempty"`
	CheckedAt   time.Time       `json:"checked_at,omitempty"`
}
