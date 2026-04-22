package stateindex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// --------------------------------------------------------------------------
// Resources
// --------------------------------------------------------------------------

func (s *PostgresStore) UpsertResource(ctx context.Context, r *ObservedResource) error {
	propsJSON, err := json.Marshal(r.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	var declaredJSON []byte
	if r.DeclaredState != nil {
		declaredJSON, err = json.Marshal(r.DeclaredState)
		if err != nil {
			return fmt.Errorf("marshal declared_state: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO observed_resources (id, project_id, resource_type, environment, name, external_id, provider, region, properties, declared_state, observed_at, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			properties = EXCLUDED.properties,
			declared_state = COALESCE(EXCLUDED.declared_state, observed_resources.declared_state),
			observed_at = EXCLUDED.observed_at,
			source = EXCLUDED.source,
			updated_at = now()`,
		r.ID, r.ProjectID, string(r.ResourceType), r.Environment, r.Name,
		r.ExternalID, r.Provider, r.Region, propsJSON, declaredJSON,
		r.ObservedAt, r.Source,
	)
	return err
}

func (s *PostgresStore) GetResource(ctx context.Context, id string) (*ObservedResource, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, resource_type, environment, name, external_id, provider, region,
		       properties, declared_state, observed_at, source, created_at, updated_at
		FROM observed_resources WHERE id = $1`, id)
	return scanResource(row)
}

func (s *PostgresStore) GetResourceByExternalID(ctx context.Context, projectID, externalID string) (*ObservedResource, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, resource_type, environment, name, external_id, provider, region,
		       properties, declared_state, observed_at, source, created_at, updated_at
		FROM observed_resources WHERE project_id = $1 AND external_id = $2`, projectID, externalID)
	return scanResource(row)
}

func (s *PostgresStore) QueryResources(ctx context.Context, q ResourceQuery) ([]*ObservedResource, error) {
	var args []any
	var clauses []string
	argN := 1

	clauses = append(clauses, fmt.Sprintf("project_id = $%d", argN))
	args = append(args, q.ProjectID)
	argN++

	if q.ResourceType != "" {
		clauses = append(clauses, fmt.Sprintf("resource_type = $%d", argN))
		args = append(args, string(q.ResourceType))
		argN++
	}
	if q.Environment != "" {
		clauses = append(clauses, fmt.Sprintf("environment = $%d", argN))
		args = append(args, q.Environment)
		argN++
	}
	// JSONB property filters.
	for k, v := range q.Filter {
		clauses = append(clauses, fmt.Sprintf("properties->>$%d = $%d", argN, argN+1))
		args = append(args, k, v)
		argN += 2
	}

	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, project_id, resource_type, environment, name, external_id, provider, region,
		       properties, declared_state, observed_at, source, created_at, updated_at
		FROM observed_resources
		WHERE %s
		ORDER BY observed_at DESC
		LIMIT %d`, strings.Join(clauses, " AND "), limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResources(rows)
}

func (s *PostgresStore) DeleteResource(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM observed_resources WHERE id = $1`, id)
	return err
}

// --------------------------------------------------------------------------
// Staleness
// --------------------------------------------------------------------------

func (s *PostgresStore) GetStaleResources(ctx context.Context, projectID string, resourceType ResourceType, environment string, threshold time.Duration) ([]*ObservedResource, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	var args []any
	var clauses []string
	argN := 1

	clauses = append(clauses, fmt.Sprintf("project_id = $%d", argN))
	args = append(args, projectID)
	argN++

	clauses = append(clauses, fmt.Sprintf("observed_at < $%d", argN))
	args = append(args, cutoff)
	argN++

	if resourceType != "" {
		clauses = append(clauses, fmt.Sprintf("resource_type = $%d", argN))
		args = append(args, string(resourceType))
		argN++
	}
	if environment != "" {
		clauses = append(clauses, fmt.Sprintf("environment = $%d", argN))
		args = append(args, environment)
		argN++
	}

	query := fmt.Sprintf(`
		SELECT id, project_id, resource_type, environment, name, external_id, provider, region,
		       properties, declared_state, observed_at, source, created_at, updated_at
		FROM observed_resources
		WHERE %s
		ORDER BY observed_at ASC`, strings.Join(clauses, " AND "))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResources(rows)
}

func (s *PostgresStore) CountResources(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM observed_resources WHERE project_id = $1`, projectID).Scan(&count)
	return count, err
}

// --------------------------------------------------------------------------
// Changes
// --------------------------------------------------------------------------

func (s *PostgresStore) CreateChange(ctx context.Context, c *StateChange) error {
	beforeJSON, _ := json.Marshal(c.BeforeState)
	afterJSON, _ := json.Marshal(c.AfterState)
	diffJSON, _ := json.Marshal(c.Diff)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO state_changes (id, project_id, resource_id, change_type, before_state, after_state, diff, detected_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
		c.ID, c.ProjectID, c.ResourceID, string(c.ChangeType), beforeJSON, afterJSON, diffJSON, c.DetectedAt,
	)
	return err
}

func (s *PostgresStore) GetChange(ctx context.Context, id string) (*StateChange, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, resource_id, change_type, before_state, after_state, diff, detected_at, created_at
		FROM state_changes WHERE id = $1`, id)
	return scanChange(row)
}

func (s *PostgresStore) ListChanges(ctx context.Context, projectID string, since time.Time, limit int) ([]*StateChange, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, resource_id, change_type, before_state, after_state, diff, detected_at, created_at
		FROM state_changes
		WHERE project_id = $1 AND detected_at >= $2
		ORDER BY detected_at DESC LIMIT $3`, projectID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChanges(rows)
}

func (s *PostgresStore) ListChangesByResource(ctx context.Context, resourceID string, limit int) ([]*StateChange, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, resource_id, change_type, before_state, after_state, diff, detected_at, created_at
		FROM state_changes
		WHERE resource_id = $1
		ORDER BY detected_at DESC LIMIT $2`, resourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChanges(rows)
}

// --------------------------------------------------------------------------
// Attribution
// --------------------------------------------------------------------------

func (s *PostgresStore) CreateAttribution(ctx context.Context, a *StateAttribution) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO state_attributions (id, project_id, change_id, resource_id, ticket_id, actor, actor_type, confidence, rationale, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
		a.ID, a.ProjectID, a.ChangeID, a.ResourceID,
		nullIfEmpty(a.TicketID), a.Actor, string(a.ActorType), a.Confidence, a.Rationale,
	)
	return err
}

func (s *PostgresStore) GetAttribution(ctx context.Context, id string) (*StateAttribution, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, change_id, resource_id, COALESCE(ticket_id, ''), actor, actor_type, confidence, rationale, created_at
		FROM state_attributions WHERE id = $1`, id)
	return scanAttribution(row)
}

func (s *PostgresStore) ListUnattributed(ctx context.Context, projectID string, limit int) ([]*StateAttribution, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, change_id, resource_id, COALESCE(ticket_id, ''), actor, actor_type, confidence, rationale, created_at
		FROM state_attributions
		WHERE project_id = $1 AND actor_type = 'unattributed'
		ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttributions(rows)
}

func (s *PostgresStore) ListAttributionsByResource(ctx context.Context, resourceID string) ([]*StateAttribution, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, change_id, resource_id, COALESCE(ticket_id, ''), actor, actor_type, confidence, rationale, created_at
		FROM state_attributions
		WHERE resource_id = $1
		ORDER BY created_at DESC`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttributions(rows)
}

func (s *PostgresStore) ListAttributionsByTicket(ctx context.Context, ticketID string) ([]*StateAttribution, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, change_id, resource_id, COALESCE(ticket_id, ''), actor, actor_type, confidence, rationale, created_at
		FROM state_attributions
		WHERE ticket_id = $1
		ORDER BY created_at DESC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttributions(rows)
}

// --------------------------------------------------------------------------
// Summary
// --------------------------------------------------------------------------

func (s *PostgresStore) GetSummary(ctx context.Context, projectID string, stalenessThreshold time.Duration) (*StateSummary, error) {
	summary := &StateSummary{
		ProjectID:       projectID,
		ResourcesByType: make(map[string]int),
		ResourcesByEnv:  make(map[string]int),
		StalenessDistribution: make(map[string]int),
	}

	// Total + by type.
	rows, err := s.pool.Query(ctx, `
		SELECT resource_type, COUNT(*) FROM observed_resources
		WHERE project_id = $1 GROUP BY resource_type`, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rt string
		var count int
		if err := rows.Scan(&rt, &count); err != nil {
			rows.Close()
			return nil, err
		}
		summary.ResourcesByType[rt] = count
		summary.TotalResources += count
	}
	rows.Close()

	// By environment.
	rows, err = s.pool.Query(ctx, `
		SELECT environment, COUNT(*) FROM observed_resources
		WHERE project_id = $1 GROUP BY environment`, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var env string
		var count int
		if err := rows.Scan(&env, &count); err != nil {
			rows.Close()
			return nil, err
		}
		summary.ResourcesByEnv[env] = count
	}
	rows.Close()

	// Stale count.
	cutoff := time.Now().UTC().Add(-stalenessThreshold)
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM observed_resources
		WHERE project_id = $1 AND observed_at < $2`, projectID, cutoff).Scan(&summary.StaleCount)
	if err != nil {
		return nil, err
	}

	// Drift count (resources with declared_state that differs from properties).
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM observed_resources
		WHERE project_id = $1 AND declared_state IS NOT NULL AND declared_state != properties`,
		projectID).Scan(&summary.DriftCount)
	if err != nil {
		return nil, err
	}

	// Unattributed count.
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM state_attributions
		WHERE project_id = $1 AND actor_type = 'unattributed'`,
		projectID).Scan(&summary.UnattributedCount)
	if err != nil {
		return nil, err
	}

	// Last full scan.
	var lastScan *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT MAX(executed_at) FROM steampipe_snapshots WHERE project_id = $1`,
		projectID).Scan(&lastScan)
	if err == nil && lastScan != nil {
		summary.LastFullScan = lastScan
	}

	// Staleness distribution (buckets: <1m, 1-5m, 5-15m, 15-60m, >60m).
	sRows, err := s.pool.Query(ctx, `
		SELECT
			CASE
				WHEN now() - observed_at < interval '1 minute' THEN 'fresh_under_1m'
				WHEN now() - observed_at < interval '5 minutes' THEN 'recent_1_5m'
				WHEN now() - observed_at < interval '15 minutes' THEN 'aging_5_15m'
				WHEN now() - observed_at < interval '60 minutes' THEN 'stale_15_60m'
				ELSE 'very_stale_over_60m'
			END as bucket,
			COUNT(*)
		FROM observed_resources WHERE project_id = $1
		GROUP BY bucket`, projectID)
	if err != nil {
		return nil, err
	}
	for sRows.Next() {
		var bucket string
		var count int
		if err := sRows.Scan(&bucket, &count); err != nil {
			sRows.Close()
			return nil, err
		}
		summary.StalenessDistribution[bucket] = count
	}
	sRows.Close()

	return summary, nil
}

// --------------------------------------------------------------------------
// Steampipe Snapshots
// --------------------------------------------------------------------------

func (s *PostgresStore) SaveSnapshot(ctx context.Context, snap *SteampipeSnapshot) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO steampipe_snapshots (id, project_id, query, resource_type, environment, result_hash, row_count, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		snap.ID, snap.ProjectID, snap.Query, snap.ResourceType, snap.Environment, snap.ResultHash, snap.RowCount, snap.ExecutedAt,
	)
	return err
}

func (s *PostgresStore) GetLatestSnapshot(ctx context.Context, projectID, resourceType, environment string) (*SteampipeSnapshot, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, query, resource_type, environment, result_hash, row_count, executed_at
		FROM steampipe_snapshots
		WHERE project_id = $1 AND resource_type = $2 AND environment = $3
		ORDER BY executed_at DESC LIMIT 1`, projectID, resourceType, environment)

	snap := &SteampipeSnapshot{}
	err := row.Scan(&snap.ID, &snap.ProjectID, &snap.Query, &snap.ResourceType, &snap.Environment, &snap.ResultHash, &snap.RowCount, &snap.ExecutedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return snap, err
}

