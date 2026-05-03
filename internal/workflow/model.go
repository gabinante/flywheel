package workflow

import "time"

// PhaseType determines how a workflow phase maps to internal ticket state transitions.
type PhaseType string

const (
	PhaseAgent    PhaseType = "agent"    // agentic code session with worker role
	PhaseExternal PhaseType = "external" // HTTP call (sync/async/poll)
	PhaseGate     PhaseType = "gate"     // condition checkpoint (CI checks, PR approval, human sign-off, etc.)
	PhaseAction   PhaseType = "action"   // inline Go function execution

	// Legacy types — still accepted in validation but UI only shows the three above.
	PhaseManual    PhaseType = "manual"    // legacy: human must approve → use gate
	PhaseAutomated PhaseType = "automated" // legacy: system runs action → use external
	PhaseDeploy    PhaseType = "deploy"    // legacy: triggers deployment → use external
	PhaseObserve   PhaseType = "observe"   // legacy: monitoring window → use external
)

// Phase is a single step in a workflow definition.
type Phase struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        PhaseType      `json:"type"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	OnFailure   string         `json:"on_failure,omitempty"`
	Timeout     string         `json:"timeout,omitempty"` // e.g. "30m", "2h"; parsed as time.Duration
}

// Definition is a reusable workflow template that defines the delivery pipeline.
type Definition struct {
	ID          string    `json:"id"`
	Scope       string    `json:"scope"`    // "system", "org", "project"
	ScopeID     string    `json:"scope_id"` // empty for system, org or project ID
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     int       `json:"version"`
	Phases      []Phase   `json:"phases"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PhaseCompletion records a ticket's completion of a workflow phase.
type PhaseCompletion struct {
	ID          string         `json:"id"`
	TicketID    string         `json:"ticket_id"`
	WorkflowID  string         `json:"workflow_id"`
	PhaseID     string         `json:"phase_id"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
	Outcome     string         `json:"outcome"` // "success", "failed", "skipped"
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Position describes a ticket's current location within its workflow.
type Position struct {
	WorkflowID   string            `json:"workflow_id"`
	WorkflowName string            `json:"workflow_name"`
	CurrentPhase *Phase            `json:"current_phase"`
	PhaseIndex   int               `json:"phase_index"`
	TotalPhases  int               `json:"total_phases"`
	Phases       []Phase           `json:"phases"`
	History      []PhaseCompletion `json:"history"`
}

// ValidPhaseTypes returns all recognized phase types (including legacy).
func ValidPhaseTypes() []PhaseType {
	return []PhaseType{PhaseAgent, PhaseExternal, PhaseGate, PhaseAction, PhaseManual, PhaseAutomated, PhaseDeploy, PhaseObserve}
}

// PrimaryPhaseTypes returns the three primary user-facing phase types.
func PrimaryPhaseTypes() []PhaseType {
	return []PhaseType{PhaseAgent, PhaseExternal, PhaseGate, PhaseAction}
}

// IsValidPhaseType checks if a phase type is recognized.
func IsValidPhaseType(pt PhaseType) bool {
	for _, v := range ValidPhaseTypes() {
		if pt == v {
			return true
		}
	}
	return false
}
