package entity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("entity not found")
	ErrAlreadyRetired = errors.New("entity already retired")
	ErrDuplicateEnv   = errors.New("entity instance for this environment already exists")
)

// Store persists entities, instances, and stream events.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateEntity inserts a new entity. ID must already be set.
func (s *Store) CreateEntity(ctx context.Context, e *Entity) error {
	attrsJSON, _ := json.Marshal(e.Attributes)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO entities (id, type, logical_name, project_id, description, attributes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, string(e.Type), e.LogicalName, e.ProjectID, e.Description, attrsJSON, e.CreatedAt, e.UpdatedAt)
	return err
}

// GetEntityByID returns an entity by ID.
func (s *Store) GetEntityByID(ctx context.Context, id string) (*Entity, error) {
	var e Entity
	var attrsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, type, logical_name, project_id, description, attributes, retired_at, created_at, updated_at
		 FROM entities WHERE id = $1`, id).
		Scan(&e.ID, &e.Type, &e.LogicalName, &e.ProjectID, &e.Description, &attrsJSON, &e.RetiredAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Attributes = make(map[string]any)
	_ = json.Unmarshal(attrsJSON, &e.Attributes)
	return &e, nil
}

// ListEntities returns entities for a project, optionally filtered by type.
// If includeRetired is false, only active entities are returned.
func (s *Store) ListEntities(ctx context.Context, projectID string, entityType Type, includeRetired bool) ([]*Entity, error) {
	q := `SELECT id, type, logical_name, project_id, description, attributes, retired_at, created_at, updated_at
		 FROM entities WHERE project_id = $1`
	args := []any{projectID}
	argNum := 2
	if entityType != "" {
		q += fmt.Sprintf(` AND type = $%d`, argNum)
		args = append(args, string(entityType))
		argNum++
	}
	if !includeRetired {
		q += ` AND retired_at IS NULL`
	}
	q += ` ORDER BY logical_name, created_at`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanEntities(rows)
}

// UpdateLogicalName updates the logical name (rename).
func (s *Store) UpdateLogicalName(ctx context.Context, id, newName string) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE entities SET logical_name = $1, updated_at = now() WHERE id = $2 AND retired_at IS NULL`,
		newName, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAttributes merges new attributes into the entity's attributes JSONB.
func (s *Store) UpdateAttributes(ctx context.Context, id string, attrs map[string]any) error {
	attrsJSON, _ := json.Marshal(attrs)
	cmd, err := s.pool.Exec(ctx,
		`UPDATE entities SET attributes = attributes || $1, updated_at = now() WHERE id = $2 AND retired_at IS NULL`,
		attrsJSON, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RetireEntity soft-deletes an entity by setting retired_at.
func (s *Store) RetireEntity(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE entities SET retired_at = now(), updated_at = now() WHERE id = $1 AND retired_at IS NULL`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateInstance inserts a new entity instance.
func (s *Store) CreateInstance(ctx context.Context, inst *Instance) error {
	attrsJSON, _ := json.Marshal(inst.Attributes)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO entity_instances (id, entity_id, environment, attributes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		inst.ID, inst.EntityID, inst.Environment, attrsJSON, inst.CreatedAt, inst.UpdatedAt)
	if err != nil {
		// Check for unique constraint violation (entity_id, environment)
		if isDuplicateKeyError(err) {
			return ErrDuplicateEnv
		}
		return err
	}
	return nil
}

// GetInstanceByID returns an instance by ID.
func (s *Store) GetInstanceByID(ctx context.Context, id string) (*Instance, error) {
	var inst Instance
	var attrsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, entity_id, environment, attributes, retired_at, created_at, updated_at
		 FROM entity_instances WHERE id = $1`, id).
		Scan(&inst.ID, &inst.EntityID, &inst.Environment, &attrsJSON, &inst.RetiredAt, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	inst.Attributes = make(map[string]any)
	_ = json.Unmarshal(attrsJSON, &inst.Attributes)
	return &inst, nil
}

// ListInstances returns instances for an entity.
func (s *Store) ListInstances(ctx context.Context, entityID string, includeRetired bool) ([]*Instance, error) {
	q := `SELECT id, entity_id, environment, attributes, retired_at, created_at, updated_at
		 FROM entity_instances WHERE entity_id = $1`
	if !includeRetired {
		q += ` AND retired_at IS NULL`
	}
	q += ` ORDER BY environment, created_at`
	rows, err := s.pool.Query(ctx, q, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanInstances(rows)
}

// RetireInstance soft-deletes an instance.
func (s *Store) RetireInstance(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE entity_instances SET retired_at = now(), updated_at = now() WHERE id = $1 AND retired_at IS NULL`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendStreamEvent inserts a new event into the entity stream.
func (s *Store) AppendStreamEvent(ctx context.Context, evt *StreamEvent) error {
	payloadJSON, _ := json.Marshal(evt.Payload)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO entity_stream (id, entity_id, event_type, payload, actor, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		evt.ID, evt.EntityID, evt.EventType, payloadJSON, evt.Actor, evt.CreatedAt)
	return err
}

// GetStream returns stream events for an entity, ordered by creation time.
func (s *Store) GetStream(ctx context.Context, entityID string, limit int) ([]*StreamEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, entity_id, event_type, payload, actor, created_at
		 FROM entity_stream WHERE entity_id = $1 ORDER BY created_at ASC LIMIT $2`,
		entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*StreamEvent
	for rows.Next() {
		var evt StreamEvent
		var payloadJSON []byte
		if err := rows.Scan(&evt.ID, &evt.EntityID, &evt.EventType, &payloadJSON, &evt.Actor, &evt.CreatedAt); err != nil {
			return nil, err
		}
		evt.Payload = make(map[string]any)
		_ = json.Unmarshal(payloadJSON, &evt.Payload)
		events = append(events, &evt)
	}
	return events, rows.Err()
}

// GetStreamSince returns stream events since a given time (for tailing).
func (s *Store) GetStreamSince(ctx context.Context, since time.Time, limit int) ([]*StreamEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, entity_id, event_type, payload, actor, created_at
		 FROM entity_stream WHERE created_at > $1 ORDER BY created_at ASC LIMIT $2`,
		since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*StreamEvent
	for rows.Next() {
		var evt StreamEvent
		var payloadJSON []byte
		if err := rows.Scan(&evt.ID, &evt.EntityID, &evt.EventType, &payloadJSON, &evt.Actor, &evt.CreatedAt); err != nil {
			return nil, err
		}
		evt.Payload = make(map[string]any)
		_ = json.Unmarshal(payloadJSON, &evt.Payload)
		events = append(events, &evt)
	}
	return events, rows.Err()
}

func (s *Store) scanEntities(rows pgx.Rows) ([]*Entity, error) {
	var list []*Entity
	for rows.Next() {
		var e Entity
		var attrsJSON []byte
		if err := rows.Scan(&e.ID, &e.Type, &e.LogicalName, &e.ProjectID, &e.Description, &attrsJSON, &e.RetiredAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Attributes = make(map[string]any)
		_ = json.Unmarshal(attrsJSON, &e.Attributes)
		list = append(list, &e)
	}
	return list, rows.Err()
}

func (s *Store) scanInstances(rows pgx.Rows) ([]*Instance, error) {
	var list []*Instance
	for rows.Next() {
		var inst Instance
		var attrsJSON []byte
		if err := rows.Scan(&inst.ID, &inst.EntityID, &inst.Environment, &attrsJSON, &inst.RetiredAt, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		inst.Attributes = make(map[string]any)
		_ = json.Unmarshal(attrsJSON, &inst.Attributes)
		list = append(list, &inst)
	}
	return list, rows.Err()
}

// isDuplicateKeyError checks for Postgres unique constraint violation (23505).
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return errors.As(err, new(interface{ SQLState() string })) ||
		// pgx wraps these; check string contains the SQLSTATE code
		containsSQLState(err, "23505")
}

func containsSQLState(err error, code string) bool {
	if err == nil {
		return false
	}
	// pgx errors expose Code field via interface
	type pgErr interface {
		SQLState() string
	}
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == code
	}
	return false
}
