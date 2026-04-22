package observation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ Store = (*PostgresStore)(nil)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a new Postgres-backed observation store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateWindow(ctx context.Context, window *ObservationWindow) error {
	scopeJSON, _ := json.Marshal(window.Scope)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO observation_windows (id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		window.ID, window.TicketID, window.ProjectID, scopeJSON,
		window.StartedAt, window.EndsAt, window.ClosedAt, string(window.State))
	return err
}

func (s *PostgresStore) CloseWindow(ctx context.Context, windowID string, closedAt time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE observation_windows SET closed_at = $1, state = 'closed' WHERE id = $2`,
		closedAt, windowID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrWindowNotFound
	}
	return nil
}

func (s *PostgresStore) GetWindow(ctx context.Context, windowID string) (*ObservationWindow, error) {
	return s.scanWindow(s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state
		 FROM observation_windows WHERE id = $1`, windowID))
}

func (s *PostgresStore) GetWindowByTicket(ctx context.Context, ticketID string) (*ObservationWindow, error) {
	return s.scanWindow(s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state
		 FROM observation_windows WHERE ticket_id = $1 ORDER BY started_at DESC LIMIT 1`, ticketID))
}

func (s *PostgresStore) GetOpenWindows(ctx context.Context, projectID string) ([]*ObservationWindow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state
		 FROM observation_windows WHERE project_id = $1 AND state IN ('active', 'extended')`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanWindows(rows)
}

func (s *PostgresStore) GetOverlappingWindows(ctx context.Context, projectID string, start, end time.Time) ([]*ObservationWindow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state
		 FROM observation_windows
		 WHERE project_id = $1
		   AND started_at <= $3
		   AND COALESCE(closed_at, ends_at) >= $2`, projectID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanWindows(rows)
}

func (s *PostgresStore) GetRecentlyClosedWindows(ctx context.Context, projectID string, lookback time.Duration) ([]*ObservationWindow, error) {
	cutoff := time.Now().UTC().Add(-lookback)
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, project_id, scope, started_at, ends_at, closed_at, state
		 FROM observation_windows
		 WHERE project_id = $1 AND state = 'closed' AND closed_at >= $2`, projectID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanWindows(rows)
}

func (s *PostgresStore) CreateSignal(ctx context.Context, signal *Signal) error {
	scopeJSON, _ := json.Marshal(signal.Scope)
	metaJSON, _ := json.Marshal(signal.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO signals (id, project_id, source, signal_type, severity, title, detail, scope, metadata, occurred_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		signal.ID, signal.ProjectID, signal.Source, string(signal.SignalType),
		string(signal.Severity), signal.Title, signal.Detail, scopeJSON, metaJSON,
		signal.OccurredAt, signal.CreatedAt)
	return err
}

func (s *PostgresStore) GetSignal(ctx context.Context, signalID string) (*Signal, error) {
	var sig Signal
	var scopeJSON, metaJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, source, signal_type, severity, title, detail, scope, metadata, occurred_at, created_at
		 FROM signals WHERE id = $1`, signalID).
		Scan(&sig.ID, &sig.ProjectID, &sig.Source, &sig.SignalType, &sig.Severity,
			&sig.Title, &sig.Detail, &scopeJSON, &metaJSON, &sig.OccurredAt, &sig.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSignalNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(scopeJSON, &sig.Scope)
	if metaJSON != nil {
		_ = json.Unmarshal(metaJSON, &sig.Metadata)
	}
	return &sig, nil
}

func (s *PostgresStore) ListSignals(ctx context.Context, projectID string, since time.Time, limit int) ([]*Signal, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, source, signal_type, severity, title, detail, scope, metadata, occurred_at, created_at
		 FROM signals WHERE project_id = $1 AND occurred_at >= $2
		 ORDER BY occurred_at DESC LIMIT $3`, projectID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Signal
	for rows.Next() {
		var sig Signal
		var scopeJSON, metaJSON []byte
		if err := rows.Scan(&sig.ID, &sig.ProjectID, &sig.Source, &sig.SignalType, &sig.Severity,
			&sig.Title, &sig.Detail, &scopeJSON, &metaJSON, &sig.OccurredAt, &sig.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scopeJSON, &sig.Scope)
		if metaJSON != nil {
			_ = json.Unmarshal(metaJSON, &sig.Metadata)
		}
		result = append(result, &sig)
	}
	return result, rows.Err()
}

func (s *PostgresStore) CreateAttribution(ctx context.Context, attr *Attribution) error {
	candidatesJSON, _ := json.Marshal(attr.Candidates)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO attributions (id, signal_id, project_id, candidates, confidence, rationale, retroactive, resolved, resolved_by, resolution, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		attr.ID, attr.SignalID, attr.ProjectID, candidatesJSON, attr.Confidence,
		attr.Rationale, attr.Retroactive, attr.Resolved, nullIfEmpty(attr.ResolvedBy),
		nullIfEmpty(attr.Resolution), attr.CreatedAt, attr.UpdatedAt)
	return err
}

func (s *PostgresStore) GetAttribution(ctx context.Context, attrID string) (*Attribution, error) {
	var attr Attribution
	var candidatesJSON []byte
	var resolvedBy, resolution *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, signal_id, project_id, candidates, confidence, rationale, retroactive, resolved, resolved_by, resolution, created_at, updated_at
		 FROM attributions WHERE id = $1`, attrID).
		Scan(&attr.ID, &attr.SignalID, &attr.ProjectID, &candidatesJSON, &attr.Confidence,
			&attr.Rationale, &attr.Retroactive, &attr.Resolved, &resolvedBy, &resolution,
			&attr.CreatedAt, &attr.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrAttributionNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(candidatesJSON, &attr.Candidates)
	if resolvedBy != nil {
		attr.ResolvedBy = *resolvedBy
	}
	if resolution != nil {
		attr.Resolution = *resolution
	}
	return &attr, nil
}

func (s *PostgresStore) GetAttributionBySignal(ctx context.Context, signalID string) (*Attribution, error) {
	var attr Attribution
	var candidatesJSON []byte
	var resolvedBy, resolution *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, signal_id, project_id, candidates, confidence, rationale, retroactive, resolved, resolved_by, resolution, created_at, updated_at
		 FROM attributions WHERE signal_id = $1`, signalID).
		Scan(&attr.ID, &attr.SignalID, &attr.ProjectID, &candidatesJSON, &attr.Confidence,
			&attr.Rationale, &attr.Retroactive, &attr.Resolved, &resolvedBy, &resolution,
			&attr.CreatedAt, &attr.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrAttributionNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(candidatesJSON, &attr.Candidates)
	if resolvedBy != nil {
		attr.ResolvedBy = *resolvedBy
	}
	if resolution != nil {
		attr.Resolution = *resolution
	}
	return &attr, nil
}

func (s *PostgresStore) ListUnresolvedAttributions(ctx context.Context, projectID string) ([]*Attribution, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, signal_id, project_id, candidates, confidence, rationale, retroactive, resolved, resolved_by, resolution, created_at, updated_at
		 FROM attributions WHERE project_id = $1 AND resolved = false
		 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanAttributions(rows)
}

func (s *PostgresStore) ResolveAttribution(ctx context.Context, attrID, resolvedBy, resolution string) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE attributions SET resolved = true, resolved_by = $1, resolution = $2, updated_at = $3 WHERE id = $4`,
		resolvedBy, resolution, now, attrID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAttributionNotFound
	}
	return nil
}

func (s *PostgresStore) GetAmbiguityMetrics(ctx context.Context, projectID string, start, end time.Time) (*AmbiguityMetrics, error) {
	metrics := &AmbiguityMetrics{
		ProjectID:   projectID,
		WindowStart: start,
		WindowEnd:   end,
	}

	// Get total attributions and low-confidence count in one query.
	err := s.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE confidence < 0.5 OR jsonb_array_length(candidates) > 1),
			COALESCE(AVG(jsonb_array_length(candidates)), 0)
		 FROM attributions
		 WHERE project_id = $1 AND created_at >= $2 AND created_at <= $3`,
		projectID, start, end).
		Scan(&metrics.TotalAttributions, &metrics.LowConfidenceCount, &metrics.AvgCandidatesPerSignal)
	if err != nil {
		return nil, err
	}
	if metrics.TotalAttributions > 0 {
		metrics.AmbiguityRate = float64(metrics.LowConfidenceCount) / float64(metrics.TotalAttributions)
	}
	return metrics, nil
}

