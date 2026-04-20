package mirror

import (
	"context"
	"sync"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// mockAdapter records calls for testing.
type mockAdapter struct {
	mu             sync.Mutex
	name           string
	createdTickets []TicketData
	stateUpdates   []stateUpdate
	fieldUpdates   []fieldUpdate
	closedTickets  []closeCall
	createReturn   string
	createErr      error
}

type stateUpdate struct {
	externalID string
	newState   string
	data       TicketData
}

type fieldUpdate struct {
	externalID string
	fields     CustomFields
}

type closeCall struct {
	externalID string
	contextURL string
	data       TicketData
}

func (m *mockAdapter) Name() string { return m.name }

func (m *mockAdapter) CreateTicket(_ context.Context, _ *Config, data TicketData) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdTickets = append(m.createdTickets, data)
	if m.createErr != nil {
		return "", m.createErr
	}
	return m.createReturn, nil
}

func (m *mockAdapter) UpdateState(_ context.Context, _ *Config, externalID string, newState string, data TicketData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateUpdates = append(m.stateUpdates, stateUpdate{externalID, newState, data})
	return nil
}

func (m *mockAdapter) UpdateFields(_ context.Context, _ *Config, externalID string, fields CustomFields) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fieldUpdates = append(m.fieldUpdates, fieldUpdate{externalID, fields})
	return nil
}

func (m *mockAdapter) CloseTicket(_ context.Context, _ *Config, externalID string, contextURL string, data TicketData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closedTickets = append(m.closedTickets, closeCall{externalID, contextURL, data})
	return nil
}

// mockTicketGetter returns a fixed ticket.
type mockTicketGetter struct {
	tickets map[string]*ticket.Ticket
}

func (m *mockTicketGetter) GetTicket(_ context.Context, id string) (*ticket.Ticket, error) {
	t, ok := m.tickets[id]
	if !ok {
		return nil, &notFoundError{id}
	}
	return t, nil
}

type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "not found: " + e.id }

// mockProjectGetter returns a fixed project.
type mockProjectGetter struct {
	projects map[string]*project.Project
}

func (m *mockProjectGetter) GetProject(_ context.Context, id string) (*project.Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return nil, &notFoundError{id}
	}
	return p, nil
}

func TestServiceHandleTicketCreated(t *testing.T) {
	bus := events.NewInProcessBus()
	adapter := &mockAdapter{name: "linear", createReturn: "LIN-123"}

	tickets := &mockTicketGetter{tickets: map[string]*ticket.Ticket{
		"proj-1": {
			ID:        "proj-1",
			ProjectID: "project-uuid",
			Title:     "Test ticket",
			Type:      ticket.TypeTask,
			Priority:  ticket.P1,
			State:     ticket.StateDraft,
			Objective: ticket.Objective{Description: "Do the thing"},
			Inputs:    map[string]any{},
			Outputs:   map[string]any{},
		},
	}}

	projects := &mockProjectGetter{projects: map[string]*project.Project{
		"project-uuid": {
			ID:   "project-uuid",
			Slug: "proj",
			ContextPack: project.ContextPack{
				Extra: map[string]string{
					"mirror_config": `{"enabled":true,"provider":"linear","state_mapping":{"executing":"In Progress","observing":"In Review"},"linear":{"team_id":"team-1","api_key_secret":"linear_key"},"context_base_url":"https://warrant.example.com"}`,
				},
			},
		},
	}}

	svc := NewService(tickets, projects, bus)
	svc.RegisterAdapter("linear", adapter)

	// Publish a ticket created event.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "proj-1"},
	})

	// Verify the adapter was called.
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.createdTickets) != 1 {
		t.Fatalf("expected 1 created ticket, got %d", len(adapter.createdTickets))
	}
	created := adapter.createdTickets[0]
	if created.ID != "proj-1" {
		t.Errorf("expected ticket ID proj-1, got %s", created.ID)
	}
	if created.Title != "Test ticket" {
		t.Errorf("expected title 'Test ticket', got %s", created.Title)
	}
	if created.URL != "https://warrant.example.com/#/tickets/proj-1" {
		t.Errorf("expected URL, got %s", created.URL)
	}

	// Verify external ID was stored.
	extID, ok := svc.GetExternalID("proj-1")
	if !ok || extID != "LIN-123" {
		t.Errorf("expected external ID LIN-123, got %s (ok=%v)", extID, ok)
	}
}

