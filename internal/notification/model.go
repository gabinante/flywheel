// Package notification implements the notification layer (spec v0.2 Layer 12).
// Policy-driven async push to operators with four categories, pluggable channels,
// push vs digest routing, and dismissal rate tracking for classifier tuning.
package notification

import "time"

// Category classifies what kind of notification this is.
type Category string

const (
	CategoryUrgentDecision    Category = "urgent_decision"
	CategoryAutonomousAction  Category = "autonomous_action"
	CategoryCalibrationReview Category = "calibration_review"
	CategoryAnomalyAlert      Category = "anomaly_alert"
)

// Urgency determines routing (push vs digest) and channel selection.
type Urgency string

const (
	UrgencyCritical Urgency = "critical"
	UrgencyHigh     Urgency = "high"
	UrgencyMedium   Urgency = "medium"
	UrgencyLow      Urgency = "low"
)

// UrgencyRank returns a numeric rank for urgency comparison (lower = more urgent).
func UrgencyRank(u Urgency) int {
	switch u {
	case UrgencyCritical:
		return 0
	case UrgencyHigh:
		return 1
	case UrgencyMedium:
		return 2
	case UrgencyLow:
		return 3
	default:
		return 4
	}
}

// Channel identifies the delivery channel.
type Channel string

const (
	ChannelSlack Channel = "slack"
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

// Routing determines whether a notification is pushed immediately or batched into a digest.
type Routing string

const (
	RoutingPush   Routing = "push"
	RoutingDigest Routing = "digest"
)

// Status tracks the delivery lifecycle of a notification.
type Status string

const (
	StatusPending  Status = "pending"
	StatusSent     Status = "sent"
	StatusFailed   Status = "failed"
	StatusDigested Status = "digested"
)

// Notification is a single notification to be delivered to an operator.
type Notification struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	TicketID     string            `json:"ticket_id,omitempty"`
	Category     Category          `json:"category"`
	Urgency      Urgency           `json:"urgency"`
	Title        string            `json:"title"`
	Body         string            `json:"body,omitempty"`
	Channel      Channel           `json:"channel"`
	Routing      Routing           `json:"routing"`
	DecisionMeta map[string]any    `json:"decision_meta,omitempty"`
	Context      map[string]any    `json:"context,omitempty"`
	Status       Status            `json:"status"`
	SentAt       *time.Time        `json:"sent_at,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	Dismissed    bool              `json:"dismissed"`
	DismissedAt  *time.Time        `json:"dismissed_at,omitempty"`
	Classifier   string            `json:"classifier,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// Preferences holds per-project notification routing configuration.
type Preferences struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	CriticalChannel  Channel   `json:"critical_channel"`
	HighChannel      Channel   `json:"high_channel"`
	MediumChannel    Channel   `json:"medium_channel"`
	LowChannel       Channel   `json:"low_channel"`
	DigestEnabled    bool      `json:"digest_enabled"`
	DigestInterval   string    `json:"digest_interval"`
	PushThreshold    Urgency   `json:"push_threshold"`
	SlackWebhookURL  string    `json:"slack_webhook_url,omitempty"`
	EmailAddress     string    `json:"email_address,omitempty"`
	SMSNumber        string    `json:"sms_number,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ChannelForUrgency returns the configured channel for a given urgency level.
func (p *Preferences) ChannelForUrgency(u Urgency) Channel {
	switch u {
	case UrgencyCritical:
		return p.CriticalChannel
	case UrgencyHigh:
		return p.HighChannel
	case UrgencyMedium:
		return p.MediumChannel
	case UrgencyLow:
		return p.LowChannel
	default:
		return ChannelSlack
	}
}

// ShouldPush returns true if the given urgency should be pushed immediately
// (at or above the push threshold).
func (p *Preferences) ShouldPush(u Urgency) bool {
	return UrgencyRank(u) <= UrgencyRank(p.PushThreshold)
}

// DismissalRate tracks notification dismissal rates per classifier for tuning.
type DismissalRate struct {
	Classifier       string    `json:"classifier"`
	ProjectID        string    `json:"project_id"`
	TotalSent        int64     `json:"total_sent"`
	TotalDismissed   int64     `json:"total_dismissed"`
	WindowSent       int64     `json:"window_sent"`
	WindowDismissed  int64     `json:"window_dismissed"`
	LastUpdated      time.Time `json:"last_updated"`
}

// Rate returns the dismissal rate as a fraction (0.0 to 1.0).
// Returns 0 if no notifications have been sent.
func (d *DismissalRate) Rate() float64 {
	if d.TotalSent == 0 {
		return 0
	}
	return float64(d.TotalDismissed) / float64(d.TotalSent)
}

// WindowRate returns the dismissal rate for the rolling window.
func (d *DismissalRate) WindowRate() float64 {
	if d.WindowSent == 0 {
		return 0
	}
	return float64(d.WindowDismissed) / float64(d.WindowSent)
}

// DefaultPreferences returns sensible defaults for a project.
func DefaultPreferences(projectID string) *Preferences {
	now := time.Now().UTC()
	return &Preferences{
		ProjectID:       projectID,
		CriticalChannel: ChannelSlack,
		HighChannel:     ChannelSlack,
		MediumChannel:   ChannelSlack,
		LowChannel:      ChannelSlack,
		DigestEnabled:   true,
		DigestInterval:  "1h",
		PushThreshold:   UrgencyHigh,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
