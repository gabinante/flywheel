package hooks_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/events/hooks"
)

func TestGapDetector_NoGapWhenChangeEventExists(t *testing.T) {
	bus := events.NewInProcessBus()
	client := hooks.NewClient(bus)

	detector := hooks.NewGapDetector(bus, client,
		hooks.WithGapWindow(50*time.Millisecond),
		hooks.WithStateEventTypes("ticket.deploying"),
	)

	// Publish a change event first.
	_ = client.Publish(context.Background(), hooks.Deploy("api", "v2").By("ci"))

	// Then a state observation arrives.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "api"},
	})

	// Wait beyond the gap window, then sweep.
	time.Sleep(100 * time.Millisecond)
	detector.Sweep(context.Background())

	// No gaps should be detected — the change event explains the state change.
	if detector.PendingGaps() != 0 {
		t.Errorf("expected 0 pending gaps, got %d", detector.PendingGaps())
	}
}

func TestGapDetector_DetectsGapAndGeneratesUnattributed(t *testing.T) {
	bus := events.NewInProcessBus()
	client := hooks.NewClient(bus)

	// Capture unattributed events.
	var unattributed []events.Event
	bus.Subscribe(events.EventChangeUnattributed, func(_ context.Context, ev events.Event) {
		unattributed = append(unattributed, ev)
	})
	var gaps []events.Event
	bus.Subscribe(events.EventStateGapDetected, func(_ context.Context, ev events.Event) {
		gaps = append(gaps, ev)
	})

	detector := hooks.NewGapDetector(bus, client,
		hooks.WithGapWindow(50*time.Millisecond),
		hooks.WithStateEventTypes("ticket.deploying"),
	)

	// State observation without any preceding change event.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "orphan-service"},
	})

	if detector.PendingGaps() != 1 {
		t.Fatalf("expected 1 pending gap, got %d", detector.PendingGaps())
	}

	// Wait beyond the gap window, then sweep.
	time.Sleep(100 * time.Millisecond)
	detector.Sweep(context.Background())

	// Gap should be resolved by generating an unattributed event.
	if detector.PendingGaps() != 0 {
		t.Errorf("expected 0 pending gaps after sweep, got %d", detector.PendingGaps())
	}
	if len(gaps) != 1 {
		t.Errorf("expected 1 gap_detected event, got %d", len(gaps))
	}
	if len(unattributed) != 1 {
		t.Errorf("expected 1 unattributed event, got %d", len(unattributed))
	}
	if len(unattributed) > 0 {
		if unattributed[0].Payload["entity_id"] != "orphan-service" {
			t.Errorf("expected entity_id=orphan-service, got %v", unattributed[0].Payload["entity_id"])
		}
		if unattributed[0].Payload["change_type"] != "unattributed" {
			t.Errorf("expected change_type=unattributed, got %v", unattributed[0].Payload["change_type"])
		}
		meta, ok := unattributed[0].Payload["metadata"].(map[string]any)
		if !ok {
			t.Fatal("expected metadata in unattributed event")
		}
		if meta["auto_generated"] != true {
			t.Errorf("expected auto_generated=true")
		}
	}
}

func TestGapDetector_ChangeEventClearsGap(t *testing.T) {
	bus := events.NewInProcessBus()
	client := hooks.NewClient(bus)

	detector := hooks.NewGapDetector(bus, client,
		hooks.WithGapWindow(200*time.Millisecond),
		hooks.WithStateEventTypes("ticket.deploying"),
	)

	// State observation arrives.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "my-service"},
	})

	if detector.PendingGaps() != 1 {
		t.Fatalf("expected 1 pending gap, got %d", detector.PendingGaps())
	}

	// Change event arrives (within gap window), explaining the state change.
	_ = client.Publish(context.Background(), hooks.Deploy("my-service", "v2").By("pipeline"))

	if detector.PendingGaps() != 0 {
		t.Errorf("expected 0 pending gaps after change event, got %d", detector.PendingGaps())
	}

	// Sweep should find nothing to report.
	time.Sleep(250 * time.Millisecond)
	var unattributed []events.Event
	bus.Subscribe(events.EventChangeUnattributed, func(_ context.Context, ev events.Event) {
		unattributed = append(unattributed, ev)
	})
	detector.Sweep(context.Background())

	if len(unattributed) != 0 {
		t.Errorf("expected no unattributed events, got %d", len(unattributed))
	}
}

func TestGapDetector_MultipleEntities(t *testing.T) {
	bus := events.NewInProcessBus()
	client := hooks.NewClient(bus)

	var unattributed []events.Event
	bus.Subscribe(events.EventChangeUnattributed, func(_ context.Context, ev events.Event) {
		unattributed = append(unattributed, ev)
	})

	detector := hooks.NewGapDetector(bus, client,
		hooks.WithGapWindow(50*time.Millisecond),
		hooks.WithStateEventTypes("ticket.deploying"),
	)

	// Two state observations.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "svc-a"},
	})
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "svc-b"},
	})

	// Explain svc-a but not svc-b.
	_ = client.Publish(context.Background(), hooks.Deploy("svc-a", "v1"))

	time.Sleep(100 * time.Millisecond)
	detector.Sweep(context.Background())

	// Only svc-b should generate an unattributed event.
	if len(unattributed) != 1 {
		t.Fatalf("expected 1 unattributed event, got %d", len(unattributed))
	}
	if unattributed[0].Payload["entity_id"] != "svc-b" {
		t.Errorf("expected entity_id=svc-b, got %v", unattributed[0].Payload["entity_id"])
	}
}

func TestGapDetector_StartAndCancel(t *testing.T) {
	bus := events.NewInProcessBus()
	client := hooks.NewClient(bus)

	detector := hooks.NewGapDetector(bus, client,
		hooks.WithGapWindow(10*time.Millisecond),
		hooks.WithSweepInterval(20*time.Millisecond),
		hooks.WithStateEventTypes("ticket.deploying"),
	)

	var unattributed []events.Event
	bus.Subscribe(events.EventChangeUnattributed, func(_ context.Context, ev events.Event) {
		unattributed = append(unattributed, ev)
	})

	// State observation.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    "ticket.deploying",
		Payload: map[string]any{"ticket_id": "auto-detect-me"},
	})

	// Start the detector in background, let it sweep automatically.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		detector.Start(ctx)
		close(done)
	}()

	// Wait for at least one sweep cycle.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if len(unattributed) < 1 {
		t.Errorf("expected at least 1 auto-detected unattributed event, got %d", len(unattributed))
	}
}
