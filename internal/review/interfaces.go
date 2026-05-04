package review

import "context"

// ReviewStore is the persistence interface for reviews and escalations.
// *Store (Postgres) and embedded.ReviewStore (SQLite) implement this.
type ReviewStore interface {
	CreateReview(ctx context.Context, r *Review) error
	CreateEscalation(ctx context.Context, e *Escalation) error
	UpdateEscalationResolved(ctx context.Context, id, answer, resolvedBy string) error
	ListReviewsByTicket(ctx context.Context, ticketID string) ([]Review, error)
	CountByReviewer(ctx context.Context, reviewerID string) (approved, rejected int, err error)
	CountByReviewerPerDay(ctx context.Context, reviewerID string, days int) (approved, rejected []int, err error)
	ListPendingReviewTicketIDs(ctx context.Context, projectID string) ([]string, error)
	ListEscalationsByProject(ctx context.Context, projectID string) ([]Escalation, error)
	GetEscalationByID(ctx context.Context, id string) (*Escalation, error)
	GetLatestUnresolvedEscalation(ctx context.Context, ticketID string) (*Escalation, error)
}
