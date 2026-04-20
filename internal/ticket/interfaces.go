package ticket

import "context"

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
	UpdateTitleAndObjective(ctx context.Context, id string, title string, obj Objective) error
	CountByCreatedBy(ctx context.Context, createdBy string) (int, error)
	CountByCreatedByPerDay(ctx context.Context, createdBy string, days int) ([]int, error)
	GetTicketIDByCreateIdempotency(ctx context.Context, projectID, idempotencyKey string) (string, error)
	SetCreateIdempotency(ctx context.Context, projectID, idempotencyKey, ticketID string) error
}
