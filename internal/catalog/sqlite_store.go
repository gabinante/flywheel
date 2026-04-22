package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SQLiteStore implements CatalogStore using SQLite for embedded mode.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore returns a new SQLite-backed catalog store.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// ────────────────────────────────────────────────────────────────────────────
// Entity CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateEntity(_ context.Context, e *Entity) error {
	labelsJSON, _ := json.Marshal(e.Labels)
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.db.Exec(
		`INSERT INTO catalog_entities (id, project_id, type, name, description, labels, metadata, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, string(e.Type), e.Name, e.Description,
		string(labelsJSON), string(metaJSON), string(e.Source),
		e.CreatedAt.UTC().Format(time.RFC3339Nano), e.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) GetEntityByID(_ context.Context, projectID, id string) (*Entity, error) {
	var e Entity
	var labelsStr, metaStr, entityType, source, createdStr, updatedStr string
	err := s.db.QueryRow(
		`SELECT id, project_id, type, name, description, labels, metadata, source, created_at, updated_at
		 FROM catalog_entities WHERE project_id = ? AND id = ?`, projectID, id).
		Scan(&e.ID, &e.ProjectID, &entityType, &e.Name, &e.Description,
			&labelsStr, &metaStr, &source, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEntityNotFound, err)
	}
	e.Type = EntityType(entityType)
	e.Source = Source(source)
	_ = json.Unmarshal([]byte(labelsStr), &e.Labels)
	_ = json.Unmarshal([]byte(metaStr), &e.Metadata)
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &e, nil
}

func (s *SQLiteStore) ListEntities(_ context.Context, projectID, entityType, label string, limit int) ([]*Entity, error) {
	q := `SELECT id, project_id, type, name, description, labels, metadata, source, created_at, updated_at
	      FROM catalog_entities WHERE project_id = ?`
	args := []any{projectID}

	if entityType != "" {
		q += " AND type = ?"
		args = append(args, entityType)
	}
	if label != "" {
		// SQLite JSON: check if label key exists in labels object.
		q += " AND json_extract(labels, '$.' || ?) IS NOT NULL"
		args = append(args, label)
	}
	q += " ORDER BY name LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		var labelsStr, metaStr, entType, src, createdStr, updatedStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &entType, &e.Name, &e.Description,
			&labelsStr, &metaStr, &src, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		e.Type = EntityType(entType)
		e.Source = Source(src)
		_ = json.Unmarshal([]byte(labelsStr), &e.Labels)
		_ = json.Unmarshal([]byte(metaStr), &e.Metadata)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		entities = append(entities, &e)
	}
	return entities, rows.Err()
}

func (s *SQLiteStore) UpdateEntity(_ context.Context, e *Entity) error {
	labelsJSON, _ := json.Marshal(e.Labels)
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.db.Exec(
		`UPDATE catalog_entities SET name = ?, description = ?, labels = ?, metadata = ?, updated_at = ?
		 WHERE project_id = ? AND id = ?`,
		e.Name, e.Description, string(labelsJSON), string(metaJSON),
		e.UpdatedAt.UTC().Format(time.RFC3339Nano), e.ProjectID, e.ID)
	return err
}

func (s *SQLiteStore) DeleteEntity(_ context.Context, projectID, id string) error {
	// Delete edges first.
	_, _ = s.db.Exec(`DELETE FROM catalog_edges WHERE project_id = ? AND (from_id = ? OR to_id = ?)`, projectID, id, id)
	// Delete deployments referencing this entity.
	_, _ = s.db.Exec(`DELETE FROM catalog_deployments WHERE service_id = ? OR environment_id = ?`, id, id)
	_, err := s.db.Exec(`DELETE FROM catalog_entities WHERE project_id = ? AND id = ?`, projectID, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Edge CRUD
// ────────────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateEdge(_ context.Context, e *Edge) error {
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.db.Exec(
		`INSERT INTO catalog_edges (id, project_id, from_id, to_id, type, metadata, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.FromID, e.ToID, string(e.Type), string(metaJSON),
		string(e.Source), e.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) GetEdgeByID(_ context.Context, projectID, id string) (*Edge, error) {
	var e Edge
	var metaStr, edgeType, source, createdStr string
	err := s.db.QueryRow(
		`SELECT id, project_id, from_id, to_id, type, metadata, source, created_at
		 FROM catalog_edges WHERE project_id = ? AND id = ?`, projectID, id).
		Scan(&e.ID, &e.ProjectID, &e.FromID, &e.ToID, &edgeType, &metaStr, &source, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEdgeNotFound, err)
	}
	e.Type = EdgeType(edgeType)
	e.Source = Source(source)
	_ = json.Unmarshal([]byte(metaStr), &e.Metadata)
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	return &e, nil
}

