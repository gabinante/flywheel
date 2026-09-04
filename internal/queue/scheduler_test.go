package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// --- Mock implementations for scheduler tests ---

// mockStaleLister implements StaleTicketLister for tests.
type mockStaleLister struct {
	tickets []*ticket.Ticket
	err     error
}

func (m *mockStaleLister) ListStaleTickets(_ context.Context, states []ticket.State, threshold time.Duration) ([]*ticket.Ticket, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Filter by states (simulates the DB query)
	var result []*ticket.Ticket
	stateSet := make(map[ticket.State]bool)
	for _, s := range states {
		stateSet[s] = true
	}
	for _, t := range m.tickets {
		if stateSet[t.State] {
			result = append(result, t)
		}
	}
	return result, nil
}

// trackingTransitioner records all transitions for assertions.
type trackingTransitioner struct {
	mu          sync.Mutex
	transitions []transitionRecord
}

type transitionRecord struct {
	ID      string
	Trigger string
	Actor   ticket.Actor
}

func (t *trackingTransitioner) TransitionTicket(_ context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.transitions = append(t.transitions, transitionRecord{ID: id, Trigger: trigger, Actor: actor})
	return nil
}

func (t *trackingTransitioner) getTransitions() []transitionRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]transitionRecord, len(t.transitions))
	copy(cp, t.transitions)
	return cp
}

// simpleBus implements events.Bus with no-op for scheduler tests.
type simpleBus struct{}

func (b *simpleBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (b *simpleBus) Subscribe(_ string, _ events.HandlerFn)          {}

// mockFailureSummarizer records AppendFailureSummary calls for assertions.
type mockFailureSummarizer struct {
	mu      sync.Mutex
	entries []failureEntry
}

type failureEntry struct {
	TicketID string
	Reason   string
}

func (m *mockFailureSummarizer) AppendFailureSummary(_ context.Context, ticketID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, failureEntry{TicketID: ticketID, Reason: reason})
	return nil
}

func (m *mockFailureSummarizer) getEntries() []failureEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]failureEntry, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// --- Tests ---

func TestSweepStaleTickets_RecoverZombies(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	// Insert stale tickets: one in executing, one in planning, both with old updated_at
	staleExecuting := &ticket.Ticket{
		ID:        "proj-1",
		ProjectID: "project-id",
		State:     ticket.StateExecuting,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	stalePlanning := &ticket.Ticket{
		ID:        "proj-2",
		ProjectID: "project-id",
		State:     ticket.StatePlanning,
		UpdatedAt: time.Now().UTC().Add(-25 * time.Minute),
	}
	lister := &mockStaleLister{tickets: []*ticket.Ticket{staleExecuting, stalePlanning}}
	s.EnableStalenessSweep(lister, 0) // default: 2x lease TTL = 10 min

	// Run the sweep
	ctx := context.Background()
	s.sweepStaleTickets(ctx)

	// Assert both tickets were transitioned via TriggerLeaseExpired
	transitions := trans.getTransitions()
	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(transitions))
	}
	for i, tr := range transitions {
		if tr.Trigger != ticket.TriggerLeaseExpired {
			t.Errorf("transition[%d]: expected trigger %q, got %q", i, ticket.TriggerLeaseExpired, tr.Trigger)
		}
		if tr.Actor.Type != ticket.ActorSystem {
			t.Errorf("transition[%d]: expected system actor, got %s", i, tr.Actor.Type)
		}
		if tr.Actor.ID != "staleness-sweep" {
			t.Errorf("transition[%d]: expected actor ID 'staleness-sweep', got %q", i, tr.Actor.ID)
		}
	}
	if transitions[0].ID != "proj-1" || transitions[1].ID != "proj-2" {
		t.Errorf("expected transitions for proj-1 and proj-2, got %v", transitions)
	}
}

func TestSweepStaleTickets_Idempotent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	stale := &ticket.Ticket{
		ID:        "proj-1",
		ProjectID: "project-id",
		State:     ticket.StateExecuting,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	lister := &mockStaleLister{tickets: []*ticket.Ticket{stale}}
	s.EnableStalenessSweep(lister, 10*time.Minute)

	ctx := context.Background()

	// First sweep: should recover
	s.sweepStaleTickets(ctx)
	if len(trans.getTransitions()) != 1 {
		t.Fatalf("expected 1 transition after first sweep, got %d", len(trans.getTransitions()))
	}

	// Simulate ticket now in draft (would be removed from stale query in reality).
	// Clearing the lister simulates the ticket no longer matching the query.
	lister.tickets = nil

	// Second sweep: no stale tickets found → no additional transitions
	s.sweepStaleTickets(ctx)
	if len(trans.getTransitions()) != 1 {
		t.Fatalf("expected still 1 transition after second sweep, got %d", len(trans.getTransitions()))
	}
}

func TestSweepStaleTickets_SkipsWhenNotEnabled(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)
	// Note: EnableStalenessSweep NOT called

	ctx := context.Background()
	s.sweepStaleTickets(ctx)

	if len(trans.getTransitions()) != 0 {
		t.Errorf("expected no transitions when sweep not enabled, got %d", len(trans.getTransitions()))
	}
}

func TestSweepStaleTickets_DefaultThreshold(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseTTL := 5 * time.Minute
	leaseStore := NewRedisStore(rdb, leaseTTL)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)
	s.EnableStalenessSweep(&mockStaleLister{}, 0) // 0 → default 2x TTL

	// Verify the threshold was set to 2x lease TTL
	expectedThreshold := 2 * leaseTTL
	if s.stalenessThreshold != expectedThreshold {
		t.Errorf("expected staleness threshold %v, got %v", expectedThreshold, s.stalenessThreshold)
	}
}

