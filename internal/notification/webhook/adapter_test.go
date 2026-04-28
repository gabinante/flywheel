package webhook

import (
	"context"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/notification"
)

func TestAdapter_Name(t *testing.T) {
	adapter := NewAdapter(newMemoryStore(), events.NewInProcessBus())
	if adapter.Name() != "webhook" {
		t.Errorf("expected name 'webhook', got %q", adapter.Name())
	}
}

func TestAdapter_Send_CreatesEventAndPublishes(t *testing.T) {
	store := newMemoryStore()
	bus := events.NewInProcessBus()

	var publishedEvent events.Event
	bus.Subscribe(events.EventWebhookDeliveryQueued, func(_ context.Context, e events.Event) {
		publishedEvent = e
	})

	adapter := NewAdapter(store, bus)

	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		Category:  notification.CategoryAnomalyAlert,
		Urgency:   notification.UrgencyHigh,
		Title:     "Test Alert",
		Body:      "Something happened",
		Context: map[string]any{
			"event_type": "payment.completed",
		},
	}

	err := adapter.Send(context.Background(), n, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify event was created in store.
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event in store, got %d", len(store.events))
	}

	// Find the created event.
	var created *Event
	for _, e := range store.events {
		created = e
	}

	if created.EventType != "payment.completed" {
		t.Errorf("expected event type 'payment.completed', got %q", created.EventType)
	}
	if created.Status != EventStatusPending {
		t.Errorf("expected status pending, got %q", created.Status)
	}
	if created.MaxAttempts != MaxAttempts {
		t.Errorf("expected max attempts %d, got %d", MaxAttempts, created.MaxAttempts)
	}

	// Verify bus event was published.
	if publishedEvent.Type != events.EventWebhookDeliveryQueued {
		t.Errorf("expected published event type %q, got %q",
			events.EventWebhookDeliveryQueued, publishedEvent.Type)
	}
	if publishedEvent.Payload["project_id"] != "proj-1" {
		t.Errorf("expected project_id=proj-1 in published event")
	}
}

func TestAdapter_SendDigest(t *testing.T) {
	store := newMemoryStore()
	bus := events.NewInProcessBus()

	var deliveryCount int
	bus.Subscribe(events.EventWebhookDeliveryQueued, func(_ context.Context, _ events.Event) {
		deliveryCount++
	})

	adapter := NewAdapter(store, bus)

	notifications := []*notification.Notification{
		{ID: "n-1", ProjectID: "proj-1", Category: notification.CategoryAnomalyAlert, Title: "Alert 1"},
		{ID: "n-2", ProjectID: "proj-1", Category: notification.CategoryAnomalyAlert, Title: "Alert 2"},
		{ID: "n-3", ProjectID: "proj-1", Category: notification.CategoryAnomalyAlert, Title: "Alert 3"},
	}

	err := adapter.SendDigest(context.Background(), notifications, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.events) != 3 {
		t.Errorf("expected 3 events, got %d", len(store.events))
	}
	if deliveryCount != 3 {
		t.Errorf("expected 3 delivery events, got %d", deliveryCount)
	}
}

func TestClassifyEventType_FromContext(t *testing.T) {
	n := &notification.Notification{
		Context: map[string]any{
			"event_type": "payment.refunded",
		},
	}
	if got := classifyEventType(n); got != "payment.refunded" {
		t.Errorf("expected 'payment.refunded', got %q", got)
	}
}

func TestClassifyEventType_FromCategory(t *testing.T) {
	tests := []struct {
		category notification.Category
		expected string
	}{
		{notification.CategoryUrgentDecision, "notification.urgent"},
		{notification.CategoryAutonomousAction, "notification.action"},
		{notification.CategoryCalibrationReview, "notification.review"},
		{notification.CategoryAnomalyAlert, "notification.alert"},
	}
	for _, tc := range tests {
		n := &notification.Notification{Category: tc.category}
		got := classifyEventType(n)
		if got != tc.expected {
			t.Errorf("category %q: expected %q, got %q", tc.category, tc.expected, got)
		}
	}
}

func TestBuildPayload(t *testing.T) {
	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		TicketID:  "ticket-1",
		Category:  notification.CategoryAnomalyAlert,
		Urgency:   notification.UrgencyHigh,
		Title:     "Test Alert",
		Body:      "Something happened",
	}

	payload := buildPayload(n)

	if payload["notification_id"] != "notif-1" {
		t.Errorf("expected notification_id=notif-1")
	}
	if payload["title"] != "Test Alert" {
		t.Errorf("expected title='Test Alert'")
	}
	if payload["ticket_id"] != "ticket-1" {
		t.Errorf("expected ticket_id=ticket-1")
	}
	if payload["body"] != "Something happened" {
		t.Errorf("expected body='Something happened'")
	}
}
