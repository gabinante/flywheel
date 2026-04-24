package stream

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
)

func TestAppendEntityEvent(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)

	// Track bus events.
	var published []events.Event
	bus.Subscribe(events.EventEntityStreamAppended, func(_ context.Context, e events.Event) {
		published = append(published, e)
	})

	ctx := context.Background()
	event := &EntityEvent{
		ProjectID:     "proj-1",
		ChangeType:    EntityCreated,
		EntityID:      "svc-orders",
		EntityType:    EntityService,
		EntityName:    "OrderService",
		InitiatorID:   "agent-1",
		InitiatorType: InitiatorAgent,
		Environment:   "prod",
		AfterState:    map[string]any{"version": "1.0"},
	}

	if err := svc.AppendEntityEvent(ctx, event); err != nil {
		t.Fatalf("AppendEntityEvent: %v", err)
	}

	// ID and timestamps should be auto-set.
	if event.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if event.CreatedAt.IsZero() {
		t.Error("expected auto-set CreatedAt")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected auto-set Timestamp")
	}

	// Event should be published on bus.
	if len(published) != 1 {
		t.Fatalf("expected 1 bus event, got %d", len(published))
	}
	if published[0].Payload["entity_id"] != "svc-orders" {
		t.Errorf("expected entity_id=svc-orders, got %v", published[0].Payload["entity_id"])
	}

	// Get should return the event.
	got, err := svc.GetEntityEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetEntityEvent: %v", err)
	}
	if got.EntityName != "OrderService" {
		t.Errorf("expected OrderService, got %s", got.EntityName)
	}
}

func TestAppendStateEvent(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)

	var published []events.Event
	bus.Subscribe(events.EventStateStreamAppended, func(_ context.Context, e events.Event) {
		published = append(published, e)
	})

	ctx := context.Background()
	event := &StateEvent{
		ProjectID:     "proj-1",
		ChangeType:    StateObserved,
		EntityID:      "svc-orders",
		Source:        "kubernetes",
		InitiatorType: InitiatorSystem,
		Environment:   "prod",
		AfterState:    map[string]any{"replicas": 3, "healthy": true},
	}

	if err := svc.AppendStateEvent(ctx, event); err != nil {
		t.Fatalf("AppendStateEvent: %v", err)
	}

	if event.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if event.ObservedAt.IsZero() {
		t.Error("expected auto-set ObservedAt")
	}

	if len(published) != 1 {
		t.Fatalf("expected 1 bus event, got %d", len(published))
	}

	got, err := svc.GetStateEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetStateEvent: %v", err)
	}
	if got.Source != "kubernetes" {
		t.Errorf("expected source=kubernetes, got %s", got.Source)
	}
}

func TestAppendChangeEvent(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)

	var published []events.Event
	bus.Subscribe(events.EventChangeStreamAppended, func(_ context.Context, e events.Event) {
		published = append(published, e)
	})

	ctx := context.Background()
	event := &ChangeEvent{
		ProjectID:        "proj-1",
		ChangeType:       "deploy",
		AffectedEntities: []string{"svc-orders", "svc-payments"},
		Source:           "ci/cd",
		InitiatorID:      "user-1",
		InitiatorType:    InitiatorHuman,
		Environment:      "staging",
		BeforeState:      map[string]any{"version": "1.0"},
		AfterState:       map[string]any{"version": "1.1"},
	}

	if err := svc.AppendChangeEvent(ctx, event); err != nil {
		t.Fatalf("AppendChangeEvent: %v", err)
	}

	if event.ID == "" {
		t.Error("expected auto-generated ID")
	}

	if len(published) != 1 {
		t.Fatalf("expected 1 bus event, got %d", len(published))
	}

	got, err := svc.GetChangeEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetChangeEvent: %v", err)
	}
	if got.ChangeType != "deploy" {
		t.Errorf("expected change_type=deploy, got %s", got.ChangeType)
	}
}

