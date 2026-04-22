package project

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRepoNotFound is returned when a repository lookup finds no rows.
var ErrRepoNotFound = errors.New("repository not found")

// ErrDuplicateAlias is returned when a repo alias already exists for the project.
var ErrDuplicateAlias = errors.New("repository alias already exists for this project")

// RepositoryStore persists project repositories.
type RepositoryStore struct {
	pool *pgxpool.Pool
}

// NewRepositoryStore returns a new RepositoryStore.
func NewRepositoryStore(pool *pgxpool.Pool) *RepositoryStore {
	return &RepositoryStore{pool: pool}
}

// Create inserts a project repository.
func (s *RepositoryStore) Create(ctx context.Context, r *Repository) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO project_repositories (id, project_id, alias, repo_url, default_branch, is_primary, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, r.ProjectID, r.Alias, r.RepoURL, r.DefaultBranch, r.IsPrimary, r.CreatedAt)
	if err != nil {
		// Check for unique constraint violation on alias.
		if isUniqueViolation(err) {
			return ErrDuplicateAlias
		}
		return err
	}
	return nil
}

// GetByAlias returns a repository by project ID and alias.
func (s *RepositoryStore) GetByAlias(ctx context.Context, projectID, alias string) (*Repository, error) {
	var r Repository
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, alias, repo_url, default_branch, is_primary, created_at
		 FROM project_repositories WHERE project_id = $1 AND alias = $2`,
		projectID, alias).
		Scan(&r.ID, &r.ProjectID, &r.Alias, &r.RepoURL, &r.DefaultBranch, &r.IsPrimary, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepoNotFound
		}
		return nil, err
	}
	return &r, nil
}

// GetByID returns a repository by ID.
func (s *RepositoryStore) GetByID(ctx context.Context, id string) (*Repository, error) {
	var r Repository
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, alias, repo_url, default_branch, is_primary, created_at
		 FROM project_repositories WHERE id = $1`, id).
		Scan(&r.ID, &r.ProjectID, &r.Alias, &r.RepoURL, &r.DefaultBranch, &r.IsPrimary, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepoNotFound
		}
		return nil, err
	}
	return &r, nil
}

// GetPrimary returns the primary repository for a project.
func (s *RepositoryStore) GetPrimary(ctx context.Context, projectID string) (*Repository, error) {
	var r Repository
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, alias, repo_url, default_branch, is_primary, created_at
		 FROM project_repositories WHERE project_id = $1 AND is_primary = true`, projectID).
		Scan(&r.ID, &r.ProjectID, &r.Alias, &r.RepoURL, &r.DefaultBranch, &r.IsPrimary, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRepoNotFound
		}
		return nil, err
	}
	return &r, nil
}

// ListByProject returns all repositories for a project, ordered by alias.
func (s *RepositoryStore) ListByProject(ctx context.Context, projectID string) ([]Repository, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, alias, repo_url, default_branch, is_primary, created_at
		 FROM project_repositories WHERE project_id = $1 ORDER BY is_primary DESC, alias`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Repository
	for rows.Next() {
		var r Repository
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Alias, &r.RepoURL, &r.DefaultBranch, &r.IsPrimary, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// Delete removes a repository by project ID and alias.
func (s *RepositoryStore) Delete(ctx context.Context, projectID, alias string) error {
	res, err := s.pool.Exec(ctx,
		`DELETE FROM project_repositories WHERE project_id = $1 AND alias = $2`, projectID, alias)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrRepoNotFound
	}
	return nil
}

// UpdateRepoURL updates the repo_url and default_branch of a repository.
func (s *RepositoryStore) UpdateRepoURL(ctx context.Context, projectID, alias, repoURL, defaultBranch string) error {
	res, err := s.pool.Exec(ctx,
		`UPDATE project_repositories SET repo_url = $1, default_branch = $2
		 WHERE project_id = $3 AND alias = $4`, repoURL, defaultBranch, projectID, alias)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrRepoNotFound
	}
	return nil
}

// isUniqueViolation checks for Postgres unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return errors.As(err, new(interface{ SQLState() string })) &&
		err.(interface{ SQLState() string }).SQLState() == "23505"
}
