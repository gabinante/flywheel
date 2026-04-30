package ticket

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
)

// ── Minimal stubs for the Service dependencies ──────────────────────────

type stubBus struct{}

func (stubBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (stubBus) Subscribe(_ string, _ events.HandlerFn)          {}

type recordingBus struct {
	published []events.Event
}

func (b *recordingBus) Publish(_ context.Context, event events.Event) error {
	b.published = append(b.published, event)
	return nil
}

func (b *recordingBus) Subscribe(_ string, _ events.HandlerFn) {}

type stubProjectGetter struct {
	proj *project.Project
}

func (s *stubProjectGetter) GetProject(_ context.Context, id string) (*project.Project, error) {
	return s.proj, nil
}

// inMemoryTicketStore is a minimal in-memory TicketStore for unit tests.
type inMemoryTicketStore struct {
	tickets  map[string]*Ticket
	sequence int64
	idemKeys map[string]string // "projectID:key" → ticketID
}

func newInMemoryStore() *inMemoryTicketStore {
	return &inMemoryTicketStore{
		tickets:  make(map[string]*Ticket),
		sequence: 0,
		idemKeys: make(map[string]string),
	}
}

func (s *inMemoryTicketStore) NextSequence(_ context.Context, _ string) (int64, error) {
	s.sequence++
	return s.sequence, nil
}

func (s *inMemoryTicketStore) Create(_ context.Context, t *Ticket) error {
	// Deep-copy to simulate DB roundtrip (no shared pointers).
	cp := *t
	if t.DependsOn != nil {
		cp.DependsOn = make([]string, len(t.DependsOn))
		copy(cp.DependsOn, t.DependsOn)
	}
	s.tickets[t.ID] = &cp
	return nil
}

func (s *inMemoryTicketStore) GetByID(_ context.Context, id string) (*Ticket, error) {
	t, ok := s.tickets[id]
	if !ok {
		return nil, ErrVersionConflict // reuse for "not found"
	}
	return t, nil
}

func (s *inMemoryTicketStore) GetByIDs(_ context.Context, ids []string) ([]*Ticket, error) {
	var out []*Ticket
	for _, id := range ids {
		if t, ok := s.tickets[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *inMemoryTicketStore) GetByProject(_ context.Context, _ string, _ string, _ State) ([]*Ticket, error) {
	return nil, nil
}

func (s *inMemoryTicketStore) ListByState(_ context.Context, _ string, _ State) ([]*Ticket, error) {
	return nil, nil
}

func (s *inMemoryTicketStore) UpdateState(_ context.Context, _ string, _ int, _ State, _ string) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateOutputs(_ context.Context, _ string, _ int, _ map[string]any) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateContext(_ context.Context, _ string, _ TicketContext) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateDependsOn(_ context.Context, id string, deps []string) error {
	if t, ok := s.tickets[id]; ok {
		t.DependsOn = deps
	}
	return nil
}

func (s *inMemoryTicketStore) UpdateWorkStreamID(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateEnvironmentID(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateTargetRepo(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateWorkflowPhase(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *inMemoryTicketStore) UpdateTitleAndObjective(_ context.Context, id string, title string, objective Objective) error {
	if t, ok := s.tickets[id]; ok {
		t.Title = title
		t.Objective = objective
	}
	return nil
}

func (s *inMemoryTicketStore) CountByCreatedBy(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (s *inMemoryTicketStore) CountByCreatedByPerDay(_ context.Context, _ string, _ int) ([]int, error) {
	return nil, nil
}

func (s *inMemoryTicketStore) GetTicketIDByCreateIdempotency(_ context.Context, projectID, key string) (string, error) {
	if id, ok := s.idemKeys[projectID+":"+key]; ok {
		return id, nil
	}
	return "", nil
}

func (s *inMemoryTicketStore) SetCreateIdempotency(_ context.Context, projectID, key, ticketID string) error {
	s.idemKeys[projectID+":"+key] = ticketID
	return nil
}

func (s *inMemoryTicketStore) ListStaleTickets(_ context.Context, _ []State, _ time.Duration) ([]*Ticket, error) {
	return nil, nil
}

func (s *inMemoryTicketStore) PatchOutputs(_ context.Context, id string, patch map[string]any) error {
	t, ok := s.tickets[id]
	if !ok {
		return fmt.Errorf("ticket %q not found", id)
	}
	if t.Outputs == nil {
		t.Outputs = make(map[string]any)
	}
	for k, v := range patch {
		t.Outputs[k] = v
	}
	return nil
}

// ── Tests ───────────────────────────────────────────────────────────────

func newTestService(store TicketStore) *Service {
	proj := &project.Project{ID: "proj-1", Slug: "test"}
	return NewService(store, stubBus{}, &stubProjectGetter{proj: proj})
}

func newTestServiceWithBus(store TicketStore, bus events.Bus) *Service {
	proj := &project.Project{ID: "proj-1", Slug: "test"}
	return NewService(store, bus, &stubProjectGetter{proj: proj})
}

func TestCreateTicket_DependsOnPersisted(t *testing.T) {
	store := newInMemoryStore()
	svc := newTestService(store)
	ctx := context.Background()

	deps := []string{"test-1", "test-2"}
	ticket, err := svc.CreateTicket(ctx, "proj-1", "with deps", TypeSpike, P2, "agent-1", deps, "", Objective{Description: "test"}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}

	// Returned ticket should have DependsOn set.
	if len(ticket.DependsOn) != 2 {
		t.Fatalf("returned ticket DependsOn len = %d, want 2; got %v", len(ticket.DependsOn), ticket.DependsOn)
	}
	if ticket.DependsOn[0] != "test-1" || ticket.DependsOn[1] != "test-2" {
		t.Errorf("returned ticket DependsOn = %v, want [test-1 test-2]", ticket.DependsOn)
	}

	// Read back from store — should also have DependsOn set.
	stored, err := store.GetByID(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if len(stored.DependsOn) != 2 {
		t.Fatalf("stored ticket DependsOn len = %d, want 2; got %v", len(stored.DependsOn), stored.DependsOn)
	}
	if stored.DependsOn[0] != "test-1" || stored.DependsOn[1] != "test-2" {
		t.Errorf("stored ticket DependsOn = %v, want [test-1 test-2]", stored.DependsOn)
	}
}

func TestCreateTicket_DependsOnSingleElement(t *testing.T) {
	store := newInMemoryStore()
	svc := newTestService(store)
	ctx := context.Background()

	deps := []string{"lightning-talks-ai-adjacent-topics-21"}
	ticket, err := svc.CreateTicket(ctx, "proj-1", "MCP demo", TypeSpike, P2, "agent-1", deps, "", Objective{Description: "test"}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}

	if len(ticket.DependsOn) != 1 {
		t.Fatalf("returned ticket DependsOn len = %d, want 1; got %v", len(ticket.DependsOn), ticket.DependsOn)
	}
	if ticket.DependsOn[0] != "lightning-talks-ai-adjacent-topics-21" {
		t.Errorf("returned ticket DependsOn[0] = %q, want %q", ticket.DependsOn[0], "lightning-talks-ai-adjacent-topics-21")
	}
}

func TestCreateTicket_DependsOnNilNormalized(t *testing.T) {
	store := newInMemoryStore()
	svc := newTestService(store)
	ctx := context.Background()

	// Pass nil for dependsOn — should be normalized to empty slice, not nil.
	ticket, err := svc.CreateTicket(ctx, "proj-1", "no deps", TypeSpike, P2, "agent-1", nil, "", Objective{Description: "test"}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}

	if ticket.DependsOn == nil {
		t.Fatal("returned ticket DependsOn is nil, want empty slice")
	}
	if len(ticket.DependsOn) != 0 {
		t.Errorf("returned ticket DependsOn len = %d, want 0", len(ticket.DependsOn))
	}
}

func TestCreateTicket_DependsOnEmpty(t *testing.T) {
	store := newInMemoryStore()
	svc := newTestService(store)
	ctx := context.Background()

	ticket, err := svc.CreateTicket(ctx, "proj-1", "empty deps", TypeSpike, P2, "agent-1", []string{}, "", Objective{Description: "test"}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}

	if ticket.DependsOn == nil {
		t.Fatal("returned ticket DependsOn is nil, want empty slice")
	}
	if len(ticket.DependsOn) != 0 {
		t.Errorf("returned ticket DependsOn len = %d, want 0", len(ticket.DependsOn))
	}
}

func TestCreateTicket_PublishesRichCreatedEvent(t *testing.T) {
	store := newInMemoryStore()
	bus := &recordingBus{}
	svc := newTestServiceWithBus(store, bus)
	ctx := context.Background()

	ticket, err := svc.CreateTicket(ctx, "proj-1", "Wire activity feed", TypeSpike, P2, "agent-7", nil, "", Objective{Description: "test"}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}

	if len(bus.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(bus.published))
	}
	event := bus.published[0]
	if event.Type != events.EventTicketCreated {
		t.Fatalf("event.Type = %q, want %q", event.Type, events.EventTicketCreated)
	}
	if got := event.Payload["ticket_id"]; got != ticket.ID {
		t.Errorf("ticket_id = %v, want %q", got, ticket.ID)
	}
	if got := event.Payload["project_id"]; got != "proj-1" {
		t.Errorf("project_id = %v, want proj-1", got)
	}
	if got := event.Payload["title"]; got != "Wire activity feed" {
		t.Errorf("title = %v, want Wire activity feed", got)
	}
	if got := event.Payload["state"]; got != string(StateDraft) {
		t.Errorf("state = %v, want %q", got, StateDraft)
	}
	if got := event.Payload["created_by"]; got != "agent-7" {
		t.Errorf("created_by = %v, want agent-7", got)
	}
}

func TestPatchTicketMetadata_PublishesUpdatedEvent(t *testing.T) {
	store := newInMemoryStore()
	bus := &recordingBus{}
	svc := newTestServiceWithBus(store, bus)
	ctx := context.Background()

	ticket, err := svc.CreateTicket(ctx, "proj-1", "Old title", TypeSpike, P2, "agent-7", nil, "", Objective{
		Description:     "before",
		SuccessCriteria: []string{"old"},
		AcceptanceTest:  "go test ./...",
	}, TicketContext{}, "")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}
	bus.published = nil

	newTitle := "New title"
	newDesc := "after"
	newCriteria := []string{"new"}
	if err := svc.PatchTicketMetadata(ctx, ticket.ID, &newTitle, &newDesc, &newCriteria, nil); err != nil {
		t.Fatalf("PatchTicketMetadata() error = %v", err)
	}

	if len(bus.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(bus.published))
	}
	event := bus.published[0]
	if event.Type != events.EventTicketUpdated {
		t.Fatalf("event.Type = %q, want %q", event.Type, events.EventTicketUpdated)
	}
	if got := event.Payload["ticket_id"]; got != ticket.ID {
		t.Errorf("ticket_id = %v, want %q", got, ticket.ID)
	}
	if got := event.Payload["title"]; got != newTitle {
		t.Errorf("title = %v, want %q", got, newTitle)
	}
	fields, ok := event.Payload["changed_fields"].([]string)
	if !ok {
		t.Fatalf("changed_fields type = %T, want []string", event.Payload["changed_fields"])
	}
	if len(fields) != 3 {
		t.Fatalf("changed_fields len = %d, want 3; got %v", len(fields), fields)
	}
}
