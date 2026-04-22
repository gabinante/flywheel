package policy

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// Posture & Rule Engine types (composable rules with most-restrictive-wins)
// =============================================================================

// Action defines what happens when a policy rule matches a transition.
// Ordered from least to most restrictive.
type Action string

const (
	// ActionAuto proceeds automatically with no human involvement.
	ActionAuto Action = "auto"
	// ActionNotify proceeds automatically but notifies the operator.
	ActionNotify Action = "notify"
	// ActionPlanOnly allows agents to author/plan but the operator applies manually.
	ActionPlanOnly Action = "plan-only"
	// ActionOpenPRStop opens a PR and stops until the operator merges.
	ActionOpenPRStop Action = "open-pr-stop"
	// ActionApprove requires explicit human approval before proceeding.
	ActionApprove Action = "approve"
	// ActionTypedConfirm requires a typed confirmation string (e.g., "deploy prod").
	ActionTypedConfirm Action = "typed-confirm"
	// ActionHumanRequired requires a human to perform the action entirely.
	ActionHumanRequired Action = "human-required"
)

// actionRestrictiveness maps actions to a numeric level. Higher = more restrictive.
// Most-restrictive-applicable-rule wins.
var actionRestrictiveness = map[Action]int{
	ActionAuto:          0,
	ActionNotify:        1,
	ActionPlanOnly:      2,
	ActionOpenPRStop:    3,
	ActionApprove:       4,
	ActionTypedConfirm:  5,
	ActionHumanRequired: 6,
}

// Restrictiveness returns the numeric restrictiveness level for an action.
// Higher values are more restrictive. Unknown actions return max restrictiveness.
func (a Action) Restrictiveness() int {
	if v, ok := actionRestrictiveness[a]; ok {
		return v
	}
	return 6 // unknown = most restrictive (conservative default)
}

// IsValid returns true if the action is a recognized type.
func (a Action) IsValid() bool {
	_, ok := actionRestrictiveness[a]
	return ok
}

// MoreRestrictive returns the more restrictive of two actions.
func MoreRestrictive(a, b Action) Action {
	if a.Restrictiveness() >= b.Restrictiveness() {
		return a
	}
	return b
}

// AllActions returns all valid actions ordered from least to most restrictive.
func AllActions() []Action {
	return []Action{
		ActionAuto,
		ActionNotify,
		ActionPlanOnly,
		ActionOpenPRStop,
		ActionApprove,
		ActionTypedConfirm,
		ActionHumanRequired,
	}
}

// PredicateField identifies what aspect of the ticket/transition a predicate matches against.
type PredicateField string

const (
	FieldEnvironment    PredicateField = "environment"     // development, staging, production
	FieldTicketType     PredicateField = "ticket.type"     // task, bug, spike, review
	FieldTicketPriority PredicateField = "ticket.priority" // 0-3
	FieldService        PredicateField = "service"         // service name from ticket context
	FieldTransition     PredicateField = "transition"      // trigger name (e.g., deploy, submit)
	FieldRiskLevel      PredicateField = "risk.level"      // low, medium, high, critical (from risk classification)
	FieldRiskReversible PredicateField = "risk.reversible"  // true, false
)

// PredicateOperator defines how values are compared.
type PredicateOperator string

const (
	OpEquals    PredicateOperator = "eq"
	OpNotEquals PredicateOperator = "neq"
	OpIn        PredicateOperator = "in"
	OpNotIn     PredicateOperator = "not_in"
	OpMatches   PredicateOperator = "matches" // glob/regex for service names
)

// Predicate is a single matching condition. All predicates on a rule are ANDed.
type Predicate struct {
	Field    PredicateField    `json:"field"`
	Operator PredicateOperator `json:"operator"`
	Values   []string          `json:"values"`
}

// Rule is a single composable policy rule with applies-to predicates and an action.
type Rule struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Predicates  []Predicate `json:"predicates"` // ANDed: all must match
	Action      Action      `json:"action"`
	Enabled     bool        `json:"enabled"`
}

// PolicySet is a named collection of rules associated with a project.
// Only one policy set is active per project at a time.
type PolicySet struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"project_id"`
	Name             string           `json:"name"`
	Description      string           `json:"description,omitempty"`
	Posture          string           `json:"posture,omitempty"` // posture name if from a bundle, empty if custom
	Rules            []Rule           `json:"rules"`
	CredentialScopes CredentialScopes `json:"credential_scopes"`
	IsActive         bool             `json:"is_active"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// CredentialScopes defines what credentials the posture requires.
type CredentialScopes struct {
	CodeAccess    CredentialLevel `json:"code_access"`    // none, read-only, read-write
	DeployCreds   CredentialLevel `json:"deploy_creds"`   // none, read-only, read-write
	InfraCreds    CredentialLevel `json:"infra_creds"`    // none, read-only, read-write
	SecretAccess  CredentialLevel `json:"secret_access"`  // none, read-only, read-write
	SandboxAccess bool            `json:"sandbox_access"` // whether sandbox execution is permitted
}

// CredentialLevel describes the access level for a credential category.
type CredentialLevel string

const (
	CredNone      CredentialLevel = "none"
	CredReadOnly  CredentialLevel = "read-only"
	CredReadWrite CredentialLevel = "read-write"
)

