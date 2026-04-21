package project

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RepositoryStoreInterface is the persistence interface for project repositories.
type RepositoryStoreInterface interface {
	Create(ctx context.Context, r *Repository) error
	GetByAlias(ctx context.Context, projectID, alias string) (*Repository, error)
	GetByID(ctx context.Context, id string) (*Repository, error)
	GetPrimary(ctx context.Context, projectID string) (*Repository, error)
	ListByProject(ctx context.Context, projectID string) ([]Repository, error)
	Delete(ctx context.Context, projectID, alias string) error
	UpdateRepoURL(ctx context.Context, projectID, alias, repoURL, defaultBranch string) error
}

// RepositoryService provides project repository operations.
type RepositoryService struct {
	store RepositoryStoreInterface
}

// NewRepositoryService returns a new RepositoryService.
func NewRepositoryService(store RepositoryStoreInterface) *RepositoryService {
	return &RepositoryService{store: store}
}

// AddRepository adds a repository to a project. Alias must be unique per project.
// If isPrimary and a primary already exists, returns an error.
func (s *RepositoryService) AddRepository(ctx context.Context, projectID, alias, repoURL, defaultBranch string, isPrimary bool) (*Repository, error) {
	if alias == "" {
		return nil, fmt.Errorf("alias is required")
	}
	if repoURL == "" {
		return nil, fmt.Errorf("repo_url is required")
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	r := &Repository{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     projectID,
		Alias:         alias,
		RepoURL:       repoURL,
		DefaultBranch: defaultBranch,
		IsPrimary:     isPrimary,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.store.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// GetRepository returns a repository by project ID and alias.
func (s *RepositoryService) GetRepository(ctx context.Context, projectID, alias string) (*Repository, error) {
	return s.store.GetByAlias(ctx, projectID, alias)
}

// GetPrimaryRepository returns the primary repository for a project.
func (s *RepositoryService) GetPrimaryRepository(ctx context.Context, projectID string) (*Repository, error) {
	return s.store.GetPrimary(ctx, projectID)
}

// ListRepositories returns all repositories for a project.
func (s *RepositoryService) ListRepositories(ctx context.Context, projectID string) ([]Repository, error) {
	return s.store.ListByProject(ctx, projectID)
}

// RemoveRepository removes a repository by alias. Cannot remove the primary repo
// if other repos reference it.
func (s *RepositoryService) RemoveRepository(ctx context.Context, projectID, alias string) error {
	return s.store.Delete(ctx, projectID, alias)
}

// ResolveRepoForTicket resolves the repository URL and default branch for a ticket.
// If targetRepo is empty, returns the primary repo. If targetRepo is set, looks up by alias.
// Falls back to the project's legacy repo_url if no project_repositories rows exist.
func (s *RepositoryService) ResolveRepoForTicket(ctx context.Context, projectID, targetRepo string) (*Repository, error) {
	if targetRepo != "" {
		return s.store.GetByAlias(ctx, projectID, targetRepo)
	}
	// Try primary repo from project_repositories table.
	r, err := s.store.GetPrimary(ctx, projectID)
	if err == nil {
		return r, nil
	}
	// No primary repo found — caller should fall back to project.RepoURL.
	return nil, ErrRepoNotFound
}
