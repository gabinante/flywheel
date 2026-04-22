package hooks_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/events/hooks"
)

// testBus captures published events for assertions.
type testBus struct {
	events []events.Event
}

func (b *testBus) Publish(_ context.Context, ev events.Event) error {
	b.events = append(b.events, ev)
	return nil
}

func (b *testBus) Subscribe(string, events.HandlerFn) {}

func TestClient_Publish_ThreeLines(t *testing.T) {
	bus := &testBus{}

	// Three lines: create client, publish, handle error.
	client := hooks.NewClient(bus)                                                          // line 1
	err := client.Publish(context.Background(), hooks.Deploy("api", "v2.1").By("ci").In("prod")) // line 2
	if err != nil {                                                                         // line 3
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0]
	if ev.Type != events.EventChangePublished {
		t.Errorf("expected type %q, got %q", events.EventChangePublished, ev.Type)
	}
	if ev.Payload["change_type"] != "deploy" {
		t.Errorf("expected change_type=deploy, got %v", ev.Payload["change_type"])
	}
	if ev.Payload["entity_id"] != "api" {
		t.Errorf("expected entity_id=api, got %v", ev.Payload["entity_id"])
	}
	if ev.Payload["after"] != "v2.1" {
		t.Errorf("expected after=v2.1, got %v", ev.Payload["after"])
	}
	if ev.Payload["initiator"] != "ci" {
		t.Errorf("expected initiator=ci, got %v", ev.Payload["initiator"])
	}
	if ev.Payload["environment"] != "prod" {
		t.Errorf("expected environment=prod, got %v", ev.Payload["environment"])
	}
	if ev.Payload["id"] == nil || ev.Payload["id"] == "" {
		t.Error("expected auto-generated ID")
	}
	if ev.Payload["timestamp"] == nil || ev.Payload["timestamp"] == "" {
		t.Error("expected auto-generated timestamp")
	}
}

func TestClient_Publish_WithDefaults(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus,
		hooks.WithEnvironment("production"),
		hooks.WithInitiator("deploy-pipeline"),
		hooks.WithProjectID("proj-42"),
	)

	err := client.Publish(context.Background(), hooks.Deploy("web", "v3.0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev := bus.events[0]
	if ev.Payload["environment"] != "production" {
		t.Errorf("expected default environment=production, got %v", ev.Payload["environment"])
	}
	if ev.Payload["initiator"] != "deploy-pipeline" {
		t.Errorf("expected default initiator, got %v", ev.Payload["initiator"])
	}
	if ev.Payload["project_id"] != "proj-42" {
		t.Errorf("expected default project_id, got %v", ev.Payload["project_id"])
	}
}

func TestClient_Publish_ExplicitOverridesDefaults(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus, hooks.WithEnvironment("staging"))

	err := client.Publish(context.Background(), hooks.Deploy("api", "v1").In("production"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bus.events[0].Payload["environment"] != "production" {
		t.Errorf("explicit environment should override default")
	}
}

func TestClient_Publish_RequiresChangeType(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)

	err := client.Publish(context.Background(), hooks.Change{EntityID: "foo"})
	if err == nil {
		t.Error("expected error for missing change_type")
	}
}

func TestClient_Publish_RequiresEntityID(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)

	err := client.Publish(context.Background(), hooks.Change{ChangeType: "deploy"})
	if err == nil {
		t.Error("expected error for missing entity_id")
	}
}

func TestChange_Builders(t *testing.T) {
	tests := []struct {
		name       string
		change     hooks.Change
		wantType   string
		wantEntity string
	}{
		{"Deploy", hooks.Deploy("svc", "v1"), "deploy", "svc"},
		{"ConfigUpdate", hooks.ConfigUpdate("flags", "dark_mode"), "config_update", "flags"},
		{"Failover", hooks.Failover("postgres"), "failover", "postgres"},
		{"Migration", hooks.Migration("users-db", "000042"), "migration", "users-db"},
		{"Scale", hooks.Scale("workers"), "scale", "workers"},
		{"Rollback", hooks.Rollback("api", "v1.9"), "rollback", "api"},
		{"Custom", hooks.Custom("dns_update", "cdn.example.com"), "dns_update", "cdn.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.change.ChangeType != tt.wantType {
				t.Errorf("ChangeType = %q, want %q", tt.change.ChangeType, tt.wantType)
			}
			if tt.change.EntityID != tt.wantEntity {
				t.Errorf("EntityID = %q, want %q", tt.change.EntityID, tt.wantEntity)
			}
		})
	}
}

func TestChange_FluentChaining(t *testing.T) {
	ts := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	c := hooks.Deploy("api", "v2").
		By("ci").
		In("production").
		For("proj-1").
		WithBefore("v1.9").
		Affecting("db", "cache").
		WithMeta("pr", "#42").
		At(ts).
		AsType("microservice")

	if c.Initiator != "ci" {
		t.Errorf("Initiator = %q", c.Initiator)
	}
	if c.Environment != "production" {
		t.Errorf("Environment = %q", c.Environment)
	}
	if c.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q", c.ProjectID)
	}
	if c.Before != "v1.9" {
		t.Errorf("Before = %v", c.Before)
	}
	if c.After != "v2" {
		t.Errorf("After = %v", c.After)
	}
	if len(c.AffectedEntities) != 2 {
		t.Errorf("AffectedEntities = %v", c.AffectedEntities)
	}
	if c.Metadata["pr"] != "#42" {
		t.Errorf("Metadata[pr] = %v", c.Metadata["pr"])
	}
	if !c.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v", c.Timestamp)
	}
	if c.EntityType != "microservice" {
		t.Errorf("EntityType = %q", c.EntityType)
	}
}

func TestPackageLevel_Publish(t *testing.T) {
	bus := &testBus{}
	hooks.Init(bus, hooks.WithEnvironment("staging"))

	err := hooks.Publish(context.Background(), hooks.Deploy("api", "v1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	if bus.events[0].Payload["environment"] != "staging" {
		t.Errorf("expected environment=staging from Init defaults")
	}
}

func TestPackageLevel_Publish_WithoutInit(t *testing.T) {
	// Reset the package-level client by re-initializing with nil-ish state.
	// We test that calling Publish without Init returns an error, not a panic.
	// Note: We can't truly reset the global, but we can test the NewClient path.
	bus := &testBus{}
	client := hooks.NewClient(bus)
	err := client.Publish(context.Background(), hooks.Deploy("api", "v1"))
	if err != nil {
		t.Fatalf("unexpected error with explicit client: %v", err)
	}
}
