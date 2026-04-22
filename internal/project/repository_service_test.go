package project

import (
	"context"
	"errors"
	"testing"
)

// mockRepoStore implements RepositoryStoreInterface for testing.
type mockRepoStore struct {
	repos     map[string]*Repository // keyed by projectID+"/"+alias
	createErr error
}

func newMockRepoStore() *mockRepoStore {
	return &mockRepoStore{repos: make(map[string]*Repository)}
}

func (m *mockRepoStore) Create(_ context.Context, r *Repository) error {
	if m.createErr != nil {
		return m.createErr
	}
	key := r.ProjectID + "/" + r.Alias
	if _, exists := m.repos[key]; exists {
		return ErrDuplicateAlias
	}
	m.repos[key] = r
	return nil
}

func (m *mockRepoStore) GetByAlias(_ context.Context, projectID, alias string) (*Repository, error) {
	key := projectID + "/" + alias
	r, ok := m.repos[key]
	if !ok {
		return nil, ErrRepoNotFound
	}
	return r, nil
}

func (m *mockRepoStore) GetByID(_ context.Context, id string) (*Repository, error) {
	for _, r := range m.repos {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, ErrRepoNotFound
}

func (m *mockRepoStore) GetPrimary(_ context.Context, projectID string) (*Repository, error) {
	for _, r := range m.repos {
		if r.ProjectID == projectID && r.IsPrimary {
			return r, nil
		}
	}
	return nil, ErrRepoNotFound
}

func (m *mockRepoStore) ListByProject(_ context.Context, projectID string) ([]Repository, error) {
	var list []Repository
	for _, r := range m.repos {
		if r.ProjectID == projectID {
			list = append(list, *r)
		}
	}
	return list, nil
}

func (m *mockRepoStore) Delete(_ context.Context, projectID, alias string) error {
	key := projectID + "/" + alias
	if _, ok := m.repos[key]; !ok {
		return ErrRepoNotFound
	}
	delete(m.repos, key)
	return nil
}

func (m *mockRepoStore) UpdateRepoURL(_ context.Context, projectID, alias, repoURL, defaultBranch string) error {
	key := projectID + "/" + alias
	r, ok := m.repos[key]
	if !ok {
		return ErrRepoNotFound
	}
	r.RepoURL = repoURL
	r.DefaultBranch = defaultBranch
	return nil
}

func TestRepositoryService_AddRepository(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	repo, err := svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend.git", "main", true)
	if err != nil {
		t.Fatalf("AddRepository: %v", err)
	}
	if repo.Alias != "backend" {
		t.Errorf("Alias: got %q, want backend", repo.Alias)
	}
	if repo.RepoURL != "https://github.com/org/backend.git" {
		t.Errorf("RepoURL: got %q", repo.RepoURL)
	}
	if !repo.IsPrimary {
		t.Error("IsPrimary: should be true")
	}
	if repo.ID == "" {
		t.Error("ID should be set")
	}
}

func TestRepositoryService_AddRepository_DuplicateAlias(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, err := svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend.git", "main", false)
	if err != nil {
		t.Fatalf("first AddRepository: %v", err)
	}

	_, err = svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend2.git", "main", false)
	if !errors.Is(err, ErrDuplicateAlias) {
		t.Fatalf("expected ErrDuplicateAlias, got: %v", err)
	}
}

func TestRepositoryService_AddRepository_Validation(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, err := svc.AddRepository(ctx, "proj1", "", "https://github.com/org/backend.git", "main", false)
	if err == nil {
		t.Fatal("expected error for empty alias")
	}

	_, err = svc.AddRepository(ctx, "proj1", "backend", "", "main", false)
	if err == nil {
		t.Fatal("expected error for empty repo_url")
	}
}

func TestRepositoryService_GetRepository(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, _ = svc.AddRepository(ctx, "proj1", "frontend", "https://github.com/org/frontend.git", "main", false)

	repo, err := svc.GetRepository(ctx, "proj1", "frontend")
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repo.Alias != "frontend" {
		t.Errorf("Alias: got %q", repo.Alias)
	}
}

func TestRepositoryService_GetRepository_NotFound(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, err := svc.GetRepository(ctx, "proj1", "nonexistent")
	if !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("expected ErrRepoNotFound, got: %v", err)
	}
}

func TestRepositoryService_ListRepositories(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, _ = svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend.git", "main", true)
	_, _ = svc.AddRepository(ctx, "proj1", "frontend", "https://github.com/org/frontend.git", "main", false)
	_, _ = svc.AddRepository(ctx, "proj2", "api", "https://github.com/org/api.git", "main", true)

	repos, err := svc.ListRepositories(ctx, "proj1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestRepositoryService_RemoveRepository(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, _ = svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend.git", "main", false)

	err := svc.RemoveRepository(ctx, "proj1", "backend")
	if err != nil {
		t.Fatalf("RemoveRepository: %v", err)
	}

	_, err = svc.GetRepository(ctx, "proj1", "backend")
	if !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("expected not found after removal")
	}
}

func TestRepositoryService_ResolveRepoForTicket_Primary(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, _ = svc.AddRepository(ctx, "proj1", "main-repo", "https://github.com/org/main.git", "main", true)

	// Empty target_repo should resolve to primary.
	repo, err := svc.ResolveRepoForTicket(ctx, "proj1", "")
	if err != nil {
		t.Fatalf("ResolveRepoForTicket: %v", err)
	}
	if repo.Alias != "main-repo" {
		t.Errorf("expected primary repo, got %q", repo.Alias)
	}
}

func TestRepositoryService_ResolveRepoForTicket_ByAlias(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	_, _ = svc.AddRepository(ctx, "proj1", "backend", "https://github.com/org/backend.git", "main", true)
	_, _ = svc.AddRepository(ctx, "proj1", "frontend", "https://github.com/org/frontend.git", "develop", false)

	repo, err := svc.ResolveRepoForTicket(ctx, "proj1", "frontend")
	if err != nil {
		t.Fatalf("ResolveRepoForTicket: %v", err)
	}
	if repo.RepoURL != "https://github.com/org/frontend.git" {
		t.Errorf("RepoURL: got %q", repo.RepoURL)
	}
	if repo.DefaultBranch != "develop" {
		t.Errorf("DefaultBranch: got %q, want develop", repo.DefaultBranch)
	}
}

func TestRepositoryService_DefaultBranch(t *testing.T) {
	store := newMockRepoStore()
	svc := NewRepositoryService(store)
	ctx := context.Background()

	// Empty default_branch should default to "main".
	repo, err := svc.AddRepository(ctx, "proj1", "repo", "https://github.com/org/repo.git", "", false)
	if err != nil {
		t.Fatalf("AddRepository: %v", err)
	}
	if repo.DefaultBranch != "main" {
		t.Errorf("DefaultBranch: got %q, want main", repo.DefaultBranch)
	}
}
