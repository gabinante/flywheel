package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	_, err = s.pool.Exec(ctx,
		`INSERT INTO workflow_definitions (id, scope, scope_id, name, description, version, phases, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		d.ID, d.Scope, d.ScopeID, d.Name, d.Description, d.Version, phasesJSON, d.IsActive, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return err
	}
	return s.snapshotVersion(ctx, d.ID, d.Version, d.Name, d.Description, phasesJSON)
}

// Update replaces a workflow definition, bumping its version and snapshotting it.
func (s *Store) Update(ctx context.Context, d *Definition) error {
	d.UpdatedAt = time.Now().UTC()
	d.Version++
	phasesJSON, err := json.Marshal(d.Phases)
	if err != nil {
		return fmt.Errorf("marshal phases: %w", err)
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE workflow_definitions SET name = $1, description = $2, version = $3, phases = $4, is_active = $5, updated_at = $6
		 WHERE id = $7`,
		d.Name, d.Description, d.Version, phasesJSON, d.IsActive, d.UpdatedAt, d.ID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return s.snapshotVersion(ctx, d.ID, d.Version, d.Name, d.Description, phasesJSON)
}

// GetByID returns a workflow definition by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Definition, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, created_at, updated_at
		 FROM workflow_definitions WHERE id = $1`, id)
	return s.scanDefinition(row)
}

// GetByScope returns the active workflow for a scope (system/org/project).
func (s *Store) GetByScope(ctx context.Context, scope, scopeID string) (*Definition, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, created_at, updated_at
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
	rows, err := s.pool.Query(ctx,
		`SELECT id, scope, scope_id, name, description, version, phases, is_active, created_at, updated_at
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
	cmd, err := s.pool.Exec(ctx, `DELETE FROM workflow_definitions WHERE id = $1`, id)
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
	_, err := s.pool.Exec(ctx,
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
	_, err = s.pool.Exec(ctx,
		`INSERT INTO workflow_phase_completions (id, ticket_id, workflow_id, phase_id, started_at, completed_at, outcome, metadata)
		 VALUES ($1, $2, $3, $4,
		   COALESCE((SELECT workflow_phase_entered_at FROM tickets WHERE id = $2), $5),
		   $6, $7, $8)`,
		c.ID, c.TicketID, c.WorkflowID, c.PhaseID, c.StartedAt, c.CompletedAt, c.Outcome, metaJSON)
	return err
}

// ListCompletions returns all phase completions for a ticket.
func (s *Store) ListCompletions(ctx context.Context, ticketID string) ([]PhaseCompletion, error) {
	rows, err := s.pool.Query(ctx,
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
	err := row.Scan(&d.ID, &d.Scope, &d.ScopeID, &d.Name, &d.Description, &d.Version, &phasesJSON, &d.IsActive, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(phasesJSON, &d.Phases)
	return &d, nil
}

func (s *Store) scanDefinitionFromRows(rows pgx.Rows) (*Definition, error) {
	var d Definition
	var phasesJSON []byte
	err := rows.Scan(&d.ID, &d.Scope, &d.ScopeID, &d.Name, &d.Description, &d.Version, &phasesJSON, &d.IsActive, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(phasesJSON, &d.Phases)
	return &d, nil
}

// GetByIDAndVersion returns a definition with the phases from a specific version.
// Falls back to the current definition if no version snapshot exists (pre-migration data).
func (s *Store) GetByIDAndVersion(ctx context.Context, id string, version int) (*Definition, error) {
	// Get the base definition for scope/active metadata.
	def, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Overlay phases from the version snapshot.
	var phasesJSON []byte
	var name, description string
	err = s.pool.QueryRow(ctx,
		`SELECT name, description, phases FROM workflow_definition_versions WHERE workflow_id = $1 AND version = $2`,
		id, version).Scan(&name, &description, &phasesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		// No snapshot — return current definition as fallback.
		return def, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get version snapshot: %w", err)
	}
	def.Version = version
	def.Name = name
	def.Description = description
	_ = json.Unmarshal(phasesJSON, &def.Phases)
	return def, nil
}

// snapshotVersion inserts an immutable version snapshot.
func (s *Store) snapshotVersion(ctx context.Context, workflowID string, version int, name, description string, phasesJSON []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_definition_versions (workflow_id, version, name, description, phases)
		 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		workflowID, version, name, description, phasesJSON)
	return err
}
