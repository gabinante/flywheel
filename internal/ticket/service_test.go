package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
)

// ── Minimal stubs for the Service dependencies ──────────────────────────

type stubBus struct{}

func (stubBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (stubBus) Subscribe(_ string, _ events.HandlerFn)          {}

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

func (s *inMemoryTicketStore) UpdateTitleAndObjective(_ context.Context, _ string, _ string, _ Objective) error {
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

// ── Tests ───────────────────────────────────────────────────────────────

func newTestService(store TicketStore) *Service {
	proj := &project.Project{ID: "proj-1", Slug: "test"}
	return NewService(store, stubBus{}, &stubProjectGetter{proj: proj})
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
