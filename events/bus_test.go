package events

import (
	"context"
	"testing"
)

func TestInProcessBus_Publish(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	received := false
	bus.Subscribe("test.event", func(_ context.Context, e Event) {
		received = true
		if e.Type != "test.event" {
			t.Errorf("unexpected type: %s", e.Type)
		}
	})

	if err := bus.Publish(ctx, Event{Type: "test.event", Payload: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Error("handler was not called")
	}
}

func TestInProcessBus_NoSubscriber(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	// Should not panic.
	if err := bus.Publish(ctx, Event{Type: "no.subscriber", Payload: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
}

func TestInProcessBus_MultipleSubscribers(t *testing.T) {
	bus := NewInProcessBus()
	ctx := context.Background()

	count := 0
	bus.Subscribe("test.event", func(_ context.Context, _ Event) { count++ })
	bus.Subscribe("test.event", func(_ context.Context, _ Event) { count++ })

	_ = bus.Publish(ctx, Event{Type: "test.event", Payload: map[string]any{}})
	if count != 2 {
		t.Errorf("expected 2 calls, got %d", count)
	}
}