func TestServiceHandleStateTransition(t *testing.T) {
	bus := events.NewInProcessBus()
	adapter := &mockAdapter{name: "linear", createReturn: "LIN-456"}

	tickets := &mockTicketGetter{tickets: map[string]*ticket.Ticket{
		"proj-2": {
			ID:          "proj-2",
			ProjectID:   "project-uuid",
			Title:       "State test",
			Type:        ticket.TypeTask,
			Priority:    ticket.P2,
			State:       ticket.StateExecuting,
			Environment: ticket.EnvProduction,
			Objective:   ticket.Objective{Description: "Test state mapping"},
			Inputs: map[string]any{
				"risk_classification": "high",
				"linked_services":     []any{"svc-a", "svc-b"},
			},
			Outputs: map[string]any{},
		},
	}}

	projects := &mockProjectGetter{projects: map[string]*project.Project{
		"project-uuid": {
			ID:   "project-uuid",
			Slug: "proj",
			ContextPack: project.ContextPack{
				Extra: map[string]string{
					"mirror_config": `{"enabled":true,"provider":"linear","state_mapping":{"executing":"In Progress","observing":"In Review","closed":"Done"},"linear":{"team_id":"team-1","api_key_secret":"linear_key"},"context_base_url":"https://warrant.example.com"}`,
				},
			},
		},
	}}

	svc := NewService(tickets, projects, bus)
	svc.RegisterAdapter("linear", adapter)
	// Pre-set the external ID (simulating a previously created mirror).
	svc.SetExternalID("proj-2", "LIN-456")

	// Publish a state transition event.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "proj-2", "state": "executing"},
	})

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.stateUpdates) != 1 {
		t.Fatalf("expected 1 state update, got %d", len(adapter.stateUpdates))
	}
	update := adapter.stateUpdates[0]
	if update.externalID != "LIN-456" {
		t.Errorf("expected external ID LIN-456, got %s", update.externalID)
	}
	if update.newState != "In Progress" {
		t.Errorf("expected mapped state 'In Progress', got %s", update.newState)
	}

	// Verify custom fields were also updated.
	if len(adapter.fieldUpdates) != 1 {
		t.Fatalf("expected 1 field update, got %d", len(adapter.fieldUpdates))
	}
	fields := adapter.fieldUpdates[0].fields
	if fields.RiskClassification != "high" {
		t.Errorf("expected risk 'high', got %s", fields.RiskClassification)
	}
	if fields.EnvironmentTarget != "production" {
		t.Errorf("expected env 'production', got %s", fields.EnvironmentTarget)
	}
	if len(fields.LinkedServices) != 2 {
		t.Errorf("expected 2 linked services, got %d", len(fields.LinkedServices))
	}
}

