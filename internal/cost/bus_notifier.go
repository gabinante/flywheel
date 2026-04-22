package cost

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/events"
)

// BusNotifier adapts the event bus to both AlertNotifier and RateLimitNotifier interfaces.
// It publishes budget alerts and rate-limit notifications as events on the bus.
type BusNotifier struct {
	bus events.Bus
}

// NewBusNotifier creates a notifier that publishes to the event bus.
func NewBusNotifier(bus events.Bus) *BusNotifier {
	return &BusNotifier{bus: bus}
}

// NotifyBudgetAlert publishes a budget alert event.
func (n *BusNotifier) NotifyBudgetAlert(ctx context.Context, alert *BudgetAlert) error {
	return n.bus.Publish(ctx, events.Event{
		Type: events.EventBudgetAlert,
		Payload: map[string]any{
			"alert_id":     alert.ID,
			"project_id":   alert.ProjectID,
			"ticket_id":    alert.TicketID,
			"alert_type":   string(alert.AlertType),
			"message":      alert.Message,
			"spent":        alert.Status.SpentMilli.ToDollars(),
			"limit":        alert.Status.Budget.LimitMilli.ToDollars(),
			"used_fraction": alert.Status.UsedFraction,
			"over_budget":  alert.Status.OverBudget,
		},
	})
}

// NotifyRateLimit publishes a rate-limit hit event.
func (n *BusNotifier) NotifyRateLimit(ctx context.Context, event *RateLimitEvent) error {
	return n.bus.Publish(ctx, events.Event{
		Type: events.EventRateLimitHit,
		Payload: map[string]any{
			"event_id":    event.ID,
			"project_id":  event.ProjectID,
			"ticket_id":   event.TicketID,
			"provider":    event.Provider,
			"model":       event.Model,
			"retry_after": event.RetryAfter.String(),
			"reset_at":    event.ResetAt.Format(time.RFC3339),
		},
	})
}

// NotifyRateLimitResume publishes a rate-limit resumed event.
func (n *BusNotifier) NotifyRateLimitResume(ctx context.Context, event *RateLimitEvent) error {
	return n.bus.Publish(ctx, events.Event{
		Type: events.EventRateLimitResumed,
		Payload: map[string]any{
			"event_id":   event.ID,
			"project_id": event.ProjectID,
			"ticket_id":  event.TicketID,
			"provider":   event.Provider,
			"model":      event.Model,
			"resumed_at": event.ResumedAt.Format(time.RFC3339),
		},
	})
}
