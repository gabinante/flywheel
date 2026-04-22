package stream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ Store = (*PostgresStore)(nil)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a new Postgres-backed stream store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// --- Entity stream ---

func (s *PostgresStore) AppendEntity(ctx context.Context, event *EntityEvent) error {
	beforeJSON, _ := json.Marshal(event.BeforeState)
	afterJSON, _ := json.Marshal(event.AfterState)
	metaJSON, _ := json.Marshal(event.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO entity_stream
		 (id, project_id, change_type, affected_entities, before_state, after_state,
		  initiator_id, initiator_type, environment, metadata, timestamp,
		  entity_id, entity_type, entity_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		event.ID, event.ProjectID, string(event.ChangeType), event.AffectedEntities,
		beforeJSON, afterJSON, event.InitiatorID, string(event.InitiatorType),
		event.Environment, metaJSON, event.Timestamp,
		event.EntityID, string(event.EntityType), event.EntityName)
	return err
}

func (s *PostgresStore) GetEntity(ctx context.Context, id string) (*EntityEvent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        entity_id, entity_type, entity_name
		 FROM entity_stream WHERE id = $1`, id)
	return scanEntityEvent(row)
}

func (s *PostgresStore) ListEntity(ctx context.Context, q StreamQuery) (*StreamPage[EntityEvent], error) {
	// Build dynamic query
	where := "WHERE project_id = $1"
	args := []any{q.ProjectID}
	argNum := 2

	if q.EntityID != "" {
		where += fmt.Sprintf(" AND entity_id = $%d", argNum)
		args = append(args, q.EntityID)
		argNum++
	}
	if q.ChangeType != "" {
		where += fmt.Sprintf(" AND change_type = $%d", argNum)
		args = append(args, q.ChangeType)
		argNum++
	}
	if q.Environment != "" {
		where += fmt.Sprintf(" AND environment = $%d", argNum)
		args = append(args, q.Environment)
		argNum++
	}
	if q.InitiatorID != "" {
		where += fmt.Sprintf(" AND initiator_id = $%d", argNum)
		args = append(args, q.InitiatorID)
		argNum++
	}
	if !q.Since.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, q.Since)
		argNum++
	}
	if !q.Until.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, q.Until)
		argNum++
	}

	// Count
	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM entity_stream "+where, args...).Scan(&total)
	if err != nil {
		return nil, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset

	// Fetch page
	query := fmt.Sprintf(
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        entity_id, entity_type, entity_name
		 FROM entity_stream %s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d`,
		where, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*EntityEvent
	for rows.Next() {
		e, err := scanEntityEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		events = []*EntityEvent{}
	}

	return &StreamPage[EntityEvent]{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// --- State stream ---

func (s *PostgresStore) AppendState(ctx context.Context, event *StateEvent) error {
	beforeJSON, _ := json.Marshal(event.BeforeState)
	afterJSON, _ := json.Marshal(event.AfterState)
	metaJSON, _ := json.Marshal(event.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO state_stream
		 (id, project_id, change_type, affected_entities, before_state, after_state,
		  initiator_id, initiator_type, environment, metadata, timestamp,
		  entity_id, source, observed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		event.ID, event.ProjectID, string(event.ChangeType), event.AffectedEntities,
		beforeJSON, afterJSON, event.InitiatorID, string(event.InitiatorType),
		event.Environment, metaJSON, event.Timestamp,
		event.EntityID, event.Source, event.ObservedAt)
	return err
}

func (s *PostgresStore) GetState(ctx context.Context, id string) (*StateEvent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        entity_id, source, observed_at
		 FROM state_stream WHERE id = $1`, id)
	return scanStateEvent(row)
}

func (s *PostgresStore) ListState(ctx context.Context, q StreamQuery) (*StreamPage[StateEvent], error) {
	where := "WHERE project_id = $1"
	args := []any{q.ProjectID}
	argNum := 2

	if q.EntityID != "" {
		where += fmt.Sprintf(" AND entity_id = $%d", argNum)
		args = append(args, q.EntityID)
		argNum++
	}
	if q.ChangeType != "" {
		where += fmt.Sprintf(" AND change_type = $%d", argNum)
		args = append(args, q.ChangeType)
		argNum++
	}
	if q.Environment != "" {
		where += fmt.Sprintf(" AND environment = $%d", argNum)
		args = append(args, q.Environment)
		argNum++
	}
	if q.InitiatorID != "" {
		where += fmt.Sprintf(" AND initiator_id = $%d", argNum)
		args = append(args, q.InitiatorID)
		argNum++
	}
	if q.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", argNum)
		args = append(args, q.Source)
		argNum++
	}
	if !q.Since.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, q.Since)
		argNum++
	}
	if !q.Until.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, q.Until)
		argNum++
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM state_stream "+where, args...).Scan(&total)
	if err != nil {
		return nil, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset

	query := fmt.Sprintf(
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        entity_id, source, observed_at
		 FROM state_stream %s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d`,
		where, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*StateEvent
	for rows.Next() {
		e, err := scanStateEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		events = []*StateEvent{}
	}

	return &StreamPage[StateEvent]{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// --- Change stream ---

func (s *PostgresStore) AppendChange(ctx context.Context, event *ChangeEvent) error {
	beforeJSON, _ := json.Marshal(event.BeforeState)
	afterJSON, _ := json.Marshal(event.AfterState)
	metaJSON, _ := json.Marshal(event.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO change_stream
		 (id, project_id, change_type, affected_entities, before_state, after_state,
		  initiator_id, initiator_type, environment, metadata, timestamp, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		event.ID, event.ProjectID, event.ChangeType, event.AffectedEntities,
		beforeJSON, afterJSON, event.InitiatorID, string(event.InitiatorType),
		event.Environment, metaJSON, event.Timestamp, event.Source)
	return err
}

func (s *PostgresStore) GetChange(ctx context.Context, id string) (*ChangeEvent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        source
		 FROM change_stream WHERE id = $1`, id)
	return scanChangeEvent(row)
}

func (s *PostgresStore) ListChange(ctx context.Context, q StreamQuery) (*StreamPage[ChangeEvent], error) {
	where := "WHERE project_id = $1"
	args := []any{q.ProjectID}
	argNum := 2

	if q.EntityID != "" {
		where += fmt.Sprintf(" AND $%d = ANY(affected_entities)", argNum)
		args = append(args, q.EntityID)
		argNum++
	}
	if q.ChangeType != "" {
		where += fmt.Sprintf(" AND change_type = $%d", argNum)
		args = append(args, q.ChangeType)
		argNum++
	}
	if q.Environment != "" {
		where += fmt.Sprintf(" AND environment = $%d", argNum)
		args = append(args, q.Environment)
		argNum++
	}
	if q.InitiatorID != "" {
		where += fmt.Sprintf(" AND initiator_id = $%d", argNum)
		args = append(args, q.InitiatorID)
		argNum++
	}
	if q.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", argNum)
		args = append(args, q.Source)
		argNum++
	}
	if !q.Since.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, q.Since)
		argNum++
	}
	if !q.Until.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, q.Until)
		argNum++
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM change_stream "+where, args...).Scan(&total)
	if err != nil {
		return nil, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset

	query := fmt.Sprintf(
		`SELECT id, project_id, change_type, affected_entities, before_state, after_state,
		        initiator_id, initiator_type, environment, metadata, timestamp, created_at,
		        source
		 FROM change_stream %s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d`,
		where, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*ChangeEvent
	for rows.Next() {
		e, err := scanChangeEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		events = []*ChangeEvent{}
	}

	return &StreamPage[ChangeEvent]{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// --- Scan helpers ---

func scanEntityEvent(row pgx.Row) (*EntityEvent, error) {
	var e EntityEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := row.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.EntityID, &e.EntityType, &e.EntityName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func scanEntityEventRow(rows pgx.Rows) (*EntityEvent, error) {
	var e EntityEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := rows.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.EntityID, &e.EntityType, &e.EntityName)
	if err != nil {
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func scanStateEvent(row pgx.Row) (*StateEvent, error) {
	var e StateEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := row.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.EntityID, &e.Source, &e.ObservedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func scanStateEventRow(rows pgx.Rows) (*StateEvent, error) {
	var e StateEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := rows.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.EntityID, &e.Source, &e.ObservedAt)
	if err != nil {
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func scanChangeEvent(row pgx.Row) (*ChangeEvent, error) {
	var e ChangeEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := row.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.Source)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func scanChangeEventRow(rows pgx.Rows) (*ChangeEvent, error) {
	var e ChangeEvent
	var beforeJSON, afterJSON, metaJSON []byte
	err := rows.Scan(
		&e.ID, &e.ProjectID, &e.ChangeType, &e.AffectedEntities,
		&beforeJSON, &afterJSON, &e.InitiatorID, &e.InitiatorType,
		&e.Environment, &metaJSON, &e.Timestamp, &e.CreatedAt,
		&e.Source)
	if err != nil {
		return nil, err
	}
	unmarshalOptional(beforeJSON, &e.BeforeState)
	unmarshalOptional(afterJSON, &e.AfterState)
	unmarshalOptional(metaJSON, &e.Metadata)
	return &e, nil
}

func unmarshalOptional(data []byte, target *map[string]any) {
	if len(data) > 0 {
		_ = json.Unmarshal(data, target)
	}
}
