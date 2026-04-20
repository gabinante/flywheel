package embedded

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

func setupTestDB(t *testing.T) *testDB {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &testDB{db: db}
}

type testDB struct {
	db interface{ Close() error }
}

func TestOrgStore(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	store := NewOrgStore(db)

	o := &org.Org{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetByID(ctx, o.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Test Org" {
		t.Errorf("expected name 'Test Org', got %q", got.Name)
	}

	got2, err := store.GetBySlug(ctx, "test-org")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got2.ID != o.ID {
		t.Errorf("expected ID %s, got %s", o.ID, got2.ID)
	}
}

func TestTicketStore(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create org and project first (FK constraints).
	orgStore := NewOrgStore(db)
	orgID := uuid.Must(uuid.NewV7()).String()
	_ = orgStore.Create(ctx, &org.Org{ID: orgID, Name: "test", Slug: "test", CreatedAt: time.Now().UTC()})

	projStore := NewProjectStore(db)
	projID := uuid.Must(uuid.NewV7()).String()
	_ = projStore.Create(ctx, &project.Project{
		ID: projID, OrgID: orgID, Name: "testproj", Slug: "testproj",
		Status: "active", CreatedAt: time.Now().UTC(),
	})

	store := NewTicketStore(db)

	// NextSequence.
	seq, err := store.NextSequence(ctx, projID)
	if err != nil {
		t.Fatalf("NextSequence: %v", err)
	}
	// Sequence starts at 1 in the DB, NextSequence increments before returning (matches Postgres).
	if seq != 2 {
		t.Errorf("expected seq 2, got %d", seq)
	}

	// Create ticket.
	now := time.Now().UTC()
	tk := &ticket.Ticket{
		ID: "testproj-1", ProjectID: projID, Title: "Test ticket",
		Type: ticket.TypeTask, Priority: ticket.P2, State: ticket.StateDraft,
		Version: 0, Objective: ticket.Objective{Description: "Do something"},
		Inputs: map[string]any{}, Outputs: map[string]any{},
		CreatedBy: "agent-1", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(ctx, tk); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// GetByID.
	got, err := store.GetByID(ctx, "testproj-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "Test ticket" {
		t.Errorf("expected title 'Test ticket', got %q", got.Title)
	}
	if got.State != ticket.StateDraft {
		t.Errorf("expected state draft, got %s", got.State)
	}

	// UpdateState.
	if err := store.UpdateState(ctx, "testproj-1", 0, ticket.StatePlanning, "agent-1"); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got2, _ := store.GetByID(ctx, "testproj-1")
	if got2.State != ticket.StatePlanning {
		t.Errorf("expected state planning, got %s", got2.State)
	}
	if got2.Version != 1 {
		t.Errorf("expected version 1, got %d", got2.Version)
	}

	// ListByState.
	list, err := store.ListByState(ctx, projID, ticket.StatePlanning)
	if err != nil {
		t.Fatalf("ListByState: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 ticket, got %d", len(list))
	}
}

func TestAgentStore(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	store := NewAgentStore(db)

	a := &agent.Agent{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Name:      "test-agent",
		Type:      agent.TypeClaude,
		APIKey:    "wf_test123",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetByAPIKey(ctx, "wf_test123")
	if err != nil {
		t.Fatalf("GetByAPIKey: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", got.Name)
	}
}

func TestExecutionStepStore(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	store := NewExecutionStepStore(db)

	step := execution.Step{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Type:      execution.StepTypeThought,
		Payload:   map[string]any{"description": "thinking"},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.AppendStep(ctx, "ticket-1", "agent-1", step); err != nil {
		t.Fatalf("AppendStep: %v", err)
	}

	steps, err := store.GetStepsByTicketID(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("GetStepsByTicketID: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Type != execution.StepTypeThought {
		t.Errorf("expected type thought, got %s", steps[0].Type)
	}

	agentID, err := store.GetAgentIDByTicketID(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("GetAgentIDByTicketID: %v", err)
	}
	if agentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", agentID)
	}
}
