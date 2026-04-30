package progress

import (
	"context"
	"sync"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

type fakeInjector struct {
	mu     sync.Mutex
	events []injectedEvent
}

type injectedEvent struct {
	ProjectID string
	Category  string
	Summary   string
}

func (f *fakeInjector) InjectSystemEvent(_ context.Context, projectID, category, summary string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, injectedEvent{projectID, category, summary})
	return nil
}

func (f *fakeInjector) list() []injectedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]injectedEvent, len(f.events))
	copy(out, f.events)
	return out
}

type fakeTicketGetter struct {
	tickets map[string]*ticket.Ticket
}

func (f *fakeTicketGetter) GetTicket(_ context.Context, id string) (*ticket.Ticket, error) {
	if t, ok := f.tickets[id]; ok {
		return t, nil
	}
	return nil, nil
}

func ticketWithAttempts(id, projectID string, attempts int) *ticket.Ticket {
	t := &ticket.Ticket{ID: id, ProjectID: projectID}
	for i := 0; i < attempts; i++ {
		t.Context.PriorAttempts = append(t.Context.PriorAttempts, ticket.AttemptSummary{
			Outcome: "failed",
		})
	}
	return t
}

func TestMonitor_Escalation(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	NewMonitor(bus, inj, nil)

	bus.Publish(context.Background(), events.Event{
		Type: events.EventTicketEscalated,
		Payload: map[string]any{
			"ticket_id":  "t-1",
			"project_id": "p-1",
			"reason":     "blocked on auth",
		},
	})

	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "ESCALATION" {
		t.Errorf("expected ESCALATION, got %s", got[0].Category)
	}
	if got[0].ProjectID != "p-1" {
		t.Errorf("expected project p-1, got %s", got[0].ProjectID)
	}
}

func TestMonitor_FailureThreshold(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	tickets := &fakeTicketGetter{tickets: map[string]*ticket.Ticket{
		"t-1": ticketWithAttempts("t-1", "p-1", 1), // below threshold
		"t-2": ticketWithAttempts("t-2", "p-1", 3), // above threshold
	}}
	NewMonitor(bus, inj, tickets)

	// First failure: below threshold → no event
	bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketFailed,
		Payload: map[string]any{"ticket_id": "t-1", "project_id": "p-1"},
	})
	if len(inj.list()) != 0 {
		t.Fatal("expected no event for ticket below threshold")
	}

	// Second failure: above threshold → event
	bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketFailed,
		Payload: map[string]any{"ticket_id": "t-2", "project_id": "p-1"},
	})
	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "FAILURE" {
		t.Errorf("expected FAILURE, got %s", got[0].Category)
	}
}

func TestMonitor_LeaseExpiredThreshold(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	tickets := &fakeTicketGetter{tickets: map[string]*ticket.Ticket{
		"t-1": ticketWithAttempts("t-1", "p-1", 0),
		"t-2": ticketWithAttempts("t-2", "p-1", 2),
	}}
	NewMonitor(bus, inj, tickets)

	bus.Publish(context.Background(), events.Event{
		Type:    events.EventLeaseExpired,
		Payload: map[string]any{"ticket_id": "t-1", "project_id": "p-1"},
	})
	if len(inj.list()) != 0 {
		t.Fatal("expected no event for ticket below threshold")
	}

	bus.Publish(context.Background(), events.Event{
		Type:    events.EventLeaseExpired,
		Payload: map[string]any{"ticket_id": "t-2", "project_id": "p-1"},
	})
	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "STALE" {
		t.Errorf("expected STALE, got %s", got[0].Category)
	}
}

func TestMonitor_GateReached(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	NewMonitor(bus, inj, nil)

	bus.Publish(context.Background(), events.Event{
		Type: events.EventWorkflowGateReached,
		Payload: map[string]any{
			"ticket_id":  "t-1",
			"project_id": "p-1",
			"phase_name": "deploy-approval",
		},
	})

	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "GATE" {
		t.Errorf("expected GATE, got %s", got[0].Category)
	}
}

func TestMonitor_WorkStreamCompleted(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	NewMonitor(bus, inj, nil)

	bus.Publish(context.Background(), events.Event{
		Type: events.EventWorkStreamCompleted,
		Payload: map[string]any{
			"project_id":     "p-1",
			"work_stream_id": "ws-1",
			"ticket_count":   float64(5),
		},
	})

	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "COMPLETE" {
		t.Errorf("expected COMPLETE, got %s", got[0].Category)
	}
}

func TestMonitor_Invalidated(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	NewMonitor(bus, inj, nil)

	bus.Publish(context.Background(), events.Event{
		Type: events.EventTicketInvalidated,
		Payload: map[string]any{
			"ticket_id":  "t-1",
			"project_id": "p-1",
		},
	})

	got := inj.list()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Category != "INVALIDATED" {
		t.Errorf("expected INVALIDATED, got %s", got[0].Category)
	}
}

func TestMonitor_MissingPayload(t *testing.T) {
	bus := events.NewInProcessBus()
	inj := &fakeInjector{}
	NewMonitor(bus, inj, nil)

	// Missing project_id → should be silently ignored
	bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketEscalated,
		Payload: map[string]any{"ticket_id": "t-1"},
	})
	// Missing ticket_id → should be silently ignored
	bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketEscalated,
		Payload: map[string]any{"project_id": "p-1"},
	})

	if len(inj.list()) != 0 {
		t.Fatal("expected no events for incomplete payloads")
	}
}