func TestServiceHandleTicketClosed(t *testing.T) {
	bus := events.NewInProcessBus()
	adapter := &mockAdapter{name: "linear"}

	tickets := &mockTicketGetter{tickets: map[string]*ticket.Ticket{
		"proj-3": {
			ID:        "proj-3",
			ProjectID: "project-uuid",
			Title:     "Close test",
			Type:      ticket.TypeTask,
			State:     ticket.StateClosed,
			Objective: ticket.Objective{Description: "Done"},
			Inputs:    map[string]any{},
			Outputs:   map[string]any{},
		},
	}}

	projects := &mockProjectGetter{projects: map[string]*project.Project{
		"project-uuid": {
			ID:   "project-uuid",
			Slug: "proj",
			ContextPack: project.ContextPack{
				Extra: map[string]string{
					"mirror_config": `{"enabled":true,"provider":"linear","state_mapping":{"closed":"Done"},"linear":{"team_id":"team-1","api_key_secret":"linear_key"},"context_base_url":"https://warrant.example.com"}`,
				},
			},
		},
	}}

	svc := NewService(tickets, projects, bus)
	svc.RegisterAdapter("linear", adapter)
	svc.SetExternalID("proj-3", "LIN-789")

	// Publish close event.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketClosed,
		Payload: map[string]any{"ticket_id": "proj-3", "state": "closed"},
	})

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.closedTickets) != 1 {
		t.Fatalf("expected 1 close call, got %d", len(adapter.closedTickets))
	}
	closed := adapter.closedTickets[0]
	if closed.externalID != "LIN-789" {
		t.Errorf("expected external ID LIN-789, got %s", closed.externalID)
	}
	if closed.contextURL != "https://warrant.example.com/#/tickets/proj-3" {
		t.Errorf("expected context URL, got %s", closed.contextURL)
	}
}

func TestServiceMirroringDisabled(t *testing.T) {
	bus := events.NewInProcessBus()
	adapter := &mockAdapter{name: "linear", createReturn: "LIN-999"}

	tickets := &mockTicketGetter{tickets: map[string]*ticket.Ticket{
		"proj-4": {
			ID:        "proj-4",
			ProjectID: "project-uuid",
			Title:     "No mirror",
			Type:      ticket.TypeTask,
			State:     ticket.StateDraft,
			Objective: ticket.Objective{Description: "Not mirrored"},
			Inputs:    map[string]any{},
			Outputs:   map[string]any{},
		},
	}}

	// Project without mirror config - mirroring is opt-in.
	projects := &mockProjectGetter{projects: map[string]*project.Project{
		"project-uuid": {
			ID:   "project-uuid",
			Slug: "proj",
			ContextPack: project.ContextPack{
				Extra: map[string]string{},
			},
		},
	}}

	svc := NewService(tickets, projects, bus)
	svc.RegisterAdapter("linear", adapter)

	_ = bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "proj-4"},
	})

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.createdTickets) != 0 {
		t.Errorf("expected no created tickets when mirroring disabled, got %d", len(adapter.createdTickets))
	}
}

func TestServiceUnmappedStateSkipped(t *testing.T) {
	bus := events.NewInProcessBus()
	adapter := &mockAdapter{name: "linear"}

	tickets := &mockTicketGetter{tickets: map[string]*ticket.Ticket{
		"proj-5": {
			ID:        "proj-5",
			ProjectID: "project-uuid",
			Title:     "Unmapped state",
			Type:      ticket.TypeTask,
			State:     ticket.StateSpecced,
			Objective: ticket.Objective{Description: "Not mapped"},
			Inputs:    map[string]any{},
			Outputs:   map[string]any{},
		},
	}}

	projects := &mockProjectGetter{projects: map[string]*project.Project{
		"project-uuid": {
			ID:   "project-uuid",
			Slug: "proj",
			ContextPack: project.ContextPack{
				Extra: map[string]string{
					// Only "executing" and "closed" are mapped; "specced" is not.
					"mirror_config": `{"enabled":true,"provider":"linear","state_mapping":{"executing":"In Progress","closed":"Done"},"linear":{"team_id":"team-1","api_key_secret":"linear_key"}}`,
				},
			},
		},
	}}

	svc := NewService(tickets, projects, bus)
	svc.RegisterAdapter("linear", adapter)
	svc.SetExternalID("proj-5", "LIN-555")

	// Publish event for unmapped state.
	_ = bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketSpecced,
		Payload: map[string]any{"ticket_id": "proj-5", "state": "specced"},
	})

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.stateUpdates) != 0 {
		t.Errorf("expected no state updates for unmapped state, got %d", len(adapter.stateUpdates))
	}
}
