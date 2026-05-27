package orchestrator

import "time"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type Message struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type RunEventKind string

const (
	RunEventKindStatus       RunEventKind = "status"
	RunEventKindWorkerOutput RunEventKind = "worker_output"
	RunEventKindError        RunEventKind = "error"
	RunEventKindToolCall     RunEventKind = "tool_call"
	RunEventKindToolResult   RunEventKind = "tool_result"
	RunEventKindPhaseChange  RunEventKind = "phase_change"
)

type OrchestratorPhase string

const (
	PhaseQueued        OrchestratorPhase = "queued"
	PhaseConnecting    OrchestratorPhase = "connecting"
	PhaseInvestigating OrchestratorPhase = "investigating"
	PhasePlanning      OrchestratorPhase = "planning"
	PhaseAuthoring     OrchestratorPhase = "authoring"
	PhaseComposing     OrchestratorPhase = "composing"
	PhaseComplete      OrchestratorPhase = "complete"
	PhaseFailed        OrchestratorPhase = "failed"
	PhaseCancelled     OrchestratorPhase = "cancelled"
)

type RunEvent struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Kind      RunEventKind   `json:"kind"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type Run struct {
	ID                 string            `json:"id"`
	ProjectID          string            `json:"project_id"`
	UserMessageID      string            `json:"user_message_id"`
	AssistantMessageID string            `json:"assistant_message_id,omitempty"`
	Status             RunStatus         `json:"status"`
	Phase              OrchestratorPhase `json:"phase,omitempty"`
	WorkerID           string            `json:"worker_id,omitempty"`
	WorkerName         string            `json:"worker_name,omitempty"`
	Runner             string            `json:"runner,omitempty"`
	Driver             string            `json:"driver,omitempty"`
	Model              string            `json:"model,omitempty"`
	Error              string            `json:"error,omitempty"`
	StartedAt          time.Time         `json:"started_at"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
	Events             []RunEvent        `json:"events,omitempty"`
}

type Thread struct {
	ProjectID string    `json:"project_id"`
	Messages  []Message `json:"messages"`
	Runs      []Run     `json:"runs"`
	Playbook  Playbook  `json:"playbook"`
}
