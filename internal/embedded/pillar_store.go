package embedded

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gabinante/flywheel/internal/pillar"
)

// PillarStore implements pillar.Store using SQLite.
type PillarStore struct{ db *sql.DB }

// NewPillarStore returns a new SQLite-backed pillar store.
func NewPillarStore(db *sql.DB) *PillarStore { return &PillarStore{db: db} }

// ────────────────────────────────────────────────────────────────────────────
// Entry CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PillarStore) CreateEntry(_ context.Context, e *pillar.Entry) error {
	gapsJSON, _ := json.Marshal(e.Gaps)
	var lastReviewed, nextReview *string
	if e.LastReviewedAt != nil {
		v := e.LastReviewedAt.UTC().Format(time.RFC3339Nano)
		lastReviewed = &v
	}
	if e.NextReviewAt != nil {
		v := e.NextReviewAt.UTC().Format(time.RFC3339Nano)
		nextReview = &v
	}
	_, err := s.db.Exec(
		`INSERT INTO pillar_entries
			(id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
			 review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.EntityID, e.EntityType, string(e.PillarType),
		e.Strategy, string(gapsJSON), string(e.ReviewCadence),
		lastReviewed, nextReview,
		e.Version, e.CreatedBy,
		e.CreatedAt.UTC().Format(time.RFC3339Nano),
		e.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *PillarStore) GetEntry(_ context.Context, id string) (*pillar.Entry, error) {
	var e pillar.Entry
	var gapsJSON string
	var pt, cad string
	var lastReviewed, nextReview sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
				review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		 FROM pillar_entries WHERE id = ?`, id).
		Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pt,
			&e.Strategy, &gapsJSON, &cad,
			&lastReviewed, &nextReview,
			&e.Version, &e.CreatedBy, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, pillar.ErrEntryNotFound
		}
		return nil, err
	}
	e.PillarType = pillar.PillarType(pt)
	e.ReviewCadence = pillar.ReviewCadence(cad)
	_ = json.Unmarshal([]byte(gapsJSON), &e.Gaps)
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastReviewed.Valid {
		t, _ := time.Parse(time.RFC3339Nano, lastReviewed.String)
		e.LastReviewedAt = &t
	}
	if nextReview.Valid {
		t, _ := time.Parse(time.RFC3339Nano, nextReview.String)
		e.NextReviewAt = &t
	}
	return &e, nil
}

func (s *PillarStore) ListEntries(_ context.Context, projectID, entityID, pillarType string, limit int) ([]*pillar.Entry, error) {
	q := `SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
			review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		  FROM pillar_entries WHERE project_id = ?`
	args := []any{projectID}

	if entityID != "" {
		q += " AND entity_id = ?"
		args = append(args, entityID)
	}
	if pillarType != "" {
		q += " AND pillar_type = ?"
		args = append(args, pillarType)
	}
	q += " ORDER BY pillar_type, entity_type LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*pillar.Entry
	for rows.Next() {
		var e pillar.Entry
		var gapsJSON string
		var pt, cad string
		var lastReviewed, nextReview sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pt,
			&e.Strategy, &gapsJSON, &cad,
			&lastReviewed, &nextReview,
			&e.Version, &e.CreatedBy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.PillarType = pillar.PillarType(pt)
		e.ReviewCadence = pillar.ReviewCadence(cad)
		_ = json.Unmarshal([]byte(gapsJSON), &e.Gaps)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastReviewed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastReviewed.String)
			e.LastReviewedAt = &t
		}
		if nextReview.Valid {
			t, _ := time.Parse(time.RFC3339Nano, nextReview.String)
			e.NextReviewAt = &t
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

func (s *PillarStore) UpdateEntry(_ context.Context, e *pillar.Entry) error {
	gapsJSON, _ := json.Marshal(e.Gaps)
	var lastReviewed, nextReview *string
	if e.LastReviewedAt != nil {
		v := e.LastReviewedAt.UTC().Format(time.RFC3339Nano)
		lastReviewed = &v
	}
	if e.NextReviewAt != nil {
		v := e.NextReviewAt.UTC().Format(time.RFC3339Nano)
		nextReview = &v
	}
	res, err := s.db.Exec(
		`UPDATE pillar_entries SET strategy = ?, gaps = ?, review_cadence = ?,
			last_reviewed_at = ?, next_review_at = ?, version = ?, updated_at = ?
		 WHERE id = ? AND version = ?`,
		e.Strategy, string(gapsJSON), string(e.ReviewCadence),
		lastReviewed, nextReview,
		e.Version, e.UpdatedAt.UTC().Format(time.RFC3339Nano),
		e.ID, e.Version-1)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return pillar.ErrVersionConflict
	}
	return nil
}

