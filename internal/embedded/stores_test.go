package embedded

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/environment"
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

func TestTicketStore_DependsOnPersisted(t *testing.T) {
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

	now := time.Now().UTC()

	// Subtest: Create ticket WITH depends_on and verify roundtrip.
	t.Run("non-empty depends_on persisted", func(t *testing.T) {
		tk := &ticket.Ticket{
			ID: "testproj-10", ProjectID: projID, Title: "With deps",
			Type: ticket.TypeTask, Priority: ticket.P2, State: ticket.StateDraft,
			Version: 0, Objective: ticket.Objective{Description: "test"},
			Inputs: map[string]any{}, Outputs: map[string]any{},
			DependsOn: []string{"lightning-talks-ai-adjacent-topics-21"},
			CreatedBy: "agent-1", CreatedAt: now, UpdatedAt: now,
		}
		if err := store.Create(ctx, tk); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.GetByID(ctx, "testproj-10")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.DependsOn) != 1 {
			t.Fatalf("DependsOn len = %d, want 1; got %v", len(got.DependsOn), got.DependsOn)
		}
		if got.DependsOn[0] != "lightning-talks-ai-adjacent-topics-21" {
			t.Errorf("DependsOn[0] = %q, want %q", got.DependsOn[0], "lightning-talks-ai-adjacent-topics-21")
		}
	})

	// Subtest: Multiple depends_on entries.
	t.Run("multiple depends_on persisted", func(t *testing.T) {
		tk := &ticket.Ticket{
			ID: "testproj-11", ProjectID: projID, Title: "Multi deps",
			Type: ticket.TypeTask, Priority: ticket.P2, State: ticket.StateDraft,
			Version: 0, Objective: ticket.Objective{Description: "test"},
			Inputs: map[string]any{}, Outputs: map[string]any{},
			DependsOn: []string{"dep-a", "dep-b", "dep-c"},
			CreatedBy: "agent-1", CreatedAt: now, UpdatedAt: now,
		}
		if err := store.Create(ctx, tk); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.GetByID(ctx, "testproj-11")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.DependsOn) != 3 {
			t.Fatalf("DependsOn len = %d, want 3; got %v", len(got.DependsOn), got.DependsOn)
		}
		expected := []string{"dep-a", "dep-b", "dep-c"}
		for i, exp := range expected {
			if got.DependsOn[i] != exp {
				t.Errorf("DependsOn[%d] = %q, want %q", i, got.DependsOn[i], exp)
			}
		}
	})

	// Subtest: Empty depends_on returns empty slice (not nil).
	t.Run("empty depends_on returns empty slice", func(t *testing.T) {
		tk := &ticket.Ticket{
			ID: "testproj-12", ProjectID: projID, Title: "No deps",
			Type: ticket.TypeTask, Priority: ticket.P2, State: ticket.StateDraft,
			Version: 0, Objective: ticket.Objective{Description: "test"},
			Inputs: map[string]any{}, Outputs: map[string]any{},
			DependsOn: []string{},
			CreatedBy: "agent-1", CreatedAt: now, UpdatedAt: now,
		}
		if err := store.Create(ctx, tk); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.GetByID(ctx, "testproj-12")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.DependsOn == nil {
			t.Fatal("DependsOn is nil, want empty slice")
		}
		if len(got.DependsOn) != 0 {
			t.Errorf("DependsOn len = %d, want 0", len(got.DependsOn))
		}
	})

	// Subtest: nil depends_on normalized to empty slice.
	t.Run("nil depends_on normalized to empty slice", func(t *testing.T) {
		tk := &ticket.Ticket{
			ID: "testproj-13", ProjectID: projID, Title: "Nil deps",
			Type: ticket.TypeTask, Priority: ticket.P2, State: ticket.StateDraft,
			Version: 0, Objective: ticket.Objective{Description: "test"},
			Inputs: map[string]any{}, Outputs: map[string]any{},
			DependsOn: nil,
			CreatedBy: "agent-1", CreatedAt: now, UpdatedAt: now,
		}
		if err := store.Create(ctx, tk); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.GetByID(ctx, "testproj-13")
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.DependsOn == nil {
			t.Fatal("DependsOn is nil, want empty slice")
		}
		if len(got.DependsOn) != 0 {
			t.Errorf("DependsOn len = %d, want 0", len(got.DependsOn))
		}
	})

	// Subtest: depends_on persisted through GetByIDs (batch read).
	t.Run("depends_on via GetByIDs", func(t *testing.T) {
		list, err := store.GetByIDs(ctx, []string{"testproj-10", "testproj-11"})
		if err != nil {
			t.Fatalf("GetByIDs: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("expected 2 tickets, got %d", len(list))
		}
		for _, tk := range list {
			if tk.ID == "testproj-10" && len(tk.DependsOn) != 1 {
				t.Errorf("testproj-10 DependsOn len = %d, want 1", len(tk.DependsOn))
			}
			if tk.ID == "testproj-11" && len(tk.DependsOn) != 3 {
				t.Errorf("testproj-11 DependsOn len = %d, want 3", len(tk.DependsOn))
			}
		}
	})

	// Subtest: depends_on persisted through GetByProject (list).
	t.Run("depends_on via GetByProject", func(t *testing.T) {
		list, err := store.GetByProject(ctx, projID, "", "")
		if err != nil {
			t.Fatalf("GetByProject: %v", err)
		}
		found := false
		for _, tk := range list {
			if tk.ID == "testproj-10" {
				found = true
				if len(tk.DependsOn) != 1 {
					t.Errorf("testproj-10 DependsOn len = %d, want 1", len(tk.DependsOn))
				}
			}
		}
		if !found {
			t.Error("testproj-10 not found in GetByProject results")
		}
	})

	// Subtest: UpdateDependsOn works.
	t.Run("UpdateDependsOn", func(t *testing.T) {
		if err := store.UpdateDependsOn(ctx, "testproj-12", []string{"new-dep-1", "new-dep-2"}); err != nil {
			t.Fatalf("UpdateDependsOn: %v", err)
		}
		got, err := store.GetByID(ctx, "testproj-12")
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if len(got.DependsOn) != 2 {
			t.Fatalf("DependsOn len = %d, want 2", len(got.DependsOn))
		}
		if got.DependsOn[0] != "new-dep-1" || got.DependsOn[1] != "new-dep-2" {
			t.Errorf("DependsOn = %v, want [new-dep-1 new-dep-2]", got.DependsOn)
		}
	})
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

func TestEnvironmentStore(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create org and project first.
	orgStore := NewOrgStore(db)
	orgID := uuid.Must(uuid.NewV7()).String()
	_ = orgStore.Create(ctx, &org.Org{ID: orgID, Name: "test", Slug: "test", CreatedAt: time.Now().UTC()})

	projStore := NewProjectStore(db)
	projID := uuid.Must(uuid.NewV7()).String()
	_ = projStore.Create(ctx, &project.Project{
		ID: projID, OrgID: orgID, Name: "testproj", Slug: "testproj",
		Status: "active", CreatedAt: time.Now().UTC(),
	})

	store := NewEnvironmentStore(db)

	// Create dev environment.
	devEnv := &environment.Environment{
		ID:              uuid.Must(uuid.NewV7()).String(),
		ProjectID:       projID,
		Name:            "Development",
		Slug:            "dev",
		Infrastructure:  environment.InfraDev,
		DataTenancy:     environment.DataSynthetic,
		IntegrationMode: environment.IntegrationSandbox,
		IsDefault:       true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := store.Create(ctx, devEnv); err != nil {
		t.Fatalf("Create dev: %v", err)
	}

	// Create staging environment.
	stagingEnv := &environment.Environment{
		ID:              uuid.Must(uuid.NewV7()).String(),
		ProjectID:       projID,
		Name:            "Staging",
		Slug:            "staging",
		Infrastructure:  environment.InfraStaging,
		DataTenancy:     environment.DataAnonymized,
		IntegrationMode: environment.IntegrationTest,
		IsDefault:       false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := store.Create(ctx, stagingEnv); err != nil {
		t.Fatalf("Create staging: %v", err)
	}

	// GetByID.
	got, err := store.GetByID(ctx, devEnv.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Development" {
		t.Errorf("expected name 'Development', got %q", got.Name)
	}
	if got.Infrastructure != environment.InfraDev {
		t.Errorf("expected infra dev, got %s", got.Infrastructure)
	}
	if got.DataTenancy != environment.DataSynthetic {
		t.Errorf("expected data synthetic, got %s", got.DataTenancy)
	}
	if got.IntegrationMode != environment.IntegrationSandbox {
		t.Errorf("expected mode sandbox, got %s", got.IntegrationMode)
	}
	if !got.IsDefault {
		t.Error("expected is_default true")
	}

	// GetBySlug.
	got2, err := store.GetBySlug(ctx, projID, "staging")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got2.ID != stagingEnv.ID {
		t.Errorf("expected ID %s, got %s", stagingEnv.ID, got2.ID)
	}

	// GetBySlug not found.
	_, err = store.GetBySlug(ctx, projID, "nonexistent")
	if err != environment.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// ListByProject.
	list, err := store.ListByProject(ctx, projID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(list))
	}
	// Default should come first.
	if list[0].Slug != "dev" {
		t.Errorf("expected dev first (default), got %s", list[0].Slug)
	}

	// CountByProject.
	count, err := store.CountByProject(ctx, projID)
	if err != nil {
		t.Fatalf("CountByProject: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Update.
	got.Name = "Dev Updated"
	got.Infrastructure = environment.InfraProd
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := store.GetByID(ctx, devEnv.ID)
	if updated.Name != "Dev Updated" {
		t.Errorf("expected updated name, got %q", updated.Name)
	}
	if updated.Infrastructure != environment.InfraProd {
		t.Errorf("expected infra prod after update, got %s", updated.Infrastructure)
	}

	// ClearDefault.
	if err := store.ClearDefault(ctx, projID); err != nil {
		t.Fatalf("ClearDefault: %v", err)
	}
	cleared, _ := store.GetByID(ctx, devEnv.ID)
	if cleared.IsDefault {
		t.Error("expected is_default false after ClearDefault")
	}

	// Create a third environment then delete it.
	prodEnv := &environment.Environment{
		ID:              uuid.Must(uuid.NewV7()).String(),
		ProjectID:       projID,
		Name:            "Production",
		Slug:            "prod",
		Infrastructure:  environment.InfraProd,
		DataTenancy:     environment.DataReal,
		IntegrationMode: environment.IntegrationLive,
		IsDefault:       false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := store.Create(ctx, prodEnv); err != nil {
		t.Fatalf("Create prod: %v", err)
	}

	if err := store.Delete(ctx, prodEnv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = store.GetByID(ctx, prodEnv.ID)
	if err != environment.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete not found.
	err = store.Delete(ctx, "nonexistent")
	if err != environment.ErrNotFound {
		t.Errorf("expected ErrNotFound on bad delete, got %v", err)
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
