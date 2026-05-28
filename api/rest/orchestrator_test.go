package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/orchestrator"
	"github.com/gabinante/flywheel/internal/project"
)

type mockOrchestratorService struct {
	thread        *orchestrator.Thread
	getErr        error
	sendErr       error
	cancelErr     error
	lastProjectID string
	lastContent   string
	lastRunID     string
}

func (m *mockOrchestratorService) GetThread(_ context.Context, projectID string) (*orchestrator.Thread, error) {
	m.lastProjectID = projectID
	return m.thread, m.getErr
}

func (m *mockOrchestratorService) SendUserMessage(_ context.Context, projectID, content string) (*orchestrator.Thread, error) {
	m.lastProjectID = projectID
	m.lastContent = content
	return m.thread, m.sendErr
}

func (m *mockOrchestratorService) CancelRun(_ context.Context, projectID, runID string) error {
	m.lastProjectID = projectID
	m.lastRunID = runID
	return m.cancelErr
}

func (m *mockOrchestratorService) SubscribeRunEvents(_ context.Context, projectID string) <-chan orchestrator.RunEvent {
	m.lastProjectID = projectID
	ch := make(chan orchestrator.RunEvent)
	close(ch)
	return ch
}

func authWithAgent(agentID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ContextKeyAgentID, agentID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestOrchestratorHandlerGetThreadRequiresAuth(t *testing.T) {
	router := NewRouter(RouterConfig{
		StrictServer: &StrictServer{},
		OrchestratorHandler: &OrchestratorHandler{
			Service:    &mockOrchestratorService{},
			ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj-1", OrgID: "org-1"}},
			OrgSvc:     &mockOrgSvc{orgIDs: []string{"org-1"}},
			AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent-1", UserID: "user-1"}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://test/api/command-center/projects/proj-1/orchestrator", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestOrchestratorHandlerGetThreadOK(t *testing.T) {
	service := &mockOrchestratorService{
		thread: &orchestrator.Thread{
			ProjectID: "proj-1",
			Messages: []orchestrator.Message{
				{ID: "msg-1", ProjectID: "proj-1", Role: orchestrator.RoleAssistant, Content: "Ready."},
			},
			Playbook: orchestrator.DefaultPlaybook(),
		},
	}
	router := NewRouter(RouterConfig{
		StrictServer:   &StrictServer{},
		AuthMiddleware: authWithAgent("agent-1"),
		OrchestratorHandler: &OrchestratorHandler{
			Service:    service,
			ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj-1", OrgID: "org-1"}},
			OrgSvc:     &mockOrgSvc{orgIDs: []string{"org-1"}},
			AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent-1", UserID: "user-1"}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://test/api/command-center/projects/proj-1/orchestrator", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var thread orchestrator.Thread
	if err := json.Unmarshal(w.Body.Bytes(), &thread); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if thread.ProjectID != "proj-1" {
		t.Fatalf("expected project proj-1, got %q", thread.ProjectID)
	}
}

func TestOrchestratorHandlerCreateMessageOK(t *testing.T) {
	service := &mockOrchestratorService{
		thread: &orchestrator.Thread{
			ProjectID: "proj-1",
			Messages: []orchestrator.Message{
				{ID: "msg-1", ProjectID: "proj-1", Role: orchestrator.RoleUser, Content: "Ship the feature."},
				{ID: "msg-2", ProjectID: "proj-1", Role: orchestrator.RoleAssistant, Content: "Created the work stream."},
			},
			Playbook: orchestrator.DefaultPlaybook(),
		},
	}
	router := NewRouter(RouterConfig{
		StrictServer:   &StrictServer{},
		AuthMiddleware: authWithAgent("agent-1"),
		OrchestratorHandler: &OrchestratorHandler{
			Service:    service,
			ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj-1", OrgID: "org-1"}},
			OrgSvc:     &mockOrgSvc{orgIDs: []string{"org-1"}},
			AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "agent-1", UserID: "user-1"}},
		},
	})

	body := bytes.NewBufferString(`{"content":"Ship the feature."}`)
	req := httptest.NewRequest(http.MethodPost, "http://test/api/command-center/projects/proj-1/orchestrator/messages", body)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}
	if service.lastProjectID != "proj-1" || service.lastContent != "Ship the feature." {
		t.Fatalf("unexpected service call: project=%q content=%q", service.lastProjectID, service.lastContent)
	}
}
