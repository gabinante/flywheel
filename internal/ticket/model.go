package ticket

import "time"

// State is the ticket lifecycle state (spec v0.2).
type State string

// Spec v0.2 lifecycle states (full SDLC lifecycle).
const (
	StateDraft              State = "draft"
	StateSpecced            State = "specced"
	StatePlanning           State = "planning"
	StateAwaitingInput      State = "awaiting_input"
	StateExecuting          State = "executing"
	StateAwaitingValidation State = "awaiting_validation"
	StateValidated          State = "validated"
	StateDeploying          State = "deploying"
	StateObserving          State = "observing"
	StateClosed             State = "closed"
)

// Deprecated state aliases — kept so existing Go code referencing old constant
// names continues to compile. Each maps to the v0.2 equivalent.
const (
	StatePending        = StateDraft              // pending → draft
	StateClaimed        = StatePlanning           // claimed → planning
	StateAwaitingReview = StateAwaitingValidation // awaiting_review → awaiting_validation
	StateDone           = StateClosed             // done → closed
	StateNeedsHuman     = StateAwaitingInput      // needs_human → awaiting_input
	StateBlocked        = StateAwaitingInput      // blocked → awaiting_input
	StateFailed         = StateDraft              // failed → draft (retry semantics)
)

// AllStates returns all 10 valid v0.2 states.
func AllStates() []State {
	return []State{
		StateDraft,
		StateSpecced,
		StatePlanning,
		StateAwaitingInput,
		StateExecuting,
		StateAwaitingValidation,
		StateValidated,
		StateDeploying,
		StateObserving,
		StateClosed,
	}
}

// IsValidState checks whether a state is one of the 10 valid v0.2 states.
func IsValidState(s State) bool {
	for _, valid := range AllStates() {
		if s == valid {
			return true
		}
	}
	return false
}

// MapLegacyState converts legacy (pre-v0.2) state strings stored in the database
// to their v0.2 equivalents. Returns the input unchanged if already a valid v0.2 state.
func MapLegacyState(s State) State {
	switch s {
	case "pending":
		return StateDraft
	case "claimed":
		return StatePlanning
	case "awaiting_review":
		return StateAwaitingValidation
	case "done":
		return StateClosed
	case "needs_human":
		return StateAwaitingInput
	case "blocked":
		return StateAwaitingInput
	case "failed":
		return StateDraft
	default:
		return s
	}
}

// Environment represents the deployment environment scope for state transitions (spec 4.1).
// The state machine is environment-scoped: transitions may have different policies per environment.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

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
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	Title        string         `json:"title"`
	Type         TicketType     `json:"type"`
	Priority     Priority       `json:"priority"`
	State        State          `json:"state"`
	Environment  Environment    `json:"environment,omitempty"` // environment scope per spec 4.1
	Version      int            `json:"version"`
	Objective    Objective      `json:"objective"`
	Context      TicketContext  `json:"ticket_context"`
	Inputs       map[string]any `json:"inputs"`
	Outputs      map[string]any `json:"outputs"`
	DependsOn    []string       `json:"depends_on"`
	WorkStreamID string         `json:"work_stream_id,omitempty"`
	AssignedTo   string         `json:"assigned_to,omitempty"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
