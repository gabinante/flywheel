package report

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists composed and posted reports.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const cols = `id, kind, project_id, window_start, window_end, body, health, posted, url, created_at`

func scan(row interface{ Scan(...any) error }) (*Report, error) {
	var r Report
	if err := row.Scan(&r.ID, &r.Kind, &r.ProjectID, &r.WindowStart, &r.WindowEnd, &r.Body, &r.Health, &r.Posted, &r.URL, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// Insert stores a report.
func (s *Store) Insert(ctx context.Context, r *Report) error {
	if r.ID == "" {
		r.ID = uuid.Must(uuid.NewV7()).String()
	}
	return s.pool.QueryRow(ctx, `INSERT INTO project_reports (id, kind, project_id, window_start, window_end, body, health, posted, url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`,
		r.ID, r.Kind, r.ProjectID, r.WindowStart, r.WindowEnd, r.Body, r.Health, r.Posted, r.URL).Scan(&r.CreatedAt)
}

// LastPosted returns when a report of this kind was last posted for a project (nil = never).
func (s *Store) LastPosted(ctx context.Context, projectID, kind string) (*time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx, `SELECT window_end FROM project_reports WHERE project_id = $1 AND kind = $2 AND posted ORDER BY created_at DESC LIMIT 1`, projectID, kind).Scan(&t)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// WeeklyPosted reports whether a roundup covering weekStart was posted.
func (s *Store) WeeklyPosted(ctx context.Context, weekStart time.Time) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM project_reports WHERE kind = $1 AND posted AND window_start = $2`, KindWeeklyRoundup, weekStart).Scan(&n)
	return n > 0, err
}

// List returns reports, newest first; projectID/kind empty = any.
func (s *Store) List(ctx context.Context, projectID, kind string, limit int) ([]*Report, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+cols+` FROM project_reports WHERE ($1 = '' OR project_id = $1) AND ($2 = '' OR kind = $2) ORDER BY created_at DESC LIMIT $3`, projectID, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Report
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
