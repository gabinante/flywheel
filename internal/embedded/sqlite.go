// Package embedded provides SQLite-backed store implementations for zero-config
// single-operator mode. No Postgres or Redis required — data is persisted to a
// local SQLite database file. Use miniredis (alicebob/miniredis) for the lease
// store alongside these SQLite stores.
package embedded

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// DefaultDataDir returns the default data directory for embedded mode.
// Uses $WARRANT_DATA_DIR if set, otherwise ~/.warrant/data.
func DefaultDataDir() string {
	if d := os.Getenv("WARRANT_DATA_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".warrant", "data")
}

// OpenDB opens (or creates) the SQLite database at the given path and runs
// migrations. If dbPath is empty, DefaultDataDir()/warrant.db is used.
// The returned *sql.DB should be closed by the caller on shutdown.
func OpenDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dir := DefaultDataDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		dbPath = filepath.Join(dir, "warrant.db")
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// migrate creates all tables if they do not exist.
func migrate(db *sql.DB) error {
	stmts := []string{
		// Orgs
		`CREATE TABLE IF NOT EXISTS orgs (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			slug       TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS org_members (
			org_id  TEXT NOT NULL REFERENCES orgs(id),
			user_id TEXT NOT NULL,
			role    TEXT NOT NULL DEFAULT 'member',
			PRIMARY KEY (org_id, user_id)
		)`,
		// Users
		`CREATE TABLE IF NOT EXISTS users (
			id         TEXT PRIMARY KEY,
			github_id  INTEGER UNIQUE,
			login      TEXT NOT NULL,
			name       TEXT NOT NULL DEFAULT '',
			email      TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Agents
		`CREATE TABLE IF NOT EXISTS agents (
			id         TEXT PRIMARY KEY,
			user_id    TEXT,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL DEFAULT 'custom',
			api_key    TEXT UNIQUE,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Projects
		`CREATE TABLE IF NOT EXISTS projects (
			id               TEXT PRIMARY KEY,
			org_id           TEXT NOT NULL REFERENCES orgs(id),
			name             TEXT NOT NULL,
			slug             TEXT NOT NULL,
			repo_url         TEXT,
			default_branch   TEXT NOT NULL DEFAULT 'main',
			tech_stack       TEXT NOT NULL DEFAULT '[]',
			context_pack     TEXT NOT NULL DEFAULT '{}',
			status           TEXT NOT NULL DEFAULT 'active',
			dispatch_enabled INTEGER NOT NULL DEFAULT 1,
			dispatch_config  TEXT NOT NULL DEFAULT '{}',
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (org_id, slug)
		)`,
		`CREATE TABLE IF NOT EXISTS ticket_sequences (
			project_id TEXT PRIMARY KEY REFERENCES projects(id),
			next_val   INTEGER NOT NULL DEFAULT 1
		)`,
		// Work streams
		`CREATE TABLE IF NOT EXISTS work_streams (
			id         TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			name       TEXT NOT NULL,
			slug       TEXT NOT NULL,
			plan       TEXT,
			branch     TEXT,
			status     TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Tickets
		`CREATE TABLE IF NOT EXISTS tickets (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL REFERENCES projects(id),
			title           TEXT NOT NULL,
			type            TEXT NOT NULL DEFAULT 'task',
			priority        INTEGER NOT NULL DEFAULT 3,
			state           TEXT NOT NULL DEFAULT 'draft',
			version         INTEGER NOT NULL DEFAULT 0,
			objective       TEXT NOT NULL DEFAULT '{}',
			ticket_context  TEXT NOT NULL DEFAULT '{}',
			inputs          TEXT NOT NULL DEFAULT '{}',
			outputs         TEXT NOT NULL DEFAULT '{}',
			depends_on      TEXT NOT NULL DEFAULT '[]',
			work_stream_id  TEXT,
			environment_id  TEXT,
			target_repo     TEXT,
			assigned_to     TEXT,
			created_by      TEXT NOT NULL,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Environments (compound tuple: infrastructure × data_tenancy × integration_mode)
		`CREATE TABLE IF NOT EXISTS environments (
			id               TEXT PRIMARY KEY,
			project_id       TEXT NOT NULL REFERENCES projects(id),
			name             TEXT NOT NULL,
			slug             TEXT NOT NULL,
			infrastructure   TEXT NOT NULL,
			data_tenancy     TEXT NOT NULL,
			integration_mode TEXT NOT NULL,
			is_default       INTEGER NOT NULL DEFAULT 0,
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, slug)
		)`,
		`CREATE TABLE IF NOT EXISTS idempotency_creates (
			project_id      TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			ticket_id       TEXT NOT NULL,
			PRIMARY KEY (project_id, idempotency_key)
		)`,
		// Project repositories (multi-repo support)
		`CREATE TABLE IF NOT EXISTS project_repositories (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL REFERENCES projects(id),
			alias           TEXT NOT NULL,
			repo_url        TEXT NOT NULL,
			default_branch  TEXT NOT NULL DEFAULT 'main',
			is_primary      INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, alias)
		)`,
		// Execution steps
		`CREATE TABLE IF NOT EXISTS execution_steps (
			id          TEXT PRIMARY KEY,
			ticket_id   TEXT NOT NULL,
			agent_id    TEXT NOT NULL,
			type        TEXT NOT NULL,
			payload     TEXT NOT NULL DEFAULT '{}',
			worker_type TEXT,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Reviews
		`CREATE TABLE IF NOT EXISTS reviews (
			id          TEXT PRIMARY KEY,
			ticket_id   TEXT NOT NULL,
			reviewer_id TEXT NOT NULL,
			decision    TEXT NOT NULL,
			notes       TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Escalations
		`CREATE TABLE IF NOT EXISTS escalations (
			id          TEXT PRIMARY KEY,
			ticket_id   TEXT NOT NULL,
			agent_id    TEXT NOT NULL,
			reason      TEXT NOT NULL DEFAULT '',
			question    TEXT NOT NULL DEFAULT '',
			answer      TEXT,
			resolved_by TEXT,
			resolved_at TEXT,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS orchestrator_messages (
			id         TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS orchestrator_runs (
			id                   TEXT PRIMARY KEY,
			project_id           TEXT NOT NULL REFERENCES projects(id),
			user_message_id      TEXT NOT NULL REFERENCES orchestrator_messages(id) ON DELETE CASCADE,
			assistant_message_id TEXT REFERENCES orchestrator_messages(id) ON DELETE SET NULL,
			status               TEXT NOT NULL,
			worker_id            TEXT,
			worker_name          TEXT,
			runner               TEXT,
			driver               TEXT,
			model                TEXT,
			error                TEXT,
			started_at           TEXT NOT NULL DEFAULT (datetime('now')),
			completed_at         TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS orchestrator_run_events (
			id         TEXT PRIMARY KEY,
			run_id     TEXT NOT NULL REFERENCES orchestrator_runs(id) ON DELETE CASCADE,
			kind       TEXT NOT NULL,
			payload    TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Indexes for common queries
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_state ON tickets(project_id, state)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_priority ON tickets(project_id, priority, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_steps_ticket ON execution_steps(ticket_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reviews_ticket ON reviews(ticket_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_work_streams_project ON work_streams(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestrator_messages_project ON orchestrator_messages(project_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestrator_runs_project ON orchestrator_runs(project_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_orchestrator_run_events_run ON orchestrator_run_events(run_id, created_at)`,
		// Catalog (Layer 14: Project Map)
		`CREATE TABLE IF NOT EXISTS catalog_entities (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL REFERENCES projects(id),
			type        TEXT NOT NULL,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			labels      TEXT NOT NULL DEFAULT '{}',
			metadata    TEXT NOT NULL DEFAULT '{}',
			source      TEXT NOT NULL DEFAULT 'declared',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_entities_project ON catalog_entities(project_id, type)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_entities_name ON catalog_entities(project_id, name)`,
		`CREATE TABLE IF NOT EXISTS catalog_edges (
			id         TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			from_id    TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			to_id      TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			type       TEXT NOT NULL,
			metadata   TEXT NOT NULL DEFAULT '{}',
			source     TEXT NOT NULL DEFAULT 'declared',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, from_id, to_id, type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_edges_from ON catalog_edges(project_id, from_id)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_edges_to ON catalog_edges(project_id, to_id)`,
		`CREATE TABLE IF NOT EXISTS catalog_deployments (
			service_id     TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			environment_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
			project_id     TEXT NOT NULL REFERENCES projects(id),
			version        TEXT NOT NULL DEFAULT '',
			source         TEXT NOT NULL DEFAULT 'declared',
			observed_at    TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (service_id, environment_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_deployments_project ON catalog_deployments(project_id)`,

		// Pillar entries (Layer 15)
		`CREATE TABLE IF NOT EXISTS pillar_entries (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL REFERENCES projects(id),
			entity_id       TEXT NOT NULL,
			entity_type     TEXT NOT NULL,
			pillar_type     TEXT NOT NULL,
			strategy        TEXT NOT NULL DEFAULT '',
			gaps            TEXT NOT NULL DEFAULT '[]',
			review_cadence  TEXT NOT NULL DEFAULT 'monthly',
			last_reviewed_at TEXT,
			next_review_at  TEXT,
			version         INTEGER NOT NULL DEFAULT 1,
			created_by      TEXT NOT NULL,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, entity_id, pillar_type)
		)`,
		`CREATE TABLE IF NOT EXISTS pillar_claims (
			id               TEXT PRIMARY KEY,
			pillar_entry_id  TEXT NOT NULL REFERENCES pillar_entries(id),
			statement        TEXT NOT NULL,
			entity_ref_id    TEXT NOT NULL,
			entity_ref_type  TEXT NOT NULL,
			evidence         TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'unverified',
			last_evaluated_at TEXT,
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS pillar_evaluations (
			id               TEXT PRIMARY KEY,
			pillar_entry_id  TEXT NOT NULL REFERENCES pillar_entries(id),
			check_type       TEXT NOT NULL,
			outcome          TEXT NOT NULL,
			details          TEXT NOT NULL DEFAULT '',
			evaluated_at     TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pillar_entries_project ON pillar_entries(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pillar_entries_entity ON pillar_entries(entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pillar_claims_entry ON pillar_claims(pillar_entry_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pillar_evaluations_entry ON pillar_evaluations(pillar_entry_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:min(len(s), 60)], err)
		}
	}

	// Additive migrations for existing databases.
	alterStmts := []string{
		// Add worker_type column if not present (added in worker type differentiation).
		`ALTER TABLE execution_steps ADD COLUMN worker_type TEXT`,
		// Add dispatch_config for project-level worker routing.
		`ALTER TABLE projects ADD COLUMN dispatch_config TEXT NOT NULL DEFAULT '{}'`,
	}
	for _, s := range alterStmts {
		// Ignore errors from ALTER — column may already exist.
		_, _ = db.Exec(s)
	}

	return nil
}