func TestListEntityEvents_Filtering(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Append several events.
	events := []*EntityEvent{
		{ProjectID: "proj-1", ChangeType: EntityCreated, EntityID: "svc-1", EntityType: EntityService, EntityName: "Svc1", InitiatorType: InitiatorSystem, Environment: "prod"},
		{ProjectID: "proj-1", ChangeType: EntityRenamed, EntityID: "svc-1", EntityType: EntityService, EntityName: "SvcRenamed", InitiatorType: InitiatorHuman, Environment: "prod"},
		{ProjectID: "proj-1", ChangeType: EntityCreated, EntityID: "svc-2", EntityType: EntityDatastore, EntityName: "DB1", InitiatorType: InitiatorSystem, Environment: "dev"},
		{ProjectID: "proj-2", ChangeType: EntityCreated, EntityID: "svc-3", EntityType: EntityService, EntityName: "Svc3", InitiatorType: InitiatorSystem, Environment: "prod"},
	}
	for _, e := range events {
		if err := svc.AppendEntityEvent(ctx, e); err != nil {
			t.Fatalf("AppendEntityEvent: %v", err)
		}
	}

	// Filter by project_id.
	page, err := svc.ListEntityEvents(ctx, StreamQuery{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListEntityEvents: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("expected 3 events for proj-1, got %d", page.Total)
	}

	// Filter by entity_id.
	page, err = svc.ListEntityEvents(ctx, StreamQuery{ProjectID: "proj-1", EntityID: "svc-1"})
	if err != nil {
		t.Fatalf("ListEntityEvents: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("expected 2 events for svc-1, got %d", page.Total)
	}

	// Filter by change_type.
	page, err = svc.ListEntityEvents(ctx, StreamQuery{ProjectID: "proj-1", ChangeType: "created"})
	if err != nil {
		t.Fatalf("ListEntityEvents: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("expected 2 created events, got %d", page.Total)
	}

	// Filter by environment.
	page, err = svc.ListEntityEvents(ctx, StreamQuery{ProjectID: "proj-1", Environment: "dev"})
	if err != nil {
		t.Fatalf("ListEntityEvents: %v", err)
	}
	if page.Total != 1 {
		t.Errorf("expected 1 dev event, got %d", page.Total)
	}
}

func TestListChangeEvents_ByEntity(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Append change events.
	changes := []*ChangeEvent{
		{ProjectID: "proj-1", ChangeType: "deploy", AffectedEntities: []string{"svc-1", "svc-2"}, InitiatorType: InitiatorSystem, Source: "ci"},
		{ProjectID: "proj-1", ChangeType: "config_update", AffectedEntities: []string{"svc-2"}, InitiatorType: InitiatorHuman, Source: "manual"},
		{ProjectID: "proj-1", ChangeType: "failover", AffectedEntities: []string{"svc-3"}, InitiatorType: InitiatorSystem, Source: "auto"},
	}
	for _, c := range changes {
		if err := svc.AppendChangeEvent(ctx, c); err != nil {
			t.Fatalf("AppendChangeEvent: %v", err)
		}
	}

	// Filter by entity_id (should match affected_entities).
	page, err := svc.ListChangeEvents(ctx, StreamQuery{ProjectID: "proj-1", EntityID: "svc-2"})
	if err != nil {
		t.Fatalf("ListChangeEvents: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("expected 2 events affecting svc-2, got %d", page.Total)
	}
}

func TestSubscribeToTicketEvents(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Set up ticket event bridge.
	svc.SubscribeToTicketEvents(func(ticketID string) string {
		return "proj-1" // simple lookup
	})

	// Simulate a ticket.created event on the bus.
	bus.Publish(ctx, events.Event{
		Type: "ticket.created",
		Payload: map[string]any{
			"ticket_id":  "ticket-42",
			"project_id": "proj-1",
			"state":      "draft",
		},
	})

	// The change stream should now have an event.
	page, err := svc.ListChangeEvents(ctx, StreamQuery{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("ListChangeEvents: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 change event from ticket bridge, got %d", page.Total)
	}
	if page.Events[0].ChangeType != "ticket_created" {
		t.Errorf("expected change_type=ticket_created, got %s", page.Events[0].ChangeType)
	}
	if page.Events[0].Source != "ticket_lifecycle" {
		t.Errorf("expected source=ticket_lifecycle, got %s", page.Events[0].Source)
	}
}

func TestSubscribeToTicketEvents_BridgesPlanAndWorkStreamEvents(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	svc.SubscribeToTicketEvents(func(ticketID string) string {
		if ticketID == "ticket-9" {
			return "proj-9"
		}
		return ""
	})

	bus.Publish(ctx, events.Event{
		Type: events.EventPlanApproved,
		Payload: map[string]any{
			"plan_id":   "plan-1",
			"ticket_id": "ticket-9",
			"backend":   "code",
		},
	})
	bus.Publish(ctx, events.Event{
		Type: events.EventWorkStreamCompleted,
		Payload: map[string]any{
			"project_id":     "proj-9",
			"work_stream_id": "ws-1",
			"ticket_count":   4,
		},
	})

	page, err := svc.ListChangeEvents(ctx, StreamQuery{ProjectID: "proj-9"})
	if err != nil {
		t.Fatalf("ListChangeEvents: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected 2 change events, got %d", page.Total)
	}

	var sawPlan, sawWorkStream bool
	for _, event := range page.Events {
		switch event.ChangeType {
		case "plan_approved":
			sawPlan = true
			if event.Source != "plan_lifecycle" {
				t.Errorf("plan source = %q, want plan_lifecycle", event.Source)
			}
		case "work_stream_completed":
			sawWorkStream = true
			if event.Source != "work_stream" {
				t.Errorf("work stream source = %q, want work_stream", event.Source)
			}
		}
	}
	if !sawPlan {
		t.Error("missing bridged plan_approved event")
	}
	if !sawWorkStream {
		t.Error("missing bridged work_stream_completed event")
	}
}

func TestListWithPagination(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Append 10 events.
	for i := 0; i < 10; i++ {
		event := &ChangeEvent{
			ProjectID:     "proj-1",
			ChangeType:    "deploy",
			InitiatorType: InitiatorSystem,
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := svc.AppendChangeEvent(ctx, event); err != nil {
			t.Fatalf("AppendChangeEvent: %v", err)
		}
	}

	// First page (limit 3, offset 0).
	page, err := svc.ListChangeEvents(ctx, StreamQuery{ProjectID: "proj-1", Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("ListChangeEvents: %v", err)
	}
	if page.Total != 10 {
		t.Errorf("expected total=10, got %d", page.Total)
	}
	if len(page.Events) != 3 {
		t.Errorf("expected 3 events on page, got %d", len(page.Events))
	}

	// Second page (limit 3, offset 3).
	page, err = svc.ListChangeEvents(ctx, StreamQuery{ProjectID: "proj-1", Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("ListChangeEvents: %v", err)
	}
	if len(page.Events) != 3 {
		t.Errorf("expected 3 events on page 2, got %d", len(page.Events))
	}
}

func TestGetNotFound(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	_, err := svc.GetEntityEvent(ctx, "nonexistent")
	if err != ErrEventNotFound {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}

	_, err = svc.GetStateEvent(ctx, "nonexistent")
	if err != ErrEventNotFound {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}

	_, err = svc.GetChangeEvent(ctx, "nonexistent")
	if err != ErrEventNotFound {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}
}