func (s *PillarStore) DeleteEntry(_ context.Context, id string) error {
	_, _ = s.db.Exec(`DELETE FROM pillar_evaluations WHERE pillar_entry_id = ?`, id)
	_, _ = s.db.Exec(`DELETE FROM pillar_claims WHERE pillar_entry_id = ?`, id)
	_, err := s.db.Exec(`DELETE FROM pillar_entries WHERE id = ?`, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Claim CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PillarStore) CreateClaim(_ context.Context, c *pillar.Claim) error {
	var lastEval *string
	if c.LastEvaluatedAt != nil {
		v := c.LastEvaluatedAt.UTC().Format(time.RFC3339Nano)
		lastEval = &v
	}
	_, err := s.db.Exec(
		`INSERT INTO pillar_claims
			(id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
			 status, last_evaluated_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.PillarEntryID, c.Statement, c.EntityRefID, c.EntityRefType,
		c.Evidence, string(c.Status), lastEval,
		c.CreatedAt.UTC().Format(time.RFC3339Nano),
		c.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *PillarStore) GetClaim(_ context.Context, id string) (*pillar.Claim, error) {
	var c pillar.Claim
	var status string
	var lastEval sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
				status, last_evaluated_at, created_at, updated_at
		 FROM pillar_claims WHERE id = ?`, id).
		Scan(&c.ID, &c.PillarEntryID, &c.Statement, &c.EntityRefID, &c.EntityRefType,
			&c.Evidence, &status, &lastEval, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, pillar.ErrClaimNotFound
		}
		return nil, err
	}
	c.Status = pillar.ClaimStatus(status)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastEval.Valid {
		t, _ := time.Parse(time.RFC3339Nano, lastEval.String)
		c.LastEvaluatedAt = &t
	}
	return &c, nil
}

func (s *PillarStore) ListClaims(_ context.Context, pillarEntryID string) ([]*pillar.Claim, error) {
	rows, err := s.db.Query(
		`SELECT id, pillar_entry_id, statement, entity_ref_id, entity_ref_type, evidence,
				status, last_evaluated_at, created_at, updated_at
		 FROM pillar_claims WHERE pillar_entry_id = ? ORDER BY created_at`, pillarEntryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claims []*pillar.Claim
	for rows.Next() {
		var c pillar.Claim
		var status string
		var lastEval sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.PillarEntryID, &c.Statement, &c.EntityRefID, &c.EntityRefType,
			&c.Evidence, &status, &lastEval, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.Status = pillar.ClaimStatus(status)
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastEval.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastEval.String)
			c.LastEvaluatedAt = &t
		}
		claims = append(claims, &c)
	}
	return claims, rows.Err()
}

func (s *PillarStore) UpdateClaim(_ context.Context, c *pillar.Claim) error {
	var lastEval *string
	if c.LastEvaluatedAt != nil {
		v := c.LastEvaluatedAt.UTC().Format(time.RFC3339Nano)
		lastEval = &v
	}
	_, err := s.db.Exec(
		`UPDATE pillar_claims SET statement = ?, evidence = ?, status = ?,
			last_evaluated_at = ?, updated_at = ?
		 WHERE id = ?`,
		c.Statement, c.Evidence, string(c.Status), lastEval,
		c.UpdatedAt.UTC().Format(time.RFC3339Nano), c.ID)
	return err
}

func (s *PillarStore) DeleteClaim(_ context.Context, id string) error {
	_, err := s.db.Exec(`DELETE FROM pillar_claims WHERE id = ?`, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Evaluations
// ────────────────────────────────────────────────────────────────────────────

func (s *PillarStore) CreateEvaluation(_ context.Context, ev *pillar.Evaluation) error {
	_, err := s.db.Exec(
		`INSERT INTO pillar_evaluations (id, pillar_entry_id, check_type, outcome, details, evaluated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.PillarEntryID, string(ev.CheckType), string(ev.Outcome), ev.Details,
		ev.EvaluatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *PillarStore) ListEvaluations(_ context.Context, pillarEntryID string, limit int) ([]*pillar.Evaluation, error) {
	rows, err := s.db.Query(
		`SELECT id, pillar_entry_id, check_type, outcome, details, evaluated_at
		 FROM pillar_evaluations WHERE pillar_entry_id = ?
		 ORDER BY evaluated_at DESC LIMIT ?`, pillarEntryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evals []*pillar.Evaluation
	for rows.Next() {
		var ev pillar.Evaluation
		var ct, oc string
		var evalAt string
		if err := rows.Scan(&ev.ID, &ev.PillarEntryID, &ct, &oc, &ev.Details, &evalAt); err != nil {
			return nil, err
		}
		ev.CheckType = pillar.EvalCheckType(ct)
		ev.Outcome = pillar.EvalOutcome(oc)
		ev.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, evalAt)
		evals = append(evals, &ev)
	}
	return evals, rows.Err()
}

// ────────────────────────────────────────────────────────────────────────────
// Review scheduling
// ────────────────────────────────────────────────────────────────────────────

func (s *PillarStore) ListDueForReview(_ context.Context, projectID string, before time.Time, limit int) ([]*pillar.Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, entity_id, entity_type, pillar_type, strategy, gaps,
				review_cadence, last_reviewed_at, next_review_at, version, created_by, created_at, updated_at
		 FROM pillar_entries
		 WHERE project_id = ? AND next_review_at IS NOT NULL AND next_review_at <= ?
		 ORDER BY next_review_at LIMIT ?`,
		projectID, before.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*pillar.Entry
	for rows.Next() {
		var e pillar.Entry
		var gapsJSON string
		var pt, cad string
		var lastReviewed, nextReview sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EntityID, &e.EntityType, &pt,
			&e.Strategy, &gapsJSON, &cad,
			&lastReviewed, &nextReview,
			&e.Version, &e.CreatedBy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.PillarType = pillar.PillarType(pt)
		e.ReviewCadence = pillar.ReviewCadence(cad)
		_ = json.Unmarshal([]byte(gapsJSON), &e.Gaps)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastReviewed.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastReviewed.String)
			e.LastReviewedAt = &t
		}
		if nextReview.Valid {
			t, _ := time.Parse(time.RFC3339Nano, nextReview.String)
			e.NextReviewAt = &t
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}
