package ticket

import "time"

// State is the ticket lifecycle state (spec v0.2).
type State string

// Canonical lifecycle states (7 states).
const (
	StateDraft              State = "draft"
	StatePlanning           State = "planning"
	StateAwaitingInput      State = "awaiting_input"
	StateExecuting          State = "executing"
	StateAwaitingValidation State = "awaiting_validation"
	StateValidated          State = "validated"
	StateClosed             State = "closed"
)

// AllStates returns all 7 canonical states.
func AllStates() []State {
	return []State{
		StateDraft,
		StatePlanning,
		StateAwaitingInput,
		StateExecuting,
		StateAwaitingValidation,
		StateValidated,
		StateClosed,
	}
}

// IsValidState checks whether a state is one of the 7 canonical states.
func IsValidState(s State) bool {
	for _, valid := range AllStates() {
		if s == valid {
			return true
		}
	}
	return false
}

// Environment represents the deployment environment scope for state transitions (spec 4.1).
// This is the environment ID (references environments table). The full compound tuple
// (infrastructure, data_tenancy, integration_mode) lives in the environment model.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// EnvironmentQualifiedState returns a state string qualified by environment slug.
// Example: "executing-dev", "validated-staging".
func EnvironmentQualifiedState(state State, envSlug string) string {
	if envSlug == "" {
		return string(state)
	}
	return string(state) + "-" + envSlug
}

// ParseQualifiedState extracts state and environment slug from a qualified state string.
// Returns the state and env slug. If not qualified, envSlug is empty.
func ParseQualifiedState(qualified string) (State, string) {
	// Try to match against known states from longest to shortest.
	for _, s := range AllStates() {
		prefix := string(s) + "-"
		if len(qualified) > len(prefix) && qualified[:len(prefix)] == prefix {
			return s, qualified[len(prefix):]
		}
	}
	return State(qualified), ""
}

// TicketType is the kind of work.
type TicketType string

const (
	TypeTask   TicketType = "task"
	TypeBug    TicketType = "bug"
	TypeSpike  TicketType = "spike"
	TypeReview TicketType = "review"
)

// Priority 0 = P0 (highest), 3 = P3 (lowest).
type Priority int

const (
	P0 Priority = 0
	P1 Priority = 1
	P2 Priority = 2
	P3 Priority = 3
)

// Objective describes what the ticket is for.
type Objective struct {
	Description     string   `json:"description"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
	AcceptanceTest  string   `json:"acceptance_test,omitempty"`
}

// AcceptanceTest is the command or check (stored in Objective for v1).
// Kept as type alias for plan compatibility.
type AcceptanceTest = string

// TicketContext holds relevant files, constraints, prior attempts, human answers.
type TicketContext struct {
	RelevantFiles []string        `json:"relevant_files,omitempty"`
	Constraints   []string        `json:"constraints,omitempty"`
	PriorAttempts []AttemptSummary `json:"prior_attempts,omitempty"`
	HumanAnswers  []string        `json:"human_answers,omitempty"` // from resolved escalations
}

// AttemptSummary is a short summary of a prior execution attempt (for context injection).
type AttemptSummary struct {
	AgentID   string `json:"agent_id"`
	Outcome   string `json:"outcome"` // e.g. "rejected", "failed"
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Lease represents an agent's claim on a ticket (TTL-based).
type Lease struct {
	TicketID  string
	AgentID   string
	Token     string
	ExpiresAt time.Time
	Renewable bool
}

// Ticket is the core entity.
type Ticket struct {
	ID            string         `json:"id"`
	ProjectID     string         `json:"project_id"`
	Title         string         `json:"title"`
	Type          TicketType     `json:"type"`
	Priority      Priority       `json:"priority"`
	State         State          `json:"state"`
	Environment   Environment    `json:"environment,omitempty"`    // legacy simple environment scope
	EnvironmentID string         `json:"environment_id,omitempty"` // compound environment ID (spec 4.1)
	Version       int            `json:"version"`
	Objective     Objective      `json:"objective"`
	Context       TicketContext  `json:"ticket_context"`
	Inputs        map[string]any `json:"inputs"`
	Outputs       map[string]any `json:"outputs"`
	DependsOn     []string       `json:"depends_on"`
	WorkStreamID  string         `json:"work_stream_id,omitempty"`
	TargetRepo    string         `json:"target_repo,omitempty"`    // repo alias from project_repositories; empty = primary repo
	AssignedTo    string         `json:"assigned_to,omitempty"`
	WorkflowID             string         `json:"workflow_id,omitempty"`              // active workflow definition; empty = legacy state machine
	WorkflowVersion        int            `json:"workflow_version,omitempty"`          // pinned definition version; 0 = use latest
	WorkflowPhase          string         `json:"workflow_phase,omitempty"`            // current phase ID within the workflow
	WorkflowPhaseStatus    string         `json:"workflow_phase_status,omitempty"`     // "", "ready", "running", "blocked"
	WorkflowPhaseEnteredAt *time.Time     `json:"workflow_phase_entered_at,omitempty"` // when the current phase was entered
	CreatedBy              string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
