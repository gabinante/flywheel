package webhook

import "time"

// EventStatus represents the lifecycle status of a webhook event.
type EventStatus string

const (
	EventStatusPending   EventStatus = "pending"
	EventStatusDelivered EventStatus = "delivered"
	EventStatusFailed    EventStatus = "failed"
)

// Event represents a webhook event to be delivered to a project's webhook URL.
// Events are created by the post-payment pipeline or other internal triggers.
type Event struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	EventType    string            `json:"event_type"`     // e.g. "payment.completed"
	Payload      map[string]any    `json:"payload"`        // event-specific data
	Status       EventStatus       `json:"status"`
	AttemptCount int               `json:"attempt_count"`
	MaxAttempts  int               `json:"max_attempts"`
	CreatedAt    time.Time         `json:"created_at"`
	DeliveredAt  *time.Time        `json:"delivered_at,omitempty"`
	FailedAt     *time.Time        `json:"failed_at,omitempty"`
}

// Delivery represents a single delivery attempt for a webhook event.
type Delivery struct {
	ID             string    `json:"id"`
	WebhookEventID string    `json:"webhook_event_id"`
	ProjectID      string    `json:"project_id"`
	AttemptNumber  int       `json:"attempt_number"`
	StatusCode     *int      `json:"status_code,omitempty"` // nil for network errors
	ResponseBody   string    `json:"response_body"`         // truncated to 1KB
	DurationMs     int       `json:"duration_ms"`
	Error          string    `json:"error,omitempty"`
	Success        bool      `json:"success"`
	CreatedAt      time.Time `json:"created_at"`
}

// StandardPayload is the standardized webhook payload format sent to merchants.
type StandardPayload struct {
	ID      string         `json:"id"`      // webhook event ID
	Type    string         `json:"type"`    // event type, e.g. "payment.completed"
	Created string         `json:"created"` // ISO 8601 timestamp
	Data    map[string]any `json:"data"`    // event-specific payload
}

// WebhookConfig holds the webhook configuration for a project.
type WebhookConfig struct {
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
}

// SupportedEventTypes lists all recognized webhook event types.
var SupportedEventTypes = []string{
	"payment.completed",
	"payment.failed",
	"payment.refunded",
	"chargeback.created",
	"chargeback.updated",
}

// MaxResponseBodySize is the maximum stored response body size (1KB).
const MaxResponseBodySize = 1024

// MaxAttempts is the default maximum number of delivery attempts.
const MaxAttempts = 4

// RetryDelays defines the backoff delays for each retry attempt.
// Attempt 1: immediate (0s), Attempt 2: 10s, Attempt 3: 60s, Attempt 4: 300s.
var RetryDelays = []time.Duration{
	0,
	10 * time.Second,
	60 * time.Second,
	300 * time.Second,
}

// TruncateResponseBody truncates the response body to MaxResponseBodySize.
func TruncateResponseBody(body string) string {
	if len(body) <= MaxResponseBodySize {
		return body
	}
	return body[:MaxResponseBodySize]
}
