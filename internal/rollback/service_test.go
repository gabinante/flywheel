package rollback

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// --- Mock implementations ---

type mockTransitioner struct {
	tickets    map[string]*ticket.Ticket
	transErr   error
	lastTrigger string
}

func (m *mockTransitioner) GetTicket(_ context.Context, id string) (*ticket.Ticket, error) {
	t, ok := m.tickets[id]
	if !ok {
		return nil, fmt.Errorf("ticket not found: %s", id)
	}
	return t, nil
}

func (m *mockTransitioner) TransitionTicket(_ context.Context, id string, trigger string, _ ticket.Actor, _ map[string]any) error {
	m.lastTrigger = trigger
	if m.transErr != nil {
		return m.transErr
	}
	if t, ok := m.tickets[id]; ok {
		t.State = ticket.StateDraft
		t.AssignedTo = ""
	}
	return nil
}

type mockCreator struct {
	created []*ticket.Ticket
	nextID  string
	err     error
}

func (m *mockCreator) CreateTicket(_ context.Context, projectID, title string, typ ticket.TicketType, priority ticket.Priority, createdBy string, _ []string, workStreamID string, objective ticket.Objective, ticketCtx ticket.TicketContext, _ string, _ ...string) (*ticket.Ticket, error) {
	if m.err != nil {
		return nil, m.err
	}
	t := &ticket.Ticket{
		ID:           m.nextID,
		ProjectID:    projectID,
		Title:        title,
		Type:         typ,
		Priority:     priority,
		State:        ticket.StateDraft,
		WorkStreamID: workStreamID,
		CreatedBy:    createdBy,
		Objective:    objective,
		Context:      ticketCtx,
	}
	m.created = append(m.created, t)
	return t, nil
}

type mockWorktree struct {
	removed []string
	paths   map[string]string
}

func (m *mockWorktree) Remove(ticketID string) error {
	m.removed = append(m.removed, ticketID)
	return nil
}

func (m *mockWorktree) Path(ticketID string) string {
	if m.paths == nil {
		return ""
	}
	return m.paths[ticketID]
}

type mockLeaseRemover struct {
	released []string
}

func (m *mockLeaseRemover) ForceReleaseLease(_ context.Context, ticketID string) error {
	m.released = append(m.released, ticketID)
	return nil
}

type mockBus struct {
	published []events.Event
}

func (m *mockBus) Publish(_ context.Context, e events.Event) error {
	m.published = append(m.published, e)
	return nil
}

func (m *mockBus) Subscribe(_ string, _ events.HandlerFn) {}

// --- Tests ---

func TestClassifyStage(t *testing.T) {
	tests := []struct {
		state ticket.State
		want  Stage
		err   bool
	}{
		{ticket.StateExecuting, StageExecution, false},
		{ticket.StateAwaitingValidation, StagePreDeploy, false},
		{ticket.StateValidated, StagePreDeploy, false},
		{ticket.StateClosed, StagePostObserve, false},
		{ticket.StateDraft, "", true},
		{ticket.StatePlanning, "", true},
	}
	for _, tt := range tests {
		got, err := ClassifyStage(tt.state)
		if (err != nil) != tt.err {
			t.Errorf("ClassifyStage(%q) error = %v, wantErr %v", tt.state, err, tt.err)
		}
		if got != tt.want {
			t.Errorf("ClassifyStage(%q) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestRollback_Execution(t *testing.T) {
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-1": {
				ID: "proj-1", ProjectID: "p1", Title: "Test",
				State: ticket.StateExecuting, AssignedTo: "agent1",
			},
		},
	}
	creator := &mockCreator{}
	bus := &mockBus{}
	wt := &mockWorktree{paths: map[string]string{"proj-1": "/tmp/wt/proj-1"}}
	leases := &mockLeaseRemover{}

	svc := NewService(trans, creator, bus)
	svc.SetWorktreeRemover(wt)
	svc.SetLeaseRemover(leases)

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	result, err := svc.Rollback(context.Background(), "proj-1", actor, "code is broken")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if result.Stage != StageExecution {
		t.Errorf("stage = %q, want %q", result.Stage, StageExecution)
	}
	if result.NewState != string(ticket.StateDraft) {
		t.Errorf("new_state = %q, want %q", result.NewState, ticket.StateDraft)
	}
	if !result.ClaimsReleased {
		t.Error("expected claims to be released")
	}
	if !result.WorktreeRemoved {
		t.Error("expected worktree to be removed")
	}
	if trans.lastTrigger != ticket.TriggerRollback {
		t.Errorf("trigger = %q, want %q", trans.lastTrigger, ticket.TriggerRollback)
	}
	if len(wt.removed) != 1 || wt.removed[0] != "proj-1" {
		t.Errorf("worktree not removed correctly: %v", wt.removed)
	}
	if len(leases.released) != 1 || leases.released[0] != "proj-1" {
		t.Errorf("lease not released correctly: %v", leases.released)
	}
}

