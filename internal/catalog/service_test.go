package catalog

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// testDB creates an in-memory SQLite database with catalog tables for testing.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE projects (id TEXT PRIMARY KEY, org_id TEXT, name TEXT, slug TEXT, repo_url TEXT, default_branch TEXT, tech_stack TEXT DEFAULT '[]', context_pack TEXT DEFAULT '{}', status TEXT DEFAULT 'active', created_at TEXT DEFAULT (datetime('now')))`,
		`INSERT INTO projects (id, org_id, name, slug) VALUES ('proj-1', 'org-1', 'Test', 'test')`,
		`CREATE TABLE catalog_entities (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id),
			type TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
			labels TEXT NOT NULL DEFAULT '{}', metadata TEXT NOT NULL DEFAULT '{}',
			source TEXT NOT NULL DEFAULT 'declared',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE catalog_edges (
			id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id),
			from_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			to_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			type TEXT NOT NULL, metadata TEXT NOT NULL DEFAULT '{}',
			source TEXT NOT NULL DEFAULT 'declared',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, from_id, to_id, type)
		)`,
		`CREATE TABLE catalog_deployments (
			service_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			environment_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id),
			version TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'declared',
			observed_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (service_id, environment_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %v\n%s", err, s)
		}
	}
	return db
}

func TestCreateAndGetEntity(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	e, err := svc.CreateEntity(ctx, "proj-1", "service", "api-server", "Main API", nil, nil, "declared")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" || e.Name != "api-server" || e.Type != EntityService || e.Source != SourceDeclared {
		t.Fatalf("unexpected entity: %+v", e)
	}

	got, err := svc.GetEntity(ctx, "proj-1", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "api-server" {
		t.Fatalf("expected api-server, got %s", got.Name)
	}
}

func TestListEntitiesByType(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	_, _ = svc.CreateEntity(ctx, "proj-1", "service", "svc-a", "", nil, nil, "declared")
	_, _ = svc.CreateEntity(ctx, "proj-1", "datastore", "redis", "", nil, nil, "observed")
	_, _ = svc.CreateEntity(ctx, "proj-1", "service", "svc-b", "", nil, nil, "declared")

	services, err := svc.ListEntities(ctx, "proj-1", "service", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	all, err := svc.ListEntities(ctx, "proj-1", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entities, got %d", len(all))
	}
}

func TestUpdateEntity(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	e, _ := svc.CreateEntity(ctx, "proj-1", "service", "old-name", "", nil, nil, "declared")
	updated, err := svc.UpdateEntity(ctx, "proj-1", e.ID, "new-name", "new desc", map[string]string{"team": "backend"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "new-name" || updated.Description != "new desc" || updated.Labels["team"] != "backend" {
		t.Fatalf("update failed: %+v", updated)
	}
}

func TestCreateAndListEdges(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	svcA, _ := svc.CreateEntity(ctx, "proj-1", "service", "api", "", nil, nil, "declared")
	db1, _ := svc.CreateEntity(ctx, "proj-1", "datastore", "postgres", "", nil, nil, "declared")

	edge, err := svc.CreateEdge(ctx, "proj-1", svcA.ID, db1.ID, "depends_on", nil, "declared")
	if err != nil {
		t.Fatal(err)
	}
	if edge.Type != EdgeDependsOn || edge.FromID != svcA.ID || edge.ToID != db1.ID {
		t.Fatalf("unexpected edge: %+v", edge)
	}

	// List outgoing edges from svcA.
	outgoing, err := svc.ListEdges(ctx, "proj-1", svcA.ID, "", "outgoing")
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 {
		t.Fatalf("expected 1 outgoing edge, got %d", len(outgoing))
	}

	// List incoming edges to db1.
	incoming, err := svc.ListEdges(ctx, "proj-1", db1.ID, "", "incoming")
	if err != nil {
		t.Fatal(err)
	}
	if len(incoming) != 1 {
		t.Fatalf("expected 1 incoming edge, got %d", len(incoming))
	}
}

func TestInvalidEntityType(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	_, err := svc.CreateEntity(ctx, "proj-1", "invalid_type", "test", "", nil, nil, "declared")
	if err != ErrInvalidType {
		t.Fatalf("expected ErrInvalidType, got %v", err)
	}
}

func TestInvalidEdgeType(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	svcA, _ := svc.CreateEntity(ctx, "proj-1", "service", "api", "", nil, nil, "declared")
	svcB, _ := svc.CreateEntity(ctx, "proj-1", "service", "worker", "", nil, nil, "declared")

	_, err := svc.CreateEdge(ctx, "proj-1", svcA.ID, svcB.ID, "invalid_type", nil, "declared")
	if err != ErrInvalidEdge {
		t.Fatalf("expected ErrInvalidEdge, got %v", err)
	}
}

func TestDeleteEntityCascadesEdges(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	svcA, _ := svc.CreateEntity(ctx, "proj-1", "service", "api", "", nil, nil, "declared")
	svcB, _ := svc.CreateEntity(ctx, "proj-1", "service", "worker", "", nil, nil, "declared")
	_, _ = svc.CreateEdge(ctx, "proj-1", svcA.ID, svcB.ID, "depends_on", nil, "declared")

	// Delete svcA should also remove the edge.
	if err := svc.DeleteEntity(ctx, "proj-1", svcA.ID); err != nil {
		t.Fatal(err)
	}

	edges, _ := svc.ListEdges(ctx, "proj-1", svcB.ID, "", "both")
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges after delete, got %d", len(edges))
	}
}

func TestDeclaredVsObserved(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	declared, _ := svc.CreateEntity(ctx, "proj-1", "service", "api", "", nil, nil, "declared")
	observed, _ := svc.CreateEntity(ctx, "proj-1", "service", "worker", "", nil, nil, "observed")

	if declared.Source != SourceDeclared {
		t.Fatalf("expected declared, got %s", declared.Source)
	}
	if observed.Source != SourceObserved {
		t.Fatalf("expected observed, got %s", observed.Source)
	}
}

func TestDeploymentMatrix(t *testing.T) {
	db := testDB(t)
	svc := NewService(NewSQLiteStore(db))
	ctx := context.Background()

	apiSvc, _ := svc.CreateEntity(ctx, "proj-1", "service", "api", "", nil, nil, "declared")
	prodEnv, _ := svc.CreateEntity(ctx, "proj-1", "environment", "production", "", nil, nil, "declared")

	err := svc.UpsertDeployment(ctx, &DeploymentEntry{
		ServiceID:     apiSvc.ID,
		EnvironmentID: prodEnv.ID,
		Version:       "v1.2.3",
		Source:        SourceObserved,
	})
	if err != nil {
		t.Fatal(err)
	}

	matrix, err := svc.DeploymentMatrix(ctx, "proj-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(matrix))
	}
	if matrix[0].Version != "v1.2.3" || matrix[0].ServiceName != "api" || matrix[0].EnvName != "production" {
		t.Fatalf("unexpected deployment: %+v", matrix[0])
	}
}
