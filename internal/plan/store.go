package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrVersionConflict is returned when an optimistic lock check fails.
var ErrVersionConflict = errors.New("plan version conflict")

// ErrPlanNotFound is returned when a plan ID does not exist.
var ErrPlanNotFound = errors.New("plan not found")

// Store persists plans and plan versions.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new plan Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// planColumns is the column list for SELECT queries.
const planColumns = `id, ticket_id, backend, state, version, content, freshness_stamp, freshness_data, expires_at, environment, created_by, created_at, updated_at`

// Create inserts a new plan. ID must already be set.
func (s *Store) Create(ctx context.Context, p *Plan) error {
	contentJSON, err := json.Marshal(p.Content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	var freshnessDataJSON []byte
	if p.FreshnessData != nil {
		freshnessDataJSON, err = json.Marshal(p.FreshnessData)
		if err != nil {
			return fmt.Errorf("marshal freshness_data: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO plans (id, ticket_id, backend, state, version, content, freshness_stamp, freshness_data, expires_at, environment, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		p.ID, p.TicketID, string(p.Backend), string(p.State), p.Version,
		contentJSON, p.FreshnessStamp, freshnessDataJSON, p.ExpiresAt, p.Environment,
		p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

// GetByID returns a plan by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Plan, error) {
	var p Plan
	var contentJSON []byte
	var freshnessDataJSON []byte
	var expiresAt *time.Time
	var environment *string
	err := s.pool.QueryRow(ctx,
		`SELECT `+planColumns+` FROM plans WHERE id = $1`, id).
		Scan(&p.ID, &p.TicketID, &p.Backend, &p.State, &p.Version,
			&contentJSON, &p.FreshnessStamp, &freshnessDataJSON, &expiresAt, &environment,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, err
	}
	p.ExpiresAt = expiresAt
	if environment != nil {
		p.Environment = *environment
	}
	if err := json.Unmarshal(contentJSON, &p.Content); err != nil {
		return nil, fmt.Errorf("unmarshal content: %w", err)
	}
	if len(freshnessDataJSON) > 0 {
		p.FreshnessData = &FreshnessData{}
		if err := json.Unmarshal(freshnessDataJSON, p.FreshnessData); err != nil {
			return nil, fmt.Errorf("unmarshal freshness_data: %w", err)
		}
	}
	return &p, nil
}

// ListByTicket returns all plans for a given ticket, ordered by creation time (newest first).
func (s *Store) ListByTicket(ctx context.Context, ticketID string) ([]*Plan, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+planColumns+` FROM plans WHERE ticket_id = $1 ORDER BY created_at DESC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// ListByTicketAndBackend returns plans for a ticket filtered by backend.
func (s *Store) ListByTicketAndBackend(ctx context.Context, ticketID string, backend Backend) ([]*Plan, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+planColumns+` FROM plans WHERE ticket_id = $1 AND backend = $2 ORDER BY created_at DESC`,
		ticketID, string(backend))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// UpdateState sets the plan state with optimistic locking. Returns ErrVersionConflict if version mismatches.
func (s *Store) UpdateState(ctx context.Context, id string, version int, newState State) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE plans SET state = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`,
		string(newState), id, version)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// UpdateContent replaces plan content and increments version with optimistic locking.
func (s *Store) UpdateContent(ctx context.Context, id string, version int, content Content) error {
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE plans SET content = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`,
		contentJSON, id, version)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// UpdateFreshness sets the freshness stamp and optional expiry.
func (s *Store) UpdateFreshness(ctx context.Context, id string, freshnessStamp time.Time, expiresAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE plans SET freshness_stamp = $1, expires_at = $2, updated_at = now() WHERE id = $3`,
		freshnessStamp, expiresAt, id)
	return err
}

// UpdateFreshnessData sets the rich freshness data (JSONB) for a plan.
func (s *Store) UpdateFreshnessData(ctx context.Context, id string, freshnessData *FreshnessData) error {
	var dataJSON []byte
	var err error
	if freshnessData != nil {
		dataJSON, err = json.Marshal(freshnessData)
		if err != nil {
			return fmt.Errorf("marshal freshness_data: %w", err)
		}
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE plans SET freshness_data = $1, updated_at = now() WHERE id = $2`,
		dataJSON, id)
	return err
}

// SupersedeByTicketAndBackend marks all non-terminal plans for a ticket+backend as superseded.
func (s *Store) SupersedeByTicketAndBackend(ctx context.Context, ticketID string, backend Backend, exceptPlanID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE plans SET state = 'superseded', updated_at = now()
		 WHERE ticket_id = $1 AND backend = $2 AND id != $3 AND state NOT IN ('applied', 'superseded', 'rejected')`,
		ticketID, string(backend), exceptPlanID)
	return err
}

// CreateVersion inserts a plan version record.
func (s *Store) CreateVersion(ctx context.Context, v *PlanVersion) error {
	contentJSON, err := json.Marshal(v.Content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO plan_versions (id, plan_id, version, content, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, v.PlanID, v.Version, contentJSON, v.CreatedBy, v.CreatedAt)
	return err
}

// ListVersions returns all versions of a plan ordered by version number.
func (s *Store) ListVersions(ctx context.Context, planID string) ([]*PlanVersion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, version, content, created_by, created_at
		 FROM plan_versions WHERE plan_id = $1 ORDER BY version`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*PlanVersion
	for rows.Next() {
		var v PlanVersion
		var contentJSON []byte
		if err := rows.Scan(&v.ID, &v.PlanID, &v.Version, &contentJSON, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(contentJSON, &v.Content)
		list = append(list, &v)
	}
	return list, rows.Err()
}

func (s *Store) scanRows(rows pgx.Rows) ([]*Plan, error) {
	var list []*Plan
	for rows.Next() {
		var p Plan
		var contentJSON []byte
		var freshnessDataJSON []byte
		var expiresAt *time.Time
		var environment *string
		if err := rows.Scan(&p.ID, &p.TicketID, &p.Backend, &p.State, &p.Version,
			&contentJSON, &p.FreshnessStamp, &freshnessDataJSON, &expiresAt, &environment,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ExpiresAt = expiresAt
		if environment != nil {
			p.Environment = *environment
		}
		_ = json.Unmarshal(contentJSON, &p.Content)
		if len(freshnessDataJSON) > 0 {
			p.FreshnessData = &FreshnessData{}
			_ = json.Unmarshal(freshnessDataJSON, p.FreshnessData)
		}
		list = append(list, &p)
	}
	return list, rows.Err()
}
