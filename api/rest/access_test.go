package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/project"
)

type mockAgentStore struct {
	agent *agent.Agent
	err   error
}

func (m *mockAgentStore) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
	return m.agent, m.err
}

type mockOrgSvc struct {
	orgIDs []string
	err    error
}

func (m *mockOrgSvc) ListOrgIDsForUser(ctx context.Context, userID string) ([]string, error) {
	return m.orgIDs, m.err
}

type mockProjectSvc struct {
	proj *project.Project
	err  error
}

func (m *mockProjectSvc) GetProject(ctx context.Context, projectID string) (*project.Project, error) {
	return m.proj, m.err
}

func TestEnsureOrgAccess_NoAgent(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.Background()
	ok := EnsureOrgAccess(ctx, w, "org1", &mockAgentStore{}, &mockOrgSvc{})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestEnsureOrgAccess_AgentNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureOrgAccess(ctx, w, "org1", &mockAgentStore{agent: nil, err: nil}, &mockOrgSvc{})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestEnsureOrgAccess_NoUserLink(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureOrgAccess(ctx, w, "org1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: ""}}, &mockOrgSvc{})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestEnsureOrgAccess_NotMember(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureOrgAccess(ctx, w, "org1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}}, &mockOrgSvc{orgIDs: []string{"org2"}})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", w.Code)
	}
}

func TestEnsureOrgAccess_Member(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureOrgAccess(ctx, w, "org1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}}, &mockOrgSvc{orgIDs: []string{"org2", "org1"}})
	if !ok {
		t.Fatal("expected true")
	}
	if w.Code != 0 && w.Code != http.StatusOK {
		t.Errorf("expected no error write, got status %d body %s", w.Code, w.Body.String())
	}
}

func TestEnsureProjectAccess_ProjectMissing(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureProjectAccess(ctx, w, "proj1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}}, &mockOrgSvc{}, &mockProjectSvc{proj: nil, err: nil})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestEnsureProjectAccess_OrgAccessDenied(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureProjectAccess(ctx, w, "proj1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}}, &mockOrgSvc{orgIDs: []string{"other-org"}}, &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}})
	if ok {
		t.Fatal("expected false")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", w.Code)
	}
}

func TestEnsureProjectAccess_Allowed(t *testing.T) {
	w := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "agent1")
	ok := EnsureProjectAccess(ctx, w, "proj1", &mockAgentStore{agent: &agent.Agent{ID: "agent1", UserID: "user1"}}, &mockOrgSvc{orgIDs: []string{"org1"}}, &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}})
	if !ok {
		t.Fatal("expected true")
	}
	if w.Code != 0 && w.Code != http.StatusOK {
		t.Errorf("expected no error write, got status %d", w.Code)
	}
}
