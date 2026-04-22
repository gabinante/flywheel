// Package policy implements the policy calibration feedback loop.
// Every policy tracks a rolling record of gated decisions with outcomes.
// Metrics (auto-approval rate, rollback rate, incident attribution rate) drive
// proposals for broadening or review flagging.
package policy

import "time"

// Decision represents what action a policy took on a ticket.
type Decision string

const (
	DecisionAutoApproved   Decision = "auto_approved"
	DecisionRequiredReview Decision = "required_review"
	DecisionBlocked        Decision = "blocked"
)

// Outcome records what happened after the policy decision.
type Outcome string

const (
	OutcomeSuccess  Outcome = "success"
	OutcomeRollback Outcome = "rollback"
	OutcomeIncident Outcome = "incident"
)

// ChangeType is the kind of policy edit.
type ChangeType string

const (
	ChangeCreated   ChangeType = "created"
	ChangeUpdated   ChangeType = "updated"
	ChangeEnabled   ChangeType = "enabled"
	ChangeDisabled  ChangeType = "disabled"
	ChangeBroadened ChangeType = "broadened"
	ChangeNarrowed  ChangeType = "narrowed"
)

// ProposalType is whether the system suggests broadening or flags for review.
type ProposalType string

const (
	ProposalBroaden ProposalType = "broaden"
	ProposalReview  ProposalType = "review"
)

// ProposalStatus tracks the lifecycle of a proposal.
type ProposalStatus string

const (
	ProposalPending   ProposalStatus = "pending"
	ProposalAccepted  ProposalStatus = "accepted"
	ProposalRejected  ProposalStatus = "rejected"
	ProposalDismissed ProposalStatus = "dismissed"
)

// Rules defines the conditions under which a policy auto-approves, requires review, or blocks.
type Rules struct {
	// AutoApproveConditions: conditions that allow auto-approval
	AutoApproveConditions []Condition `json:"auto_approve_conditions,omitempty"`
	// ReviewConditions: conditions that require human review
	ReviewConditions []Condition `json:"review_conditions,omitempty"`
	// BlockConditions: conditions that block advancement entirely
	BlockConditions []Condition `json:"block_conditions,omitempty"`
}

// Condition is a single rule predicate.
type Condition struct {
	Field    string `json:"field"`           // e.g. "ticket.type", "ticket.priority", "change.files_changed"
	Operator string `json:"operator"`        // e.g. "eq", "neq", "lt", "gt", "in", "contains"
	Value    any    `json:"value"`           // comparison value
	Label    string `json:"label,omitempty"` // human-readable description
}

// Policy is a configurable advancement policy for a project.
type Policy struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Rules       Rules     `json:"rules"`
	Enabled     bool      `json:"enabled"`
	MinSample   int       `json:"min_sample"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PolicyDecision records a single gated decision with its eventual outcome.
type PolicyDecision struct {
	ID        string     `json:"id"`
	PolicyID  string     `json:"policy_id"`
	TicketID  string     `json:"ticket_id"`
	Decision  Decision   `json:"decision"`
	Outcome   *Outcome   `json:"outcome,omitempty"`
	Reason    string     `json:"reason"`
	DecidedAt time.Time  `json:"decided_at"`
	OutcomeAt *time.Time `json:"outcome_at,omitempty"`
}

// PolicyChangeEvent is an auditable record of a policy edit.
type PolicyChangeEvent struct {
	ID         string     `json:"id"`
	PolicyID   string     `json:"policy_id"`
	ActorID    string     `json:"actor_id"`
	ChangeType ChangeType `json:"change_type"`
	PrevRules  *Rules     `json:"prev_rules,omitempty"`
	NewRules   *Rules     `json:"new_rules,omitempty"`
	Notes      string     `json:"notes"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PolicyProposal is a system-generated suggestion.
type PolicyProposal struct {
	ID           string         `json:"id"`
	PolicyID     string         `json:"policy_id"`
	ProposalType ProposalType   `json:"proposal_type"`
	Suggestion   map[string]any `json:"suggestion"`
	Statistics   map[string]any `json:"statistics"`
	Status       ProposalStatus `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy   string         `json:"resolved_by,omitempty"`
}

// Metrics holds computed per-policy calibration metrics.
type Metrics struct {
	PolicyID             string  `json:"policy_id"`
	TotalDecisions       int     `json:"total_decisions"`
	AutoApprovalRate     float64 `json:"auto_approval_rate"`
	RollbackRate         float64 `json:"rollback_rate"`
	IncidentRate         float64 `json:"incident_rate"`
	SuccessRate          float64 `json:"success_rate"`
	PendingOutcomes      int     `json:"pending_outcomes"`
	SampleSizeSufficient bool    `json:"sample_size_sufficient"`
}

// PolicyHealth is the aggregate view for the UI.
type PolicyHealth struct {
	Policy    Policy              `json:"policy"`
	Metrics   Metrics             `json:"metrics"`
	Proposals []PolicyProposal    `json:"proposals"`
	History   []PolicyChangeEvent `json:"history"`
}

// SimulationResult shows what a candidate policy would have decided on historical tickets.
type SimulationResult struct {
	PolicyID         string               `json:"policy_id"`
	CandidateRules   Rules                `json:"candidate_rules"`
	TicketsSimulated int                  `json:"tickets_simulated"`
	Results          []SimulationDecision `json:"results"`
	Summary          SimulationSummary    `json:"summary"`
}

// SimulationDecision shows what the candidate rules would decide for one ticket.
type SimulationDecision struct {
	TicketID        string   `json:"ticket_id"`
	CurrentDecision Decision `json:"current_decision"`
	NewDecision     Decision `json:"new_decision"`
	Changed         bool     `json:"changed"`
}

// SimulationSummary aggregates simulation results.
type SimulationSummary struct {
	TotalTickets       int     `json:"total_tickets"`
	WouldAutoApprove   int     `json:"would_auto_approve"`
	WouldRequireReview int     `json:"would_require_review"`
	WouldBlock         int     `json:"would_block"`
	ChangedDecisions   int     `json:"changed_decisions"`
	ChangeRate         float64 `json:"change_rate"`
}
