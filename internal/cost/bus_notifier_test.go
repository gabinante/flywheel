package cost

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
)

func TestBusNotifierBudgetAlert(t *testing.T) {
	bus := events.NewInProcessBus()
	notifier := NewBusNotifier(bus)

	// Subscribe to budget alerts.
	var received []events.Event
	bus.Subscribe(events.EventBudgetAlert, func(_ context.Context, e events.Event) {
		received = append(received, e)
	})

	ctx := context.Background()
	alert := &BudgetAlert{
		ID:        "alert-1",
		ProjectID: "proj-1",
		TicketID:  "ticket-1",
		AlertType: AlertWarning,
		Message:   "Budget warning: 85% used",
		Status: BudgetStatus{
			Budget:       Budget{LimitMilli: FromDollars(10.0)},
			SpentMilli:   FromDollars(8.5),
			UsedFraction: 0.85,
			OverBudget:   false,
		},
		CreatedAt: time.Now().UTC(),
	}

	err := notifier.NotifyBudgetAlert(ctx, alert)
	if err != nil {
		t.Fatalf("NotifyBudgetAlert: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	evt := received[0]
	if evt.Type != events.EventBudgetAlert {
		t.Errorf("expected type %s, got %s", events.EventBudgetAlert, evt.Type)
	}
	if evt.Payload["alert_id"] != "alert-1" {
		t.Errorf("expected alert_id=alert-1, got %v", evt.Payload["alert_id"])
	}
	if evt.Payload["project_id"] != "proj-1" {
		t.Errorf("expected project_id=proj-1, got %v", evt.Payload["project_id"])
	}
	if evt.Payload["alert_type"] != "warning" {
		t.Errorf("expected alert_type=warning, got %v", evt.Payload["alert_type"])
	}
}

func TestBusNotifierRateLimit(t *testing.T) {
	bus := events.NewInProcessBus()
	notifier := NewBusNotifier(bus)

	// Subscribe to rate limit events.
	var hitEvents []events.Event
	var resumeEvents []events.Event
	bus.Subscribe(events.EventRateLimitHit, func(_ context.Context, e events.Event) {
		hitEvents = append(hitEvents, e)
	})
	bus.Subscribe(events.EventRateLimitResumed, func(_ context.Context, e events.Event) {
		resumeEvents = append(resumeEvents, e)
	})

	ctx := context.Background()
	event := &RateLimitEvent{
		ID:         "rl-1",
		ProjectID:  "proj-1",
		TicketID:   "ticket-1",
		Provider:   "anthropic",
		Model:      "claude-sonnet-4-20250514",
		RetryAfter: 30 * time.Second,
		ResetAt:    time.Now().UTC().Add(30 * time.Second),
		Timestamp:  time.Now().UTC(),
	}

	// Test rate limit hit notification.
	err := notifier.NotifyRateLimit(ctx, event)
	if err != nil {
		t.Fatalf("NotifyRateLimit: %v", err)
	}

	if len(hitEvents) != 1 {
		t.Fatalf("expected 1 hit event, got %d", len(hitEvents))
	}
	if hitEvents[0].Payload["provider"] != "anthropic" {
		t.Errorf("expected provider=anthropic, got %v", hitEvents[0].Payload["provider"])
	}

	// Test rate limit resume notification.
	event.ResumedAt = time.Now().UTC()
	err = notifier.NotifyRateLimitResume(ctx, event)
	if err != nil {
		t.Fatalf("NotifyRateLimitResume: %v", err)
	}

	if len(resumeEvents) != 1 {
		t.Fatalf("expected 1 resume event, got %d", len(resumeEvents))
	}
	if resumeEvents[0].Payload["event_id"] != "rl-1" {
		t.Errorf("expected event_id=rl-1, got %v", resumeEvents[0].Payload["event_id"])
	}
}

func TestBusNotifierImplementsInterfaces(t *testing.T) {
	bus := events.NewInProcessBus()
	notifier := NewBusNotifier(bus)

	// Compile-time interface checks.
	var _ AlertNotifier = notifier
	var _ RateLimitNotifier = notifier
}
