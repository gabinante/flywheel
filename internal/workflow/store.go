package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gabinante/flywheel/db"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("workflow definition not found")

// Store persists workflow definitions and phase completions.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a new workflow definition and snapshots the version.
func (s *Store) Create(ctx context.Context, d *Definition) error {
	if !db.InTransaction(ctx) {
		return db.Transaction(ctx, s.pool, func(ctx context.Context) error { return s.Create(ctx, d) })
	}
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.Version == 0 {
		d.Version = 1
	}
	phasesJSON, err := json.Marshal(d.Phases)
	if err != nil {
		return fmt.Errorf("marshal phases: %w", err)
	}
	_, err = db.Executor(ctx, s.pool).Exec(ctx,
		`INSERT INTO workflow_definitions (id, scope, scope_id, name, description, version, phases, is_active, is_library, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		d.ID, d.Scope, d.ScopeID, d.Name, d.Description, d.Version, phasesJSON, d.IsActive, d.IsLibrary, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return err
	}
	return s.snapshotVersion(ctx, d.ID, d.Version, d.Name, d.Description, phasesJSON)
}

// Update replaces a workflow definition, bumping its version and snapshotting it.
func (s *Store) Update(ctx context.Context, d *Definition) error {
	if !db.InTransaction(ctx) {
		return db.Transaction(ctx, s.pool, func(ctx context.Context) error { return s.Update(ctx, d) })
	}
	d.UpdatedAt = time.Now().UTC()
	expectedVersion := d.Version
	d.Version++
	phasesJSON, err := json.Marshal(d.Phases)
	if err != nil {
		return fmt.Errorf("marshal phases: %w", err)
	}
	cmd, err := db.Executor(ctx, s.pool).Exec(ctx,
		`UPDATE workflow_definitions SET name = $1, description = $2, version = $3, phases = $4, is_active = $5, is_library = $6, updated_at = $7
		 WHERE id = $8 AND version = $9`,
		d.Name, d.Description, d.Version, phasesJSON, d.IsActive, d.IsLibrary, d.UpdatedAt, d.ID, expectedVersion)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("workflow update conflict: reload before saving")
	}
	return s.snapshotVersion(ctx, d.ID, d.Version, d.Name, d.Description, phasesJSON)
}

// GetByID returns a workflow definition by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Definition, error) {
	row := db.Executor(ctx, s.pool).QueryRow(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, is_library, created_at, updated_at
		 FROM workflow_definitions WHERE id = $1`, id)
	return s.scanDefinition(row)
}

