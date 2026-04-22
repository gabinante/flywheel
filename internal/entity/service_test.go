package entity

import (
	"context"
	"testing"

	"github.com/gabinante/flywheel/events"
)

// mockBus is a minimal bus that records published events.
type mockBus struct {
	published []events.Event
}

func (b *mockBus) Publish(_ context.Context, e events.Event) error {
	b.published = append(b.published, e)
	return nil
}

func (b *mockBus) Subscribe(_ string, _ events.HandlerFn) {}

func TestCreateEntity_Validation(t *testing.T) {
	bus := &mockBus{}
	svc := NewService(nil, bus) // store is nil, but validation should fail first

	ctx := context.Background()

	// Missing logical_name
	_, err := svc.CreateEntity(ctx, "proj-1", TypeService, "", "desc", nil, "actor")
	if err == nil {
		t.Fatal("expected error for empty logical_name")
	}

	// Invalid type
	_, err = svc.CreateEntity(ctx, "proj-1", Type("invalid"), "name", "desc", nil, "actor")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}

	// Missing project_id
	_, err = svc.CreateEntity(ctx, "", TypeService, "name", "desc", nil, "actor")
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

func TestCreateInstance_Validation(t *testing.T) {
	bus := &mockBus{}
	svc := NewService(nil, bus)

	ctx := context.Background()

	// Missing environment
	_, err := svc.CreateInstance(ctx, "entity-1", "", nil, "actor")
	if err == nil {
		t.Fatal("expected error for empty environment")
	}
}

func TestRenameEntity_Validation(t *testing.T) {
	bus := &mockBus{}
	svc := NewService(nil, bus)

	ctx := context.Background()

	// Empty name
	err := svc.RenameEntity(ctx, "entity-1", "", "actor")
	if err == nil {
		t.Fatal("expected error for empty new name")
	}
}