func TestSweepStaleTickets_CustomThreshold(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)
	customThreshold := 45 * time.Minute
	s.EnableStalenessSweep(&mockStaleLister{}, customThreshold)

	if s.stalenessThreshold != customThreshold {
		t.Errorf("expected staleness threshold %v, got %v", customThreshold, s.stalenessThreshold)
	}
}

func TestSweepStaleTickets_IgnoresNonStaleStates(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	// Lister has tickets in draft and closed states — these should not be swept
	lister := &mockStaleLister{tickets: []*ticket.Ticket{
		{ID: "proj-draft", State: ticket.StateDraft, UpdatedAt: time.Now().UTC().Add(-1 * time.Hour)},
		{ID: "proj-closed", State: ticket.StateClosed, UpdatedAt: time.Now().UTC().Add(-1 * time.Hour)},
	}}
	s.EnableStalenessSweep(lister, 10*time.Minute)

	ctx := context.Background()
	s.sweepStaleTickets(ctx)

	if len(trans.getTransitions()) != 0 {
		t.Errorf("expected no transitions for non-stale states, got %d", len(trans.getTransitions()))
	}
}

func TestSweepStaleTickets_HandlesTransitionError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	// Use a transitioner that fails on the first ticket but succeeds on the second
	failingTrans := &failOnceTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, failingTrans, mockList, &simpleBus{}, 30*time.Second)

	lister := &mockStaleLister{tickets: []*ticket.Ticket{
		{ID: "proj-fail", State: ticket.StateExecuting, UpdatedAt: time.Now().UTC().Add(-30 * time.Minute)},
		{ID: "proj-ok", State: ticket.StateExecuting, UpdatedAt: time.Now().UTC().Add(-30 * time.Minute)},
	}}
	s.EnableStalenessSweep(lister, 10*time.Minute)

	ctx := context.Background()
	s.sweepStaleTickets(ctx)

	// The second ticket should still be transitioned despite the first failing
	transitions := failingTrans.getSuccessful()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 successful transition, got %d", len(transitions))
	}
	if transitions[0] != "proj-ok" {
		t.Errorf("expected successful transition for proj-ok, got %s", transitions[0])
	}
}

func TestSweepStaleTickets_QueryError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	// Lister that returns an error
	lister := &mockStaleLister{err: fmt.Errorf("db connection lost")}
	s.EnableStalenessSweep(lister, 10*time.Minute)

	ctx := context.Background()
	// Should not panic; just logs the error
	s.sweepStaleTickets(ctx)

	if len(trans.getTransitions()) != 0 {
		t.Errorf("expected no transitions on query error, got %d", len(trans.getTransitions()))
	}
}

// failOnceTransitioner fails on the first call and succeeds on subsequent ones.
type failOnceTransitioner struct {
	mu         sync.Mutex
	callCount  int
	successful []string
}

func (f *failOnceTransitioner) TransitionTicket(_ context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.callCount == 1 {
		return fmt.Errorf("simulated transition error")
	}
	f.successful = append(f.successful, id)
	return nil
}

func (f *failOnceTransitioner) getSuccessful() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.successful))
	copy(cp, f.successful)
	return cp
}

func TestSweepStaleTickets_InjectsFailureContext(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}
	fs := &mockFailureSummarizer{}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	staleTicket := &ticket.Ticket{
		ID:        "proj-zombie",
		ProjectID: "project-id",
		State:     ticket.StateExecuting,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	lister := &mockStaleLister{tickets: []*ticket.Ticket{staleTicket}}
	s.EnableStalenessSweep(lister, 10*time.Minute)
	s.SetFailureSummarizer(fs)

	ctx := context.Background()
	s.sweepStaleTickets(ctx)

	// Verify transition still happened
	transitions := trans.getTransitions()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}

	// Verify failure context was injected
	entries := fs.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 failure summary entry, got %d", len(entries))
	}
	if entries[0].TicketID != "proj-zombie" {
		t.Errorf("expected ticket ID 'proj-zombie', got %q", entries[0].TicketID)
	}
	if entries[0].Reason == "" {
		t.Error("expected non-empty failure reason")
	}
	// Verify reason contains key context
	for _, substr := range []string{"Staleness sweep", "executing", "10m0s", "get_trace"} {
		if !strings.Contains(entries[0].Reason, substr) {
			t.Errorf("expected reason to contain %q, got %q", substr, entries[0].Reason)
		}
	}
}

func TestSweepStaleTickets_NoFailureContextWithoutSummarizer(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	leaseStore := NewRedisStore(rdb, 5*time.Minute)

	trans := &trackingTransitioner{}
	mockList := &mockTicketLister{tickets: map[string]*ticket.Ticket{}}

	s := NewScheduler(leaseStore, trans, mockList, &simpleBus{}, 30*time.Second)

	staleTicket := &ticket.Ticket{
		ID:        "proj-1",
		ProjectID: "project-id",
		State:     ticket.StateExecuting,
		UpdatedAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	lister := &mockStaleLister{tickets: []*ticket.Ticket{staleTicket}}
	s.EnableStalenessSweep(lister, 10*time.Minute)
	// Note: SetFailureSummarizer NOT called

	ctx := context.Background()
	// Should not panic; transition should still happen
	s.sweepStaleTickets(ctx)

	transitions := trans.getTransitions()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
}
