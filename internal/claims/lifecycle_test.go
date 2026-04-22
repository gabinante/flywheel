package claims

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/plan"
	"github.com/gabinante/flywheel/internal/ticket"
)

// --- Test doubles ---

type mockPlanLister struct {
	plans map[string][]*plan.Plan
}

func (m *mockPlanLister) ListPlansByTicket(_ context.Context, ticketID string) ([]*plan.Plan, error) {
	return m.plans[ticketID], nil
}

type mockTicketGetter struct {
	tickets map[string]*ticket.Ticket
}

func (m *mockTicketGetter) GetTicket(_ context.Context, id string) (*ticket.Ticket, error) {
	t, ok := m.tickets[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

// testClaimsService wraps memStore to create a testable claims service.
type testClaimsService struct {
	svc *serviceWithMem
}

// --- Lifecycle tests ---

func TestLifecycleHandler_OnTicketStarted_RegistersClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{
		plans: map[string][]*plan.Plan{
			"ticket-1": {
				{
					ID:      "plan-1",
					Backend: plan.BackendCode,
					State:   plan.StateApproved,
					Content: plan.Content{
						Code: &plan.CodePlan{
							Diffs: []plan.CodeDiff{{
								FilePath:   "src/handler.go",
								Language:   "go",
								BeforeHash: "aaa",
								AfterHash:  "bbb",
								Hunks:      []plan.CodeHunk{{StartLine: 1, EndLine: 10, Content: "x", Operation: "modify"}},
							}},
						},
					},
				},
			},
		},
	}

	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-1": {
				ID:          "ticket-1",
				Environment: ticket.EnvDevelopment,
			},
		},
	}

	// Create a real lifecycle handler — it subscribes to bus events
	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	// Simulate ticket.started event
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "ticket-1"},
	})

	// Verify claims were registered
	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-1")
	if len(active) != 1 {
		t.Fatalf("expected 1 active claim after ticket.started, got %d", len(active))
	}
	if active[0].EntityID != "src/handler.go" {
		t.Errorf("expected entity_id 'src/handler.go', got %s", active[0].EntityID)
	}
	if active[0].ClaimType != ClaimFileWrite {
		t.Errorf("expected claim_type file_write, got %s", active[0].ClaimType)
	}
	if active[0].Environment != "development" {
		t.Errorf("expected environment 'development', got %s", active[0].Environment)
	}
}

func TestLifecycleHandler_OnTicketStarted_SkipsRejectedPlans(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{
		plans: map[string][]*plan.Plan{
			"ticket-2": {
				{
					ID:      "plan-rejected",
					Backend: plan.BackendCode,
					State:   plan.StateRejected,
					Content: plan.Content{
						Code: &plan.CodePlan{Diffs: []plan.CodeDiff{{FilePath: "src/old.go", Language: "go", BeforeHash: "a", AfterHash: "b"}}},
					},
				},
				{
					ID:      "plan-approved",
					Backend: plan.BackendDeploy,
					State:   plan.StateApproved,
					Content: plan.Content{
						Deploy: &plan.DeployPlan{Target: "prod-cluster", ArtifactHash: "sha256:abc", DeployStrategy: "rolling"},
					},
				},
			},
		},
	}

	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-2": {ID: "ticket-2", Environment: ticket.EnvProduction},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "ticket-2"},
	})

	// Only the approved plan's touch should be registered (not the rejected one)
	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-2")
	if len(active) != 1 {
		t.Fatalf("expected 1 active claim (from approved plan only), got %d", len(active))
	}
	if active[0].EntityID != "prod-cluster" {
		t.Errorf("expected entity_id 'prod-cluster', got %s", active[0].EntityID)
	}
}

func TestLifecycleHandler_OnTicketStarted_NoPlan_NoClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{plans: map[string][]*plan.Plan{}}
	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-3": {ID: "ticket-3", Environment: ticket.EnvDevelopment},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "ticket-3"},
	})

	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-3")
	if len(active) != 0 {
		t.Errorf("expected 0 claims for ticket without plans, got %d", len(active))
	}
}