// Helpers

func (s *PostgresStore) scanWindow(row pgx.Row) (*ObservationWindow, error) {
	var w ObservationWindow
	var scopeJSON []byte
	err := row.Scan(&w.ID, &w.TicketID, &w.ProjectID, &scopeJSON,
		&w.StartedAt, &w.EndsAt, &w.ClosedAt, &w.State)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrWindowNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(scopeJSON, &w.Scope)
	return &w, nil
}

func (s *PostgresStore) scanWindows(rows pgx.Rows) ([]*ObservationWindow, error) {
	var result []*ObservationWindow
	for rows.Next() {
		var w ObservationWindow
		var scopeJSON []byte
		if err := rows.Scan(&w.ID, &w.TicketID, &w.ProjectID, &scopeJSON,
			&w.StartedAt, &w.EndsAt, &w.ClosedAt, &w.State); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scopeJSON, &w.Scope)
		result = append(result, &w)
	}
	return result, rows.Err()
}

func (s *PostgresStore) scanAttributions(rows pgx.Rows) ([]*Attribution, error) {
	var result []*Attribution
	for rows.Next() {
		var attr Attribution
		var candidatesJSON []byte
		var resolvedBy, resolution *string
		if err := rows.Scan(&attr.ID, &attr.SignalID, &attr.ProjectID, &candidatesJSON,
			&attr.Confidence, &attr.Rationale, &attr.Retroactive, &attr.Resolved,
			&resolvedBy, &resolution, &attr.CreatedAt, &attr.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(candidatesJSON, &attr.Candidates)
		if resolvedBy != nil {
			attr.ResolvedBy = *resolvedBy
		}
		if resolution != nil {
			attr.Resolution = *resolution
		}
		result = append(result, &attr)
	}
	return result, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