func (s *SQLiteStore) ListEdges(_ context.Context, projectID, entityID, edgeType, direction string) ([]*Edge, error) {
	q := `SELECT id, project_id, from_id, to_id, type, metadata, source, created_at
	      FROM catalog_edges WHERE project_id = ?`
	args := []any{projectID}

	switch direction {
	case "outgoing":
		q += " AND from_id = ?"
		args = append(args, entityID)
	case "incoming":
		q += " AND to_id = ?"
		args = append(args, entityID)
	default: // "both"
		q += " AND (from_id = ? OR to_id = ?)"
		args = append(args, entityID, entityID)
	}

	if edgeType != "" {
		q += " AND type = ?"
		args = append(args, edgeType)
	}
	q += " ORDER BY created_at"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		var e Edge
		var metaStr, eType, src, createdStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.FromID, &e.ToID, &eType, &metaStr, &src, &createdStr); err != nil {
			return nil, err
		}
		e.Type = EdgeType(eType)
		e.Source = Source(src)
		_ = json.Unmarshal([]byte(metaStr), &e.Metadata)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		edges = append(edges, &e)
	}
	return edges, rows.Err()
}

func (s *SQLiteStore) DeleteEdge(_ context.Context, projectID, id string) error {
	_, err := s.db.Exec(`DELETE FROM catalog_edges WHERE project_id = ? AND id = ?`, projectID, id)
	return err
}

// ────────────────────────────────────────────────────────────────────────────
// Deployment matrix
// ────────────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) UpsertDeployment(_ context.Context, d *DeploymentEntry) error {
	if d.ObservedAt.IsZero() {
		d.ObservedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(
		`INSERT INTO catalog_deployments (service_id, environment_id, project_id, version, source, observed_at)
		 VALUES (?, ?, (SELECT project_id FROM catalog_entities WHERE id = ?), ?, ?, ?)
		 ON CONFLICT (service_id, environment_id) DO UPDATE SET version = ?, source = ?, observed_at = ?`,
		d.ServiceID, d.EnvironmentID, d.ServiceID, d.Version, string(d.Source),
		d.ObservedAt.UTC().Format(time.RFC3339Nano),
		d.Version, string(d.Source), d.ObservedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) ListDeployments(_ context.Context, projectID, serviceID string) ([]*DeploymentEntry, error) {
	q := `SELECT d.service_id, s.name, d.environment_id, e.name, d.version, d.source, d.observed_at
	      FROM catalog_deployments d
	      JOIN catalog_entities s ON s.id = d.service_id
	      JOIN catalog_entities e ON e.id = d.environment_id
	      WHERE d.project_id = ?`
	args := []any{projectID}
	if serviceID != "" {
		q += " AND d.service_id = ?"
		args = append(args, serviceID)
	}
	q += " ORDER BY s.name, e.name"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*DeploymentEntry
	for rows.Next() {
		var d DeploymentEntry
		var src, observedStr string
		if err := rows.Scan(&d.ServiceID, &d.ServiceName, &d.EnvironmentID, &d.EnvName, &d.Version, &src, &observedStr); err != nil {
			return nil, err
		}
		d.Source = Source(src)
		d.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedStr)
		deps = append(deps, &d)
	}
	return deps, rows.Err()
}
