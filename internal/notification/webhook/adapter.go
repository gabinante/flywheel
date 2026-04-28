package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/notification"
	"github.com/google/uuid"
)

// Adapter implements the notification.ChannelAdapter interface for webhooks.
// It creates WebhookEvent records and publishes delivery jobs to the event bus.
type Adapter struct {
	store Store
	bus   events.Bus
}

// NewAdapter creates a webhook notification adapter.
func NewAdapter(store Store, bus events.Bus) *Adapter {
	return &Adapter{
		store: store,
		bus:   bus,
	}
}

// Name returns "webhook".
func (a *Adapter) Name() string { return "webhook" }

// Send creates a webhook event and queues it for delivery.
func (a *Adapter) Send(ctx context.Context, n *notification.Notification, _ *notification.Preferences) error {
	eventType := classifyEventType(n)

	event := &Event{
		ID:           "evt_" + uuid.New().String(),
		ProjectID:    n.ProjectID,
		EventType:    eventType,
		Payload:      buildPayload(n),
		Status:       EventStatusPending,
		AttemptCount: 0,
		MaxAttempts:  MaxAttempts,
		CreatedAt:    time.Now().UTC(),
	}

	if err := a.store.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("create webhook event: %w", err)
	}

	// Publish a delivery job event.
	_ = a.bus.Publish(ctx, events.Event{
		Type: events.EventWebhookDeliveryQueued,
		Payload: map[string]any{
			"webhook_event_id": event.ID,
			"project_id":       event.ProjectID,
			"event_type":       event.EventType,
		},
	})

	slog.Info("webhook adapter: event queued for delivery",
		"webhook_event_id", event.ID, "event_type", eventType)

	return nil
}

// SendDigest creates individual webhook events for each notification in the digest.
func (a *Adapter) SendDigest(ctx context.Context, notifications []*notification.Notification, prefs *notification.Preferences) error {
	for _, n := range notifications {
		if err := a.Send(ctx, n, prefs); err != nil {
			slog.Error("webhook adapter: failed to send digest notification",
				"notification_id", n.ID, "error", err)
		}
	}
	return nil
}

// classifyEventType maps a notification to a webhook event type.
func classifyEventType(n *notification.Notification) string {
	// Check if the notification context has an explicit event_type.
	if n.Context != nil {
		if et, ok := n.Context["event_type"].(string); ok && et != "" {
			return et
		}
	}

	// Map category to default event type.
	switch n.Category {
	case notification.CategoryUrgentDecision:
		return "notification.urgent"
	case notification.CategoryAutonomousAction:
		return "notification.action"
	case notification.CategoryCalibrationReview:
		return "notification.review"
	case notification.CategoryAnomalyAlert:
		return "notification.alert"
	default:
		return "notification.general"
	}
}

// buildPayload converts a notification into a webhook payload map.
func buildPayload(n *notification.Notification) map[string]any {
	payload := map[string]any{
		"notification_id": n.ID,
		"title":           n.Title,
		"category":        string(n.Category),
		"urgency":         string(n.Urgency),
	}
	if n.Body != "" {
		payload["body"] = n.Body
	}
	if n.TicketID != "" {
		payload["ticket_id"] = n.TicketID
	}
	if n.Context != nil {
		contextJSON, err := json.Marshal(n.Context)
		if err == nil {
			var ctx map[string]any
			if json.Unmarshal(contextJSON, &ctx) == nil {
				for k, v := range ctx {
					if k != "event_type" { // Don't duplicate event_type in data.
						payload[k] = v
					}
				}
			}
		}
	}
	return payload
}
