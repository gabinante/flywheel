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
			id             TEXT PRIMARY KEY,
			org_id         TEXT NOT NULL REFERENCES orgs(id),
			name           TEXT NOT NULL,
			slug           TEXT NOT NULL,
			repo_url       TEXT,
			default_branch TEXT NOT NULL DEFAULT 'main',
			tech_stack     TEXT NOT NULL DEFAULT '[]',
			context_pack   TEXT NOT NULL DEFAULT '{}',
			status         TEXT NOT NULL DEFAULT 'active',
			created_at     TEXT NOT NULL DEFAULT (datetime('now')),
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
			assigned_to     TEXT,
			created_by      TEXT NOT NULL,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS idempotency_creates (
			project_id      TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			ticket_id       TEXT NOT NULL,
			PRIMARY KEY (project_id, idempotency_key)
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
		// Indexes for common queries
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_state ON tickets(project_id, state)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_project_priority ON tickets(project_id, priority, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_steps_ticket ON execution_steps(ticket_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_reviews_ticket ON reviews(ticket_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_work_streams_project ON work_streams(project_id)`,
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
	}
	for _, s := range alterStmts {
		// Ignore errors from ALTER — column may already exist.
		_, _ = db.Exec(s)
	}

	return nil
}