// PolicyChange records a change to a policy set (for audit/streaming).
type PolicyChange struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	PolicySetID string          `json:"policy_set_id"`
	ChangeType  ChangeType      `json:"change_type"`
	ChangedBy   string          `json:"changed_by"`
	OldRules    json.RawMessage `json:"old_rules,omitempty"`
	NewRules    json.RawMessage `json:"new_rules,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ChangeType describes what kind of policy change occurred.
// Used by both policy set posture changes and calibration feedback changes.
type ChangeType string

const (
	ChangeCreated     ChangeType = "created"
	ChangeActivated   ChangeType = "activated"
	ChangeDeactivated ChangeType = "deactivated"
	ChangeUpdated     ChangeType = "updated"
	ChangeDeleted     ChangeType = "deleted"
	ChangeEnabled     ChangeType = "enabled"
	ChangeDisabled    ChangeType = "disabled"
	ChangeBroadened   ChangeType = "broadened"
	ChangeNarrowed    ChangeType = "narrowed"
)

// TransitionContext captures all the facts about a pending transition
// that policy rules evaluate against.
type TransitionContext struct {
	Environment    string // development, staging, production
	TicketType     string // task, bug, spike, review
	TicketPriority int    // 0-3
	Services       []string // service names from ticket metadata
	Transition     string // trigger name (deploy, submit, etc.)
	RiskLevel      string // low, medium, high, critical (empty if not classified)
	RiskReversible string // "true", "false", "" (empty if not classified)
}

// PolicyDecision is the result of evaluating policy rules for a transition.
type PolicyDecision struct {
	Action        Action           `json:"action"`
	MatchedRules  []MatchedRule    `json:"matched_rules"`
	EffectiveRule *MatchedRule     `json:"effective_rule,omitempty"` // the most restrictive match
}

// MatchedRule records which rule matched and what action it prescribed.
type MatchedRule struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Action   Action `json:"action"`
	Reason   string `json:"reason"` // human-readable explanation
}

// PolicyGate describes a future gate a ticket will encounter on its remaining path.
type PolicyGate struct {
	Transition string `json:"transition"` // trigger that will be gated
	FromState  string `json:"from_state"`
	ToState    string `json:"to_state"`
	Action     Action `json:"action"`      // what the policy requires
	Rules      []MatchedRule `json:"rules"` // which rules caused this gate
}

// EffectivePolicy is the complete policy view for a specific ticket,
// showing all gates it will encounter on its remaining path.
type EffectivePolicy struct {
	TicketID     string       `json:"ticket_id"`
	PolicySetID  string       `json:"policy_set_id"`
	PostureName  string       `json:"posture_name,omitempty"`
	Gates        []PolicyGate `json:"gates"`
	CurrentGate  *PolicyGate  `json:"current_gate,omitempty"` // gate for the next transition
}

// PreviewResult shows what would change if a candidate policy set replaced the active one.
type PreviewResult struct {
	TicketID     string          `json:"ticket_id"`
	Transition   string          `json:"transition"`
	OldAction    Action          `json:"old_action"`
	NewAction    Action          `json:"new_action"`
	Delta        string          `json:"delta"` // "stricter", "looser", "unchanged"
}

// Validate checks that a rule is well-formed.
func (r *Rule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if !r.Action.IsValid() {
		return fmt.Errorf("invalid action %q", r.Action)
	}
	for i, p := range r.Predicates {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("predicate[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks that a predicate is well-formed.
func (p *Predicate) Validate() error {
	switch p.Field {
	case FieldEnvironment, FieldTicketType, FieldTicketPriority,
		FieldService, FieldTransition, FieldRiskLevel, FieldRiskReversible:
		// valid
	default:
		return fmt.Errorf("unknown predicate field %q", p.Field)
	}
	switch p.Operator {
	case OpEquals, OpNotEquals, OpIn, OpNotIn, OpMatches:
		// valid
	default:
		return fmt.Errorf("unknown operator %q", p.Operator)
	}
	if len(p.Values) == 0 {
		return fmt.Errorf("predicate values must not be empty")
	}
	return nil
}

// Validate checks that a policy set is well-formed.
func (ps *PolicySet) Validate() error {
	if ps.Name == "" {
		return fmt.Errorf("policy set name is required")
	}
	if ps.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	for i, r := range ps.Rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("rule[%d] %q: %w", i, r.Name, err)
		}
	}
	return nil
}

// =============================================================================
// Calibration Feedback Loop types (decision tracking, outcome recording, metrics)
// =============================================================================

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

// CalibrationRules defines the conditions under which a policy auto-approves, requires review, or blocks.
type CalibrationRules struct {
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

// CalibrationPolicy is a configurable advancement policy for a project.
type CalibrationPolicy struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rules       CalibrationRules `json:"rules"`
	Enabled     bool             `json:"enabled"`
	MinSample   int              `json:"min_sample"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// CalibrationDecision records a single gated decision with its eventual outcome.
type CalibrationDecision struct {
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
	ID         string           `json:"id"`
	PolicyID   string           `json:"policy_id"`
	ActorID    string           `json:"actor_id"`
	ChangeType ChangeType       `json:"change_type"`
	PrevRules  *CalibrationRules `json:"prev_rules,omitempty"`
	NewRules   *CalibrationRules `json:"new_rules,omitempty"`
	Notes      string           `json:"notes"`
	CreatedAt  time.Time        `json:"created_at"`
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
	Policy    CalibrationPolicy   `json:"policy"`
	Metrics   Metrics             `json:"metrics"`
	Proposals []PolicyProposal    `json:"proposals"`
	History   []PolicyChangeEvent `json:"history"`
}

// SimulationResult shows what a candidate policy would have decided on historical tickets.
type SimulationResult struct {
	PolicyID         string               `json:"policy_id"`
	CandidateRules   CalibrationRules     `json:"candidate_rules"`
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