// --------------------------------------------------------------------------
// Scan helpers
// --------------------------------------------------------------------------

func scanResource(row pgx.Row) (*ObservedResource, error) {
	r := &ObservedResource{}
	var propsJSON, declaredJSON []byte
	err := row.Scan(&r.ID, &r.ProjectID, &r.ResourceType, &r.Environment, &r.Name,
		&r.ExternalID, &r.Provider, &r.Region, &propsJSON, &declaredJSON,
		&r.ObservedAt, &r.Source, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if propsJSON != nil {
		_ = json.Unmarshal(propsJSON, &r.Properties)
	}
	if r.Properties == nil {
		r.Properties = make(map[string]any)
	}
	if declaredJSON != nil {
		_ = json.Unmarshal(declaredJSON, &r.DeclaredState)
	}
	return r, nil
}

func scanResources(rows pgx.Rows) ([]*ObservedResource, error) {
	var result []*ObservedResource
	for rows.Next() {
		r := &ObservedResource{}
		var propsJSON, declaredJSON []byte
		err := rows.Scan(&r.ID, &r.ProjectID, &r.ResourceType, &r.Environment, &r.Name,
			&r.ExternalID, &r.Provider, &r.Region, &propsJSON, &declaredJSON,
			&r.ObservedAt, &r.Source, &r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if propsJSON != nil {
			_ = json.Unmarshal(propsJSON, &r.Properties)
		}
		if r.Properties == nil {
			r.Properties = make(map[string]any)
		}
		if declaredJSON != nil {
			_ = json.Unmarshal(declaredJSON, &r.DeclaredState)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func scanChange(row pgx.Row) (*StateChange, error) {
	c := &StateChange{}
	var beforeJSON, afterJSON, diffJSON []byte
	err := row.Scan(&c.ID, &c.ProjectID, &c.ResourceID, &c.ChangeType,
		&beforeJSON, &afterJSON, &diffJSON, &c.DetectedAt, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrChangeNotFound
	}
	if err != nil {
		return nil, err
	}
	if beforeJSON != nil {
		_ = json.Unmarshal(beforeJSON, &c.BeforeState)
	}
	if afterJSON != nil {
		_ = json.Unmarshal(afterJSON, &c.AfterState)
	}
	if diffJSON != nil {
		_ = json.Unmarshal(diffJSON, &c.Diff)
	}
	return c, nil
}

func scanChanges(rows pgx.Rows) ([]*StateChange, error) {
	var result []*StateChange
	for rows.Next() {
		c := &StateChange{}
		var beforeJSON, afterJSON, diffJSON []byte
		err := rows.Scan(&c.ID, &c.ProjectID, &c.ResourceID, &c.ChangeType,
			&beforeJSON, &afterJSON, &diffJSON, &c.DetectedAt, &c.CreatedAt)
		if err != nil {
			return nil, err
		}
		if beforeJSON != nil {
			_ = json.Unmarshal(beforeJSON, &c.BeforeState)
		}
		if afterJSON != nil {
			_ = json.Unmarshal(afterJSON, &c.AfterState)
		}
		if diffJSON != nil {
			_ = json.Unmarshal(diffJSON, &c.Diff)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func scanAttribution(row pgx.Row) (*StateAttribution, error) {
	a := &StateAttribution{}
	err := row.Scan(&a.ID, &a.ProjectID, &a.ChangeID, &a.ResourceID,
		&a.TicketID, &a.Actor, &a.ActorType, &a.Confidence, &a.Rationale, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrAttributionNotFound
	}
	return a, err
}

func scanAttributions(rows pgx.Rows) ([]*StateAttribution, error) {
	var result []*StateAttribution
	for rows.Next() {
		a := &StateAttribution{}
		err := rows.Scan(&a.ID, &a.ProjectID, &a.ChangeID, &a.ResourceID,
			&a.TicketID, &a.Actor, &a.ActorType, &a.Confidence, &a.Rationale, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
