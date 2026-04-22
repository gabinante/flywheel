package notification

import "context"

// ChannelAdapter is the pluggable interface for notification delivery channels.
// Implementations handle the actual delivery of notifications via their respective
// transport (Slack webhooks, email SMTP, SMS API, etc.).
//
// Channel adapters must be safe for concurrent use.
// Operators can swap implementations by registering alternative adapters,
// including external MCP server-backed adapters.
type ChannelAdapter interface {
	// Name returns the adapter identifier (e.g. "slack", "email", "sms").
	Name() string

	// Send delivers a single notification via the channel.
	// The adapter should format the notification appropriately for its channel.
	// Returns an error if delivery fails (the service will mark it as failed).
	Send(ctx context.Context, n *Notification, prefs *Preferences) error

	// SendDigest delivers a batch of notifications as a single digest message.
	// Adapters should format this as a summary rather than individual messages.
	SendDigest(ctx context.Context, notifications []*Notification, prefs *Preferences) error
}
