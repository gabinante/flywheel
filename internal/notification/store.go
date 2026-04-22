package notification

import "context"

// Store is the data access interface for the notification layer.
type Store interface {
	// Notification CRUD.
	CreateNotification(ctx context.Context, n *Notification) error
	GetNotification(ctx context.Context, id string) (*Notification, error)
	UpdateNotificationStatus(ctx context.Context, id string, status Status, errorMsg string) error
	ListNotifications(ctx context.Context, projectID string, limit, offset int) ([]*Notification, error)

	// Digest: fetch pending digest notifications for a project.
	ListPendingDigest(ctx context.Context, projectID string) ([]*Notification, error)
	MarkDigested(ctx context.Context, ids []string) error

	// Dismissal.
	DismissNotification(ctx context.Context, id string) error

	// Preferences.
	GetPreferences(ctx context.Context, projectID string) (*Preferences, error)
	UpsertPreferences(ctx context.Context, prefs *Preferences) error

	// Dismissal rate tracking.
	IncrementSent(ctx context.Context, classifier, projectID string) error
	IncrementDismissed(ctx context.Context, classifier, projectID string) error
	GetDismissalRate(ctx context.Context, classifier, projectID string) (*DismissalRate, error)
	ListDismissalRates(ctx context.Context, projectID string) ([]*DismissalRate, error)
}