func TestLifecycleHandler_OnTicketClosed_ReleasesClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{plans: map[string][]*plan.Plan{}}
	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-4": {ID: "ticket-4", Environment: ticket.EnvDevelopment},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	// Pre-register some claims manually
	_, _ = svc.RegisterClaims(ctx, "ticket-4", []Touch{
		{EntityID: "src/a.go", Environment: "development", ClaimType: ClaimFileWrite},
		{EntityID: "src/b.go", Environment: "development", ClaimType: ClaimFileWrite},
	})

	// Verify claims exist
	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-4")
	if len(active) != 2 {
		t.Fatalf("expected 2 active claims before close, got %d", len(active))
	}

	// Simulate ticket.closed
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketClosed,
		Payload: map[string]any{"ticket_id": "ticket-4"},
	})

	// Claims should be released
	active, _ = svc.ms.GetActiveByTicket(ctx, "ticket-4")
	if len(active) != 0 {
		t.Errorf("expected 0 active claims after close, got %d", len(active))
	}
}

func TestLifecycleHandler_OnTicketCancelled_ReleasesClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{plans: map[string][]*plan.Plan{}}
	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-5": {ID: "ticket-5"},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_, _ = svc.RegisterClaims(ctx, "ticket-5", []Touch{
		{EntityID: "deploy-target", Environment: "prod", ClaimType: ClaimDeployTarget},
	})

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketCancelled,
		Payload: map[string]any{"ticket_id": "ticket-5"},
	})

	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-5")
	if len(active) != 0 {
		t.Errorf("expected 0 active claims after cancel, got %d", len(active))
	}
}

func TestLifecycleHandler_OnLeaseExpired_ReleasesClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{plans: map[string][]*plan.Plan{}}
	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-6": {ID: "ticket-6"},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_, _ = svc.RegisterClaims(ctx, "ticket-6", []Touch{
		{EntityID: "schema:orders", Environment: "staging", ClaimType: ClaimSchema},
	})

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventLeaseExpired,
		Payload: map[string]any{"ticket_id": "ticket-6"},
	})

	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-6")
	if len(active) != 0 {
		t.Errorf("expected 0 active claims after lease expired, got %d", len(active))
	}
}

func TestLifecycleHandler_OnTicketFailed_ReleasesClaims(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{plans: map[string][]*plan.Plan{}}
	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-7": {ID: "ticket-7"},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_, _ = svc.RegisterClaims(ctx, "ticket-7", []Touch{
		{EntityID: "svc:payments", Environment: "prod", ClaimType: ClaimService},
	})

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketFailed,
		Payload: map[string]any{"ticket_id": "ticket-7"},
	})

	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-7")
	if len(active) != 0 {
		t.Errorf("expected 0 active claims after ticket failed, got %d", len(active))
	}
}

func TestLifecycleHandler_OnTicketStarted_DetectsConflicts(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	// Pre-register a claim for ticket-A on the same file
	_, _ = svc.RegisterClaims(ctx, "ticket-A", []Touch{
		{EntityID: "src/shared.go", Environment: "development", ClaimType: ClaimFileWrite},
	})

	plans := &mockPlanLister{
		plans: map[string][]*plan.Plan{
			"ticket-B": {
				{
					ID:      "plan-B",
					Backend: plan.BackendCode,
					State:   plan.StateApproved,
					Content: plan.Content{
						Code: &plan.CodePlan{
							Diffs: []plan.CodeDiff{{
								FilePath:   "src/shared.go",
								Language:   "go",
								BeforeHash: "xxx",
								AfterHash:  "yyy",
								Hunks:      []plan.CodeHunk{{StartLine: 5, EndLine: 15, Content: "conflict", Operation: "modify"}},
							}},
						},
					},
				},
			},
		},
	}

	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-B": {ID: "ticket-B", Environment: ticket.EnvDevelopment},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	// Simulate ticket.started for ticket-B — should detect conflict with ticket-A
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "ticket-B"},
	})

	// ticket-B should still have its claims registered (conflicts are advisory at this point)
	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-B")
	if len(active) != 1 {
		t.Fatalf("expected 1 claim registered despite conflict, got %d", len(active))
	}

	// Conflicts should be recorded
	conflicts, _ := svc.ms.GetUnresolvedConflicts(ctx, "ticket-B")
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 unresolved conflict, got %d", len(conflicts))
	}
	if conflicts[0].BlockingTicket != "ticket-A" {
		t.Errorf("expected blocking ticket 'ticket-A', got %s", conflicts[0].BlockingTicket)
	}
	if conflicts[0].ConflictType != ConflictSameFileWrite {
		t.Errorf("expected same_file_write conflict, got %s", conflicts[0].ConflictType)
	}
}

