package pillar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a new PostgreSQL-backed pillar store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// ────────────────────────────────────────────────────────────────────────────
// Entry CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateEntry(ctx context.Context, e *Entry) error {
	gapsJSON, err := json.Marshal(e.Gaps)
	if err != nil {
		return fmt.Errorf("marshal gaps: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO pillar_entries
			(id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
			 review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		e.ID, e.ProjectID, e.EntityID, e.EntityType, string(e.PillarType),
		e.Strategy, gapsJSON, string(e.ReviewCadence),
		e.LastReviewedAt, e.NextReviewAt,
		e.Version, e.CreatedBy, e.CreatedAt, e.UpdatedAt)
	return err
}

func (s *PostgresStore) GetEntry(ctx context.Context, id string) (*Entry, error) {
	var e Entry
	var gapsJSON []byte
	var pillarType, cadence string
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
				review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		 FROM pillar_entries WHERE id = $1`, id).
		Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pillarType,
			&e.Strategy, &gapsJSON, &cadence,
			&e.LastReviewedAt, &e.NextReviewAt,
			&e.Version, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, err
	}
	e.PillarType = PillarType(pillarType)
	e.ReviewCadence = ReviewCadence(cadence)
	_ = json.Unmarshal(gapsJSON, &e.Gaps)
	return &e, nil
}

func (s *PostgresStore) ListEntries(ctx context.Context, projectID, entityID, pillarType string, limit int) ([]*Entry, error) {
	q := `SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
			review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		  FROM pillar_entries WHERE project_id = $1`
	args := []any{projectID}
	idx := 2

	if entityID != "" {
		q += fmt.Sprintf(" AND entity_id = $%d", idx)
		args = append(args, entityID)
		idx++
	}
	if pillarType != "" {
		q += fmt.Sprintf(" AND pillar_type = $%d", idx)
		args = append(args, pillarType)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY pillar_type, entity_type LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var e Entry
		var gapsJSON []byte
		var pt, cad string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pt,
			&e.Strategy, &gapsJSON, &cad,
			&e.LastReviewedAt, &e.NextReviewAt,
			&e.Version, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.PillarType = PillarType(pt)
		e.ReviewCadence = ReviewCadence(cad)
		_ = json.Unmarshal(gapsJSON, &e.Gaps)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

func (s *PostgresStore) UpdateEntry(ctx context.Context, e *Entry) error {
	gapsJSON, err := json.Marshal(e.Gaps)
	if err != nil {
		return fmt.Errorf("marshal gaps: %w", err)
	}
	cmd, err := s.pool.Exec(ctx,
		`UPDATE pillar_entries SET strategy = $1, gaps = $2, review_cadence = $3,
			last_reviewed_at = $4, next_review_at = $5, version = $6, updated_at = $7
		 WHERE id = $8 AND version = $9`,
		e.Strategy, gapsJSON, string(e.ReviewCadence),
		e.LastReviewedAt, e.NextReviewAt,
		e.Version, e.UpdatedAt,
		e.ID, e.Version-1)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *PostgresStore) DeleteEntry(ctx context.Context, id string) error {
	// Delete claims and evaluations first (referential integrity).
	_, _ = s.pool.Exec(ctx, `DELETE FROM pillar_evaluations WHERE pillar_entry_id = $1`, id)
	_, _ = s.pool.Exec(ctx, `DELETE FROM pillar_claims WHERE pillar_entry_id = $1`, id)
	_, err := s.pool.Exec(ctx, `DELETE FROM pillar_entries WHERE id = $1`, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Claim CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateClaim(ctx context.Context, c *Claim) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO pillar_claims
			(id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
			 status, last_evaluated_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.PillarEntryID, c.Statement, c.EntityRefID, c.EntityRefType,
		c.Evidence, string(c.Status), c.LastEvaluatedAt, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *PostgresStore) GetClaim(ctx context.Context, id string) (*Claim, error) {
	var c Claim
	var status string
	err := s.pool.QueryRow(ctx,
		`SELECT id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
				status, last_evaluated_at, created_at, updated_at
		 FROM pillar_claims WHERE id = $1`, id).
		Scan(&c.ID, &c.PillarEntryID, &c.Statement, &c.EntityRefID, &c.EntityRefType,
			&c.Evidence, &status, &c.LastEvaluatedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClaimNotFound
		}
		return nil, err
	}
	c.Status = ClaimStatus(status)
	return &c, nil
}

func (s *PostgresStore) ListClaims(ctx context.Context, pillarEntryID string) ([]*Claim, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
				status, last_evaluated_at, created_at, updated_at
		 FROM pillar_claims WHERE pillar_entry_id = $1 ORDER BY created_at`, pillarEntryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claims []*Claim
	for rows.Next() {
		var c Claim
		var status string
		if err := rows.Scan(&c.ID, &c.PillarEntryID, &c.Statement, &c.EntityRefID, &c.EntityRefType,
			&c.Evidence, &status, &c.LastEvaluatedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Status = ClaimStatus(status)
		claims = append(claims, &c)
	}
	return claims, rows.Err()
}

func (s *PostgresStore) UpdateClaim(ctx context.Context, c *Claim) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE pillar_claims SET statement = $1, evidence = $2, status = $3,
			last_evaluated_at = $4, updated_at = $5
		 WHERE id = $6`,
		c.Statement, c.Evidence, string(c.Status), c.LastEvaluatedAt, c.UpdatedAt, c.ID)
	return err
}

func (s *PostgresStore) DeleteClaim(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM pillar_claims WHERE id = $1`, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Evaluations
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateEvaluation(ctx context.Context, ev *Evaluation) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO pillar_evaluations (id, pillar_entry_id, check_type, outcome, details, evaluated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ev.ID, ev.PillarEntryID, string(ev.CheckType), string(ev.Outcome), ev.Details, ev.EvaluatedAt)
	return err
}

func (s *PostgresStore) ListEvaluations(ctx context.Context, pillarEntryID string, limit int) ([]*Evaluation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, pillar_entry_id, check_type, outcome, details, evaluated_at
		 FROM pillar_evaluations WHERE pillar_entry_id = $1
		 ORDER BY evaluated_at DESC LIMIT $2`, pillarEntryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evals []*Evaluation
	for rows.Next() {
		var ev Evaluation
		var ct, oc string
		if err := rows.Scan(&ev.ID, &ev.PillarEntryID, &ct, &oc, &ev.Details, &ev.EvaluatedAt); err != nil {
			return nil, err
		}
		ev.CheckType = EvalCheckType(ct)
		ev.Outcome = EvalOutcome(oc)
		evals = append(evals, &ev)
	}
	return evals, rows.Err()
}

// ────────────────────────────────────────────────────────────────────────────
// Review scheduling
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) ListDueForReview(ctx context.Context, projectID string, before time.Time, limit int) ([]*Entry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
				review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		 FROM pillar_entries
		 WHERE project_id = $1 AND next_review_at IS NOT NULL AND next_review_at <= $2
		 ORDER BY next_review_at LIMIT $3`, projectID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var e Entry
		var gapsJSON []byte
		var pt, cad string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pt,
			&e.Strategy, &gapsJSON, &cad,
			&e.LastReviewedAt, &e.NextReviewAt,
			&e.Version, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.PillarType = PillarType(pt)
		e.ReviewCadence = ReviewCadence(cad)
		_ = json.Unmarshal(gapsJSON, &e.Gaps)
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}