func TestRollback_PreDeploy_AwaitingValidation(t *testing.T) {
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-2": {
				ID: "proj-2", ProjectID: "p1", Title: "Test",
				State: ticket.StateAwaitingValidation, AssignedTo: "agent1",
			},
		},
	}
	creator := &mockCreator{}
	bus := &mockBus{}
	leases := &mockLeaseRemover{}

	svc := NewService(trans, creator, bus)
	svc.SetLeaseRemover(leases)

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	result, err := svc.Rollback(context.Background(), "proj-2", actor, "failed validation")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if result.Stage != StagePreDeploy {
		t.Errorf("stage = %q, want %q", result.Stage, StagePreDeploy)
	}
	if result.NewState != string(ticket.StateDraft) {
		t.Errorf("new_state = %q, want %q", result.NewState, ticket.StateDraft)
	}
	if !result.ClaimsReleased {
		t.Error("expected claims to be released")
	}

	// Should contain diff-related action.
	found := false
	for _, a := range result.Actions {
		if a == "diff marked for revert" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'diff marked for revert' in actions: %v", result.Actions)
	}
}

func TestRollback_PreDeploy_Validated(t *testing.T) {
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-3": {
				ID: "proj-3", ProjectID: "p1", Title: "Test",
				State: ticket.StateValidated,
			},
		},
	}
	creator := &mockCreator{}
	bus := &mockBus{}

	svc := NewService(trans, creator, bus)

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	result, err := svc.Rollback(context.Background(), "proj-3", actor, "needs rework")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if result.Stage != StagePreDeploy {
		t.Errorf("stage = %q, want %q", result.Stage, StagePreDeploy)
	}
	if result.NewState != string(ticket.StateDraft) {
		t.Errorf("new_state = %q, want %q", result.NewState, ticket.StateDraft)
	}
}

func TestRollback_PostObserve_CreatesRollbackTicket(t *testing.T) {
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-6": {
				ID: "proj-6", ProjectID: "p1", Title: "Original Feature",
				State:        ticket.StateClosed,
				WorkStreamID: "ws-1",
			},
		},
	}
	creator := &mockCreator{nextID: "proj-100"}
	bus := &mockBus{}

	svc := NewService(trans, creator, bus)

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	result, err := svc.Rollback(context.Background(), "proj-6", actor, "feature causes issues")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if result.Stage != StagePostObserve {
		t.Errorf("stage = %q, want %q", result.Stage, StagePostObserve)
	}
	// Original ticket stays closed.
	if result.NewState != string(ticket.StateClosed) {
		t.Errorf("new_state = %q, want %q", result.NewState, ticket.StateClosed)
	}
	if result.RollbackTicketID != "proj-100" {
		t.Errorf("rollback_ticket_id = %q, want %q", result.RollbackTicketID, "proj-100")
	}
	if result.ClaimsReleased {
		t.Error("claims should not be released for closed ticket")
	}

	// Verify rollback ticket was created.
	if len(creator.created) != 1 {
		t.Fatalf("expected 1 ticket created, got %d", len(creator.created))
	}
	rollbackTicket := creator.created[0]
	if rollbackTicket.Priority != ticket.P0 {
		t.Errorf("rollback ticket priority = %d, want %d", rollbackTicket.Priority, ticket.P0)
	}

	// Verify rollback event was emitted.
	foundRollbackEvent := false
	for _, e := range bus.published {
		if e.Type == events.EventTicketRolledBack {
			foundRollbackEvent = true
			if e.Payload["rollback_ticket_id"] != "proj-100" {
				t.Errorf("event payload rollback_ticket_id = %v, want %q", e.Payload["rollback_ticket_id"], "proj-100")
			}
		}
	}
	if !foundRollbackEvent {
		t.Error("expected rolled_back event to be published")
	}
}

func TestRollback_InvalidState(t *testing.T) {
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-7": {
				ID: "proj-7", ProjectID: "p1",
				State: ticket.StateDraft,
			},
		},
	}
	creator := &mockCreator{}
	bus := &mockBus{}

	svc := NewService(trans, creator, bus)

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	_, err := svc.Rollback(context.Background(), "proj-7", actor, "test")
	if err == nil {
		t.Fatal("expected error for rollback from draft state")
	}
}

func TestRollback_NilWorktreeAndLease(t *testing.T) {
	// Rollback should work even without worktree/lease services configured.
	trans := &mockTransitioner{
		tickets: map[string]*ticket.Ticket{
			"proj-8": {
				ID: "proj-8", ProjectID: "p1",
				State: ticket.StateExecuting, AssignedTo: "agent1",
			},
		},
	}
	creator := &mockCreator{}
	bus := &mockBus{}

	svc := NewService(trans, creator, bus)
	// Deliberately NOT setting worktree or lease remover.

	actor := ticket.Actor{ID: "human1", Type: ticket.ActorHuman}
	result, err := svc.Rollback(context.Background(), "proj-8", actor, "test")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if result.NewState != string(ticket.StateDraft) {
		t.Errorf("new_state = %q, want %q", result.NewState, ticket.StateDraft)
	}
	if result.WorktreeRemoved {
		t.Error("worktree should not be removed when no worktree manager configured")
	}
}
