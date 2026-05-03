package ticket

import (
	"context"
	"time"
)

// TicketStore is the persistence interface for tickets.
// *Store (Postgres) and embedded.TicketStore (SQLite) implement this.
type TicketStore interface {
	NextSequence(ctx context.Context, projectID string) (int64, error)
	Create(ctx context.Context, t *Ticket) error
	GetByID(ctx context.Context, id string) (*Ticket, error)
	GetByIDs(ctx context.Context, ids []string) ([]*Ticket, error)
	GetByProject(ctx context.Context, projectID string, workStreamID string, state State) ([]*Ticket, error)
	ListByState(ctx context.Context, projectID string, state State) ([]*Ticket, error)
	UpdateState(ctx context.Context, id string, version int, newState State, assignedTo string) error
	UpdateOutputs(ctx context.Context, id string, version int, outputs map[string]any) error
	UpdateContext(ctx context.Context, id string, ctxVal TicketContext) error
	UpdateDependsOn(ctx context.Context, id string, dependsOn []string) error
	UpdateWorkStreamID(ctx context.Context, id string, workStreamID string) error
	UpdateEnvironmentID(ctx context.Context, id string, environmentID string) error
	UpdateTargetRepo(ctx context.Context, id string, targetRepo string) error
	UpdateWorkflowPhase(ctx context.Context, id string, workflowPhase string) error
	UpdateWorkflowPhaseStatus(ctx context.Context, id string, status string) error
	GetWorkflowPhase(ctx context.Context, id string) (string, error)
	ListByWorkflowPhaseStatus(ctx context.Context, projectID, status string) ([]*Ticket, error)
	CASWorkflowPhaseStatus(ctx context.Context, id, expected, desired string) (bool, error)
	UpdateWorkflow(ctx context.Context, id string, workflowID, workflowPhase string) error
	PatchInputs(ctx context.Context, id string, patch map[string]any) error
	UpdateTitleAndObjective(ctx context.Context, id string, title string, obj Objective) error
	CountByCreatedBy(ctx context.Context, createdBy string) (int, error)
	CountByCreatedByPerDay(ctx context.Context, createdBy string, days int) ([]int, error)
	GetTicketIDByCreateIdempotency(ctx context.Context, projectID, idempotencyKey string) (string, error)
	SetCreateIdempotency(ctx context.Context, projectID, idempotencyKey, ticketID string) error
	// ListStaleTickets returns tickets in any of the given states whose updated_at
	// is older than the staleness threshold. Used by the DB staleness sweep (Layer 3
	// recovery) to find zombie tickets across all projects.
	ListStaleTickets(ctx context.Context, states []State, threshold time.Duration) ([]*Ticket, error)
	// PatchOutputs merges the given keys into existing outputs without overwriting
	// unrelated keys. Used by the dispatcher to persist merge metadata.
	PatchOutputs(ctx context.Context, id string, patch map[string]any) error
}
