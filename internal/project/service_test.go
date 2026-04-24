package project

import (
	"context"
	"errors"
	"testing"
)

type mockProjectStore struct {
	createErr     error
	getByID       *Project
	getByIDErr    error
	list          []Project
	listErr       error
	updateErr     error
	updatePackErr error
	updateCfgErr  error
}

func (m *mockProjectStore) Create(ctx context.Context, p *Project) error {
	if m.createErr != nil {
		return m.createErr
	}
	return nil
}

func (m *mockProjectStore) GetByID(ctx context.Context, id string) (*Project, error) {
	return m.getByID, m.getByIDErr
}

func (m *mockProjectStore) ListByOrgID(ctx context.Context, orgID string, statusFilter string) ([]Project, error) {
	return m.list, m.listErr
}

func (m *mockProjectStore) UpdateStatus(ctx context.Context, projectID, status string) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateRepoURL(ctx context.Context, projectID, repoURL string) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateName(ctx context.Context, projectID, name string) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateSlug(ctx context.Context, projectID, slug string) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateDefaultBranch(ctx context.Context, projectID, branch string) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateContextPack(ctx context.Context, projectID string, pack ContextPack) error {
	return m.updatePackErr
}

func (m *mockProjectStore) UpdateDispatchEnabled(ctx context.Context, projectID string, enabled bool) error {
	return m.updateErr
}

func (m *mockProjectStore) UpdateDispatchConfig(ctx context.Context, projectID string, cfg DispatchConfig) error {
	return m.updateCfgErr
}

func TestService_CreateProject_Slugify(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, "org1", "My Cool Project", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Slug != "my-cool-project" {
		t.Errorf("Slug: got %q, want my-cool-project", p.Slug)
	}
	if p.OrgID != "org1" {
		t.Errorf("OrgID: got %q", p.OrgID)
	}
	if p.Name != "My Cool Project" {
		t.Errorf("Name: got %q", p.Name)
	}
	if p.Status != "active" {
		t.Errorf("Status: got %q", p.Status)
	}
	if p.ID == "" {
		t.Error("ID should be set")
	}
	if p.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestService_CreateProject_DefaultSlugWhenEmpty(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, "org1", "Name", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Slug != "name" {
		t.Errorf("Slug: got %q", p.Slug)
	}
}

func TestService_CreateProject_ExplicitSlug(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, "org1", "Any Name", "custom-slug", "", nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Slug != "custom-slug" {
		t.Errorf("Slug: got %q", p.Slug)
	}
}

func TestService_CreateProject_StoreError(t *testing.T) {
	store := &mockProjectStore{createErr: errors.New("db error")}
	svc := NewService(store)
	ctx := context.Background()

	_, err := svc.CreateProject(ctx, "org1", "X", "", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_GetProject(t *testing.T) {
	proj := &Project{ID: "p1", OrgID: "org1", Name: "P", Slug: "p", Status: "active"}
	store := &mockProjectStore{getByID: proj}
	svc := NewService(store)
	ctx := context.Background()

	got, err := svc.GetProject(ctx, "p1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got != proj {
		t.Errorf("got %+v", got)
	}
}

func TestService_GetProject_NotFound(t *testing.T) {
	store := &mockProjectStore{getByID: nil, getByIDErr: errors.New("not found")}
	svc := NewService(store)
	ctx := context.Background()

	_, err := svc.GetProject(ctx, "p1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_ListByOrgID_StatusFilter(t *testing.T) {
	list := []Project{{ID: "p1", Name: "A", Status: "active"}}
	store := &mockProjectStore{list: list}
	svc := NewService(store)
	ctx := context.Background()

	got, err := svc.ListByOrgID(ctx, "org1", "active")
	if err != nil {
		t.Fatalf("ListByOrgID: %v", err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("got %+v", got)
	}
}

func TestService_ListByOrgID_StoreError(t *testing.T) {
	store := &mockProjectStore{listErr: errors.New("db error")}
	svc := NewService(store)
	ctx := context.Background()

	_, err := svc.ListByOrgID(ctx, "org1", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_UpdateStatus_Active(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "p1", "active")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
}

func TestService_UpdateStatus_Closed(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "p1", "closed")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
}

func TestService_UpdateStatus_Invalid(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "p1", "invalid")
	if err != ErrInvalidStatus {
		t.Errorf("got err %v", err)
	}
}

func TestService_UpdateStatus_StoreError(t *testing.T) {
	store := &mockProjectStore{updateErr: ErrProjectNotFound}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "p1", "active")
	if err != ErrProjectNotFound {
		t.Errorf("got err %v", err)
	}
}

func TestService_UpdateContextPack(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	pack := ContextPack{Conventions: "Go 1.21"}
	err := svc.UpdateContextPack(ctx, "p1", pack)
	if err != nil {
		t.Fatalf("UpdateContextPack: %v", err)
	}
}

func TestService_UpdateContextPack_StoreError(t *testing.T) {
	store := &mockProjectStore{updatePackErr: errors.New("db error")}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateContextPack(ctx, "p1", ContextPack{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_UpdateDispatchEnabled(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateDispatchEnabled(ctx, "p1", true)
	if err != nil {
		t.Fatalf("UpdateDispatchEnabled: %v", err)
	}
}

func TestService_UpdateDispatchEnabled_StoreError(t *testing.T) {
	store := &mockProjectStore{updateErr: ErrProjectNotFound}
	svc := NewService(store)
	ctx := context.Background()

	err := svc.UpdateDispatchEnabled(ctx, "p1", false)
	if err != ErrProjectNotFound {
		t.Errorf("got err %v", err)
	}
}

func TestService_CreateProject_DispatchEnabledByDefault(t *testing.T) {
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, "org1", "Test", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if !p.DispatchEnabled {
		t.Error("DispatchEnabled should default to true")
	}
}

func TestSlugify_EmptyName(t *testing.T) {
	// CreateProject uses slugify when slug is empty; slugify("") -> "project"
	store := &mockProjectStore{}
	svc := NewService(store)
	ctx := context.Background()
	// Name that slugifies to empty (e.g. all non-alphanumeric) -> slug becomes "project"
	p, err := svc.CreateProject(ctx, "org1", "!!!", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.Slug != "project" {
		t.Errorf("Slug for '!!!': got %q, want project", p.Slug)
	}
}
