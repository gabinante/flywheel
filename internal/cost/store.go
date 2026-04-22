package cost

import "context"

// Store is the persistence interface for cost data.
// Implementations may use Postgres, Redis, or in-memory storage.
type Store interface {
	// LLM call records
	RecordCall(ctx context.Context, record *LLMCallRecord) error
	GetCallsByTicket(ctx context.Context, ticketID string) ([]*LLMCallRecord, error)
	GetCallsByProject(ctx context.Context, projectID string, month string) ([]*LLMCallRecord, error)

	// Budget management
	SetBudget(ctx context.Context, budget *Budget) error
	GetBudget(ctx context.Context, projectID, ticketID, month string) (*Budget, error)
	GetBudgetsByProject(ctx context.Context, projectID string) ([]*Budget, error)

	// Aggregation queries
	SumByProject(ctx context.Context, projectID string, month string) (Unit, error)
	SumByTicket(ctx context.Context, ticketID string) (Unit, error)
	SumByProjectGrouped(ctx context.Context, projectID string, month string) (*CostSummary, error)

	// Budget alerts
	CreateAlert(ctx context.Context, alert *BudgetAlert) error
	GetActiveAlerts(ctx context.Context, projectID string) ([]*BudgetAlert, error)
	AcknowledgeAlert(ctx context.Context, alertID string) error

	// Rate limit events
	RecordRateLimitEvent(ctx context.Context, event *RateLimitEvent) error
	MarkRateLimitResumed(ctx context.Context, eventID string) error
	GetActiveRateLimits(ctx context.Context, projectID string) ([]*RateLimitEvent, error)
}