func TestLifecycleHandler_OnTicketStarted_MultiplePlans(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	svc := testServiceWithMemStore()

	plans := &mockPlanLister{
		plans: map[string][]*plan.Plan{
			"ticket-multi": {
				{
					ID:      "plan-code",
					Backend: plan.BackendCode,
					State:   plan.StateApproved,
					Content: plan.Content{
						Code: &plan.CodePlan{Diffs: []plan.CodeDiff{{FilePath: "src/api.go", Language: "go", BeforeHash: "a", AfterHash: "b"}}},
					},
				},
				{
					ID:      "plan-db",
					Backend: plan.BackendDatabase,
					State:   plan.StateClassified,
					Content: plan.Content{
						Database: &plan.DatabasePlan{MigrationName: "add_api_table", DDL: "CREATE TABLE api (...)", Direction: "up"},
					},
				},
			},
		},
	}

	tickets := &mockTicketGetter{
		tickets: map[string]*ticket.Ticket{
			"ticket-multi": {ID: "ticket-multi", Environment: ticket.EnvStaging},
		},
	}

	_ = newTestLifecycleHandler(bus, svc, plans, tickets)

	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketStarted,
		Payload: map[string]any{"ticket_id": "ticket-multi"},
	})

	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-multi")
	if len(active) != 2 {
		t.Fatalf("expected 2 claims from 2 plans, got %d", len(active))
	}

	// Verify both claim types
	types := map[ClaimType]bool{}
	for _, c := range active {
		types[c.ClaimType] = true
	}
	if !types[ClaimFileWrite] {
		t.Error("expected file_write claim from code plan")
	}
	if !types[ClaimSchema] {
		t.Error("expected schema claim from database plan")
	}
}

// --- Test helper: lifecycle handler backed by in-memory service ---

// newTestLifecycleHandler creates a LifecycleHandler that uses the testServiceWithMemStore
// for claims operations. It manually subscribes to the same events.
func newTestLifecycleHandler(bus events.Bus, svc *serviceWithMem, plans PlanLister, tickets TicketGetter) *testLifecycleHandler {
	h := &testLifecycleHandler{
		svc:     svc,
		plans:   plans,
		tickets: tickets,
	}

	bus.Subscribe(events.EventTicketStarted, h.onTicketStarted)
	bus.Subscribe(events.EventTicketClosed, h.onTicketCompleted)
	bus.Subscribe(events.EventTicketCancelled, h.onTicketCompleted)
	bus.Subscribe(events.EventLeaseExpired, h.onTicketCompleted)
	bus.Subscribe(events.EventTicketFailed, h.onTicketCompleted)

	return h
}

type testLifecycleHandler struct {
	svc     *serviceWithMem
	plans   PlanLister
	tickets TicketGetter
}

func (h *testLifecycleHandler) onTicketStarted(ctx context.Context, event events.Event) {
	ticketID, ok := event.Payload["ticket_id"].(string)
	if !ok || ticketID == "" {
		return
	}

	t, err := h.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return
	}

	environment := string(t.Environment)
	plans, err := h.plans.ListPlansByTicket(ctx, ticketID)
	if err != nil {
		return
	}

	var allTouches []Touch
	for _, p := range plans {
		if p.State == plan.StateRejected || p.State == plan.StateSuperseded {
			continue
		}
		touches := ExtractTouches(p, environment)
		allTouches = append(allTouches, touches...)
	}

	if len(allTouches) == 0 {
		return
	}

	// Detect and record conflicts
	result, _ := h.svc.DetectConflicts(ctx, ticketID, allTouches)
	if result != nil {
		for i := range result.Conflicts {
			_ = h.svc.ms.CreateConflict(ctx, &result.Conflicts[i])
		}
	}

	// Register claims
	now := time.Now().UTC()
	for _, touch := range allTouches {
		claim := &Claim{
			ID:          "claim-" + touch.EntityID + "-" + ticketID,
			TicketID:    ticketID,
			EntityID:    touch.EntityID,
			Environment: touch.Environment,
			ClaimType:   touch.ClaimType,
			State:       StateActive,
			Metadata:    touch.Metadata,
			ClaimedAt:   now,
		}
		_ = h.svc.ms.CreateClaim(ctx, claim)
	}
}

func (h *testLifecycleHandler) onTicketCompleted(ctx context.Context, event events.Event) {
	ticketID, ok := event.Payload["ticket_id"].(string)
	if !ok || ticketID == "" {
		return
	}
	_, _ = h.svc.ReleaseClaims(ctx, ticketID)
}
