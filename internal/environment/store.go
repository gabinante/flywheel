package environment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists environments in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a new environment. ID must already be set.
func (s *Store) Create(ctx context.Context, env *Environment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO environments (id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		env.ID, env.ProjectID, env.Name, env.Slug,
		string(env.Infrastructure), string(env.DataTenancy), string(env.IntegrationMode),
		env.IsDefault, env.CreatedAt, env.UpdatedAt)
	return err
}

// GetByID returns an environment by its ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Environment, error) {
	var env Environment
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE id = $1`, id).
		Scan(&env.ID, &env.ProjectID, &env.Name, &env.Slug,
			&env.Infrastructure, &env.DataTenancy, &env.IntegrationMode,
			&env.IsDefault, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &env, nil
}

// GetBySlug returns an environment by project ID and slug.
func (s *Store) GetBySlug(ctx context.Context, projectID, slug string) (*Environment, error) {
	var env Environment
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE project_id = $1 AND slug = $2`, projectID, slug).
		Scan(&env.ID, &env.ProjectID, &env.Name, &env.Slug,
			&env.Infrastructure, &env.DataTenancy, &env.IntegrationMode,
			&env.IsDefault, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &env, nil
}

// ListByProject returns all environments for a project, ordered by is_default DESC then name ASC.
func (s *Store) ListByProject(ctx context.Context, projectID string) ([]*Environment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE project_id = $1 ORDER BY is_default DESC, name ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Environment
	for rows.Next() {
		var env Environment
		if err := rows.Scan(&env.ID, &env.ProjectID, &env.Name, &env.Slug,
			&env.Infrastructure, &env.DataTenancy, &env.IntegrationMode,
			&env.IsDefault, &env.CreatedAt, &env.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &env)
	}
	return list, rows.Err()
}

// CountByProject returns the number of environments for a project.
func (s *Store) CountByProject(ctx context.Context, projectID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM environments WHERE project_id = $1`, projectID).Scan(&count)
	return count, err
}

// Update updates an environment's mutable fields.
func (s *Store) Update(ctx context.Context, env *Environment) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE environments SET name = $1, slug = $2, infrastructure = $3, data_tenancy = $4, integration_mode = $5, is_default = $6, updated_at = $7
		 WHERE id = $8`,
		env.Name, env.Slug, string(env.Infrastructure), string(env.DataTenancy), string(env.IntegrationMode),
		env.IsDefault, time.Now().UTC(), env.ID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an environment by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM environments WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearDefault sets is_default=false for all environments in a project.
func (s *Store) ClearDefault(ctx context.Context, projectID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE environments SET is_default = false WHERE project_id = $1`, projectID)
	return err
}
