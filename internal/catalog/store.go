package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements CatalogStore using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a new PostgreSQL-backed catalog store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// ────────────────────────────────────────────────────────────────────────────
// Entity CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateEntity(ctx context.Context, e *Entity) error {
	labelsJSON, _ := json.Marshal(e.Labels)
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO catalog_entities (id, project_id, type, name, description, labels, metadata, source, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		e.ID, e.ProjectID, string(e.Type), e.Name, e.Description,
		labelsJSON, metaJSON, string(e.Source), e.CreatedAt, e.UpdatedAt)
	return err
}

func (s *PostgresStore) GetEntityByID(ctx context.Context, projectID, id string) (*Entity, error) {
	var e Entity
	var labelsJSON, metaJSON []byte
	var entityType, source string
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, type, name, description, labels, metadata, source, created_at, updated_at
		 FROM catalog_entities WHERE project_id = $1 AND id = $2`, projectID, id).
		Scan(&e.ID, &e.ProjectID, &entityType, &e.Name, &e.Description,
			&labelsJSON, &metaJSON, &source, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEntityNotFound, err)
	}
	e.Type = EntityType(entityType)
	e.Source = Source(source)
	_ = json.Unmarshal(labelsJSON, &e.Labels)
	_ = json.Unmarshal(metaJSON, &e.Metadata)
	return &e, nil
}

func (s *PostgresStore) ListEntities(ctx context.Context, projectID, entityType, label string, limit int) ([]*Entity, error) {
	q := `SELECT id, project_id, type, name, description, labels, metadata, source, created_at, updated_at
	      FROM catalog_entities WHERE project_id = $1`
	args := []any{projectID}
	idx := 2

	if entityType != "" {
		q += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, entityType)
		idx++
	}
	if label != "" {
		q += fmt.Sprintf(" AND labels ? $%d", idx)
		args = append(args, label)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY name LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		var labelsJSON, metaJSON []byte
		var entityT, src string
		if err := rows.Scan(&e.ID, &e.ProjectID, &entityT, &e.Name, &e.Description,
			&labelsJSON, &metaJSON, &src, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Type = EntityType(entityT)
		e.Source = Source(src)
		_ = json.Unmarshal(labelsJSON, &e.Labels)
		_ = json.Unmarshal(metaJSON, &e.Metadata)
		entities = append(entities, &e)
	}
	return entities, rows.Err()
}

func (s *PostgresStore) UpdateEntity(ctx context.Context, e *Entity) error {
	labelsJSON, _ := json.Marshal(e.Labels)
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.pool.Exec(ctx,
		`UPDATE catalog_entities SET name = $1, description = $2, labels = $3, metadata = $4, updated_at = $5
		 WHERE project_id = $6 AND id = $7`,
		e.Name, e.Description, labelsJSON, metaJSON, e.UpdatedAt, e.ProjectID, e.ID)
	return err
}

func (s *PostgresStore) DeleteEntity(ctx context.Context, projectID, id string) error {
	// Delete edges first (referential integrity).
	_, _ = s.pool.Exec(ctx,
		`DELETE FROM catalog_edges WHERE project_id = $1 AND (from_id = $2 OR to_id = $2)`, projectID, id)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM catalog_entities WHERE project_id = $1 AND id = $2`, projectID, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Edge CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateEdge(ctx context.Context, e *Edge) error {
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO catalog_edges (id, project_id, from_id, to_id, type, metadata, source, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.ProjectID, e.FromID, e.ToID, string(e.Type), metaJSON, string(e.Source), e.CreatedAt)
	return err
}

func (s *PostgresStore) GetEdgeByID(ctx context.Context, projectID, id string) (*Edge, error) {
	var e Edge
	var metaJSON []byte
	var edgeType, source string
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, from_id, to_id, type, metadata, source, created_at
		 FROM catalog_edges WHERE project_id = $1 AND id = $2`, projectID, id).
		Scan(&e.ID, &e.ProjectID, &e.FromID, &e.ToID, &edgeType, &metaJSON, &source, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEdgeNotFound, err)
	}
	e.Type = EdgeType(edgeType)
	e.Source = Source(source)
	_ = json.Unmarshal(metaJSON, &e.Metadata)
	return &e, nil
}

func (s *PostgresStore) ListEdges(ctx context.Context, projectID, entityID, edgeType, direction string) ([]*Edge, error) {
	q := `SELECT id, project_id, from_id, to_id, type, metadata, source, created_at
	      FROM catalog_edges WHERE project_id = $1`
	args := []any{projectID}
	idx := 2

	switch direction {
	case "outgoing":
		q += fmt.Sprintf(" AND from_id = $%d", idx)
		args = append(args, entityID)
		idx++
	case "incoming":
		q += fmt.Sprintf(" AND to_id = $%d", idx)
		args = append(args, entityID)
		idx++
	default: // "both"
		q += fmt.Sprintf(" AND (from_id = $%d OR to_id = $%d)", idx, idx)
		args = append(args, entityID)
		idx++
	}

	if edgeType != "" {
		q += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, edgeType)
	}
	q += " ORDER BY created_at"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		var e Edge
		var metaJSON []byte
		var eType, src string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.FromID, &e.ToID, &eType, &metaJSON, &src, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Type = EdgeType(eType)
		e.Source = Source(src)
		_ = json.Unmarshal(metaJSON, &e.Metadata)
		edges = append(edges, &e)
	}
	return edges, rows.Err()
}

func (s *PostgresStore) DeleteEdge(ctx context.Context, projectID, id string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM catalog_edges WHERE project_id = $1 AND id = $2`, projectID, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Deployment matrix
// ────────────────────────────────────────────────────────────────────────────

func (s *PostgresStore) UpsertDeployment(ctx context.Context, d *DeploymentEntry) error {
	if d.ObservedAt.IsZero() {
		d.ObservedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO catalog_deployments (service_id, environment_id, project_id, version, source, observed_at)
		 VALUES ($1, $2, (SELECT project_id FROM catalog_entities WHERE id = $1), $3, $4, $5)
		 ON CONFLICT (service_id, environment_id) DO UPDATE SET version = $3, source = $4, observed_at = $5`,
		d.ServiceID, d.EnvironmentID, d.Version, string(d.Source), d.ObservedAt)
	return err
}

func (s *PostgresStore) ListDeployments(ctx context.Context, projectID, serviceID string) ([]*DeploymentEntry, error) {
	q := `SELECT d.service_id, s.name, d.environment_id, e.name, d.version, d.source, d.observed_at
	      FROM catalog_deployments d
	      JOIN catalog_entities s ON s.id = d.service_id
	      JOIN catalog_entities e ON e.id = d.environment_id
	      WHERE s.project_id = $1`
	args := []any{projectID}
	if serviceID != "" {
		q += " AND d.service_id = $2"
		args = append(args, serviceID)
	}
	q += " ORDER BY s.name, e.name"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*DeploymentEntry
	for rows.Next() {
		var d DeploymentEntry
		var src string
		if err := rows.Scan(&d.ServiceID, &d.ServiceName, &d.EnvironmentID, &d.EnvName, &d.Version, &src, &d.ObservedAt); err != nil {
			return nil, err
		}
		d.Source = Source(src)
		deps = append(deps, &d)
	}
	return deps, rows.Err()
}