// GetByScope returns the active workflow for a scope (system/org/project).
func (s *Store) GetByScope(ctx context.Context, scope, scopeID string) (*Definition, error) {
	row := db.Executor(ctx, s.pool).QueryRow(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, is_library, created_at, updated_at
		 FROM workflow_definitions WHERE scope = $1 AND scope_id = $2 AND is_active = true
		 ORDER BY updated_at DESC LIMIT 1`, scope, scopeID)
	d, err := s.scanDefinition(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

// ListByScope returns all workflow definitions for a scope.
func (s *Store) ListByScope(ctx context.Context, scope, scopeID string) ([]*Definition, error) {
	rows, err := db.Executor(ctx, s.pool).Query(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, is_library, created_at, updated_at
		 FROM workflow_definitions WHERE scope = $1 AND scope_id = $2
		 ORDER BY updated_at DESC`, scope, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Definition
	for rows.Next() {
		d, err := s.scanDefinitionFromRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// Delete removes a workflow definition.
func (s *Store) Delete(ctx context.Context, id string) error {
	cmd, err := db.Executor(ctx, s.pool).Exec(ctx, `DELETE FROM workflow_definitions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeactivateScope deactivates all workflow definitions for a scope.
func (s *Store) DeactivateScope(ctx context.Context, scope, scopeID string) error {
	_, err := db.Executor(ctx, s.pool).Exec(ctx,
		`UPDATE workflow_definitions SET is_active = false, updated_at = now() WHERE scope = $1 AND scope_id = $2 AND is_active = true`,
		scope, scopeID)
	return err
}

// RecordCompletion records a phase completion for a ticket.
// Uses the ticket's workflow_phase_entered_at as started_at when available,
// providing accurate phase duration tracking.
func (s *Store) RecordCompletion(ctx context.Context, c *PhaseCompletion) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	metaJSON, err := json.Marshal(c.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = db.Executor(ctx, s.pool).Exec(ctx,
		`INSERT INTO workflow_phase_completions (id, ticket_id, workflow_id, phase_id, started_at, completed_at, outcome, metadata)
		 VALUES ($1, $2, $3, $4,
		   COALESCE((SELECT workflow_phase_entered_at FROM tickets WHERE id = $2), $5),
		   $6, $7, $8)`,
		c.ID, c.TicketID, c.WorkflowID, c.PhaseID, c.StartedAt, c.CompletedAt, c.Outcome, metaJSON)
	return err
}

// ListCompletions returns all phase completions for a ticket.
func (s *Store) ListCompletions(ctx context.Context, ticketID string) ([]PhaseCompletion, error) {
	rows, err := db.Executor(ctx, s.pool).Query(ctx,
		`SELECT id, ticket_id, workflow_id, phase_id, started_at, completed_at, outcome, metadata
		 FROM workflow_phase_completions WHERE ticket_id = $1 ORDER BY completed_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PhaseCompletion
	for rows.Next() {
		var c PhaseCompletion
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.TicketID, &c.WorkflowID, &c.PhaseID, &c.StartedAt, &c.CompletedAt, &c.Outcome, &metaJSON); err != nil {
			return nil, err
		}
		c.Metadata = make(map[string]any)
		_ = json.Unmarshal(metaJSON, &c.Metadata)
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) scanDefinition(row pgx.Row) (*Definition, error) {
	var d Definition
	var phasesJSON []byte
	err := row.Scan(&d.ID, &d.Scope, &d.ScopeID, &d.Name, &d.Description, &d.Version, &phasesJSON, &d.IsActive, &d.IsLibrary, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(phasesJSON, &d.Phases)
	return &d, nil
}

func (s *Store) scanDefinitionFromRows(rows pgx.Rows) (*Definition, error) {
	var d Definition
	var phasesJSON []byte
	err := rows.Scan(&d.ID, &d.Scope, &d.ScopeID, &d.Name, &d.Description, &d.Version, &phasesJSON, &d.IsActive, &d.IsLibrary, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(phasesJSON, &d.Phases)
	return &d, nil
}

// CreateLibraryEntry inserts a new library workflow definition.
// Library entries are org-scoped and always inactive (they don't participate in resolution).
func (s *Store) CreateLibraryEntry(ctx context.Context, d *Definition) error {
	d.Scope = "org"
	d.IsLibrary = true
	d.IsActive = false
	return s.Create(ctx, d)
}

// ListLibrary returns all library entries for an org.
func (s *Store) ListLibrary(ctx context.Context, orgID string) ([]*Definition, error) {
	rows, err := db.Executor(ctx, s.pool).Query(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, is_library, created_at, updated_at
		 FROM workflow_definitions WHERE scope = 'org' AND scope_id = $1 AND is_library = true
		 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Definition
	for rows.Next() {
		d, err := s.scanDefinitionFromRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// DeleteLibraryEntry deletes a library workflow definition.
func (s *Store) DeleteLibraryEntry(ctx context.Context, id string) error {
	cmd, err := db.Executor(ctx, s.pool).Exec(ctx, `DELETE FROM workflow_definitions WHERE id = $1 AND is_library = true`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByIDAndVersion returns a definition with the phases from a specific version.
// Only the identical current version may substitute for a missing legacy snapshot.
func (s *Store) GetByIDAndVersion(ctx context.Context, id string, version int) (*Definition, error) {
	// Get the base definition for scope/active metadata.
	def, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Overlay phases from the version snapshot.
	var phasesJSON []byte
	var name, description string
	err = db.Executor(ctx, s.pool).QueryRow(ctx,
		`SELECT name, description, phases FROM workflow_definition_versions WHERE workflow_id = $1 AND version = $2`,
		id, version).Scan(&name, &description, &phasesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		if def != nil && (version == 0 || def.Version == version) {
			return def, nil
		}
		return nil, fmt.Errorf("workflow %s version %d is unavailable", id, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get version snapshot: %w", err)
	}
	if def == nil {
		return nil, fmt.Errorf("workflow %s not found", id)
	}
	def.Version = version
	def.Name = name
	def.Description = description
	_ = json.Unmarshal(phasesJSON, &def.Phases)
	return def, nil
}

// snapshotVersion inserts an immutable version snapshot.
func (s *Store) snapshotVersion(ctx context.Context, workflowID string, version int, name, description string, phasesJSON []byte) error {
	_, err := db.Executor(ctx, s.pool).Exec(ctx,
		`INSERT INTO workflow_definition_versions (workflow_id, version, name, description, phases)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		workflowID, version, name, description, phasesJSON)
	return err
}

// WithAdvanceTransaction serializes progression per ticket and commits history
// with the new position. The entry timestamp identifies a particular phase run.
func (s *Store) WithAdvanceTransaction(ctx context.Context, ticketID string, fn func(context.Context) error) error {
	return db.Transaction(ctx, s.pool, func(ctx context.Context) error {
		var id string
		if err := db.Executor(ctx, s.pool).QueryRow(ctx, "SELECT id FROM tickets WHERE id=$1 FOR UPDATE", ticketID).Scan(&id); err != nil {
			return err
		}
		return fn(ctx)
	})
}

func (s *Store) ActivateReplacement(ctx context.Context, d *Definition) error {
	return db.Transaction(ctx, s.pool, func(ctx context.Context) error {
		if _, err := db.Executor(ctx, s.pool).Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "workflow:"+d.Scope+":"+d.ScopeID); err != nil {
			return err
		}
		if err := s.DeactivateScope(ctx, d.Scope, d.ScopeID); err != nil {
			return err
		}
		d.IsActive = true
		return s.Create(ctx, d)
	})
}
