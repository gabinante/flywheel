package webhook

import "context"

// Store is the data access interface for the webhook delivery system.
type Store interface {
	// CreateEvent persists a new webhook event.
	CreateEvent(ctx context.Context, event *Event) error

	// GetEvent returns a webhook event by ID, including the project's webhook config.
	GetEvent(ctx context.Context, id string) (*Event, error)

	// GetWebhookConfig returns the webhook configuration for a project.
	GetWebhookConfig(ctx context.Context, projectID string) (*WebhookConfig, error)

	// UpdateEventStatus updates the event status and attempt count.
	UpdateEventStatus(ctx context.Context, id string, status EventStatus, attemptCount int) error

	// MarkEventDelivered marks the event as successfully delivered.
	MarkEventDelivered(ctx context.Context, id string) error

	// MarkEventFailed marks the event as permanently failed.
	MarkEventFailed(ctx context.Context, id string) error

	// RecordDelivery records a single delivery attempt.
	RecordDelivery(ctx context.Context, delivery *Delivery) error

	// ListDeliveries returns all delivery attempts for a webhook event.
	ListDeliveries(ctx context.Context, webhookEventID string) ([]*Delivery, error)

	// ListEvents returns webhook events for a project, ordered by creation time descending.
	ListEvents(ctx context.Context, projectID string, limit, offset int) ([]*Event, error)

	// ListPendingEvents returns webhook events that need delivery (status = pending).
	ListPendingEvents(ctx context.Context, limit int) ([]*Event, error)
}
