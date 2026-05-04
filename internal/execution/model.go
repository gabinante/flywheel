package execution

import "time"

// StepType is the kind of execution step.
type StepType string

const (
	StepTypeToolCall    StepType = "tool_call"
	StepTypeObservation StepType = "observation"
	StepTypeThought     StepType = "thought"
	StepTypeError       StepType = "error"
)

// Step is one entry in an execution trace.
type Step struct {
	ID         string         `json:"id"`
	Type       StepType       `json:"type"`
	Payload    map[string]any `json:"payload"`
	WorkerType string         `json:"worker_type,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// ExecutionTrace is the full history of steps for a ticket.
type ExecutionTrace struct {
	TicketID   string `json:"ticket_id"`
	AgentID    string `json:"agent_id"`
	WorkerType string `json:"worker_type,omitempty"`
	Steps      []Step `json:"steps"`
	TotalCount int    `json:"total_count,omitempty"`
}

// AttemptSummary is a short summary of an attempt (for prior_attempts context).
type AttemptSummary struct {
	AgentID   string `json:"agent_id"`
	Outcome   string `json:"outcome"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}
