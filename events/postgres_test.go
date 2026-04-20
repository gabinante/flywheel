package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPostgresBus_Interface verifies PostgresBus satisfies DurableEventBus.
func TestPostgresBus_Interface(t *testing.T) {
	// Compile-time interface check.
	var _ DurableEventBus = (*PostgresBus)(nil)
	var _ DurableEventBus = (*InProcessBus)(nil)
}

// TestInProcessBus_PatternSubscription tests pattern matching with in-process bus.
func TestInProcessBus_PatternSubscription(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	var mu sync.Mutex
	received := make([]string, 0)

	// Subscribe to all ticket events.
	_ = bus.SubscribePattern("ticket.*", "test-sub", func(_ context.Context, e Event) {
		mu.Lock()
		received = append(received, e.Type)
		mu.Unlock()
	})

	// Publish events.
	_ = bus.Publish(ctx, Event{Type: "ticket.created", Payload: map[string]any{}})
	_ = bus.Publish(ctx, Event{Type: "ticket.closed", Payload: map[string]any{}})
	_ = bus.Publish(ctx, Event{Type: "project.created", Payload: map[string]any{}}) // Should not match.

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(received), received)
	}
	if received[0] != "ticket.created" {
		t.Errorf("expected ticket.created, got %s", received[0])
	}
	if received[1] != "ticket.closed" {
		t.Errorf("expected ticket.closed, got %s", received[1])
	}
}

// TestInProcessBus_ExactAndPattern tests that both exact and pattern subscriptions fire.
func TestInProcessBus_ExactAndPattern(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	var mu sync.Mutex
	exactReceived := 0
	patternReceived := 0

	bus.Subscribe("ticket.created", func(_ context.Context, _ Event) {
		mu.Lock()
		exactReceived++
		mu.Unlock()
	})

	_ = bus.SubscribePattern("ticket.*", "pattern-sub", func(_ context.Context, _ Event) {
		mu.Lock()
		patternReceived++
		mu.Unlock()
	})

	_ = bus.Publish(ctx, Event{Type: "ticket.created", Payload: map[string]any{}})

	mu.Lock()
	defer mu.Unlock()
	if exactReceived != 1 {
		t.Errorf("exact: expected 1, got %d", exactReceived)
	}
	if patternReceived != 1 {
		t.Errorf("pattern: expected 1, got %d", patternReceived)
	}
}

// TestInProcessBus_PublishDurable returns empty ID (no durability).
func TestInProcessBus_PublishDurable(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	id, err := bus.PublishDurable(ctx, Event{Type: "test.event", Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty ID for in-process bus, got %q", id)
	}
}

// TestInProcessBus_StartStop verifies no-op lifecycle.
func TestInProcessBus_StartStop(t *testing.T) {
	bus := NewInProcessBus()
	if err := bus.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := bus.Stop(); err != nil {
		t.Fatal(err)
	}
}

// TestInProcessBus_Ack verifies no-op ack.
func TestInProcessBus_Ack(t *testing.T) {
	bus := NewInProcessBus()
	if err := bus.Ack(context.Background(), "event-1", "sub-1"); err != nil {
		t.Fatal(err)
	}
}

// TestNewEvent creates an event with type and payload.
func TestNewEvent(t *testing.T) {
	e := NewEvent("ticket.created", map[string]any{"ticket_id": "abc"})
	if e.Type != "ticket.created" {
		t.Errorf("type: got %q", e.Type)
	}
	if e.Payload["ticket_id"] != "abc" {
		t.Errorf("payload: got %v", e.Payload)
	}
	if e.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

// TestEvent_WithEntityKey returns a copy with entity key set.
func TestEvent_WithEntityKey(t *testing.T) {
	e := NewEvent("ticket.created", map[string]any{}).WithEntityKey("ticket:123")
	if e.EntityKey != "ticket:123" {
		t.Errorf("entity key: got %q", e.EntityKey)
	}

	// Original should not be modified (value semantics).
	e2 := NewEvent("ticket.created", map[string]any{})
	_ = e2.WithEntityKey("ticket:456")
	if e2.EntityKey != "" {
		t.Errorf("original modified: got %q", e2.EntityKey)
	}
}

// TestPostgresBusConfig_Defaults verifies default config values.
func TestPostgresBusConfig_Defaults(t *testing.T) {
	bus := NewPostgresBus(nil, PostgresBusConfig{})
	if bus.config.PollInterval != defaultPollInterval {
		t.Errorf("poll interval: got %v", bus.config.PollInterval)
	}
	if bus.config.RetentionDays != defaultRetentionDays {
		t.Errorf("retention days: got %d", bus.config.RetentionDays)
	}
}

// TestPostgresBusConfig_Custom verifies custom config values.
func TestPostgresBusConfig_Custom(t *testing.T) {
	bus := NewPostgresBus(nil, PostgresBusConfig{
		PollInterval:  10 * time.Second,
		RetentionDays: 30,
	})
	if bus.config.PollInterval != 10*time.Second {
		t.Errorf("poll interval: got %v", bus.config.PollInterval)
	}
	if bus.config.RetentionDays != 30 {
		t.Errorf("retention days: got %d", bus.config.RetentionDays)
	}
}
