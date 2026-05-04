package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
)

type mockTraceService struct {
	logStepErr  error
	getTrace    *execution.ExecutionTrace
	getTraceErr error
}

func (m *mockTraceService) LogStep(ctx context.Context, ticketID, leaseToken string, step execution.Step) error {
	return m.logStepErr
}

func (m *mockTraceService) GetTrace(ctx context.Context, ticketID string) (*execution.ExecutionTrace, error) {
	return m.getTrace, m.getTraceErr
}

func (m *mockTraceService) GetTracePaginated(ctx context.Context, ticketID string, limit, offset int) (*execution.ExecutionTrace, error) {
	return m.getTrace, m.getTraceErr
}

type mockTicketGetter struct {
	ticket *ticket.Ticket
	err   error
}

func (m *mockTicketGetter) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	return m.ticket, m.err
}

type mockReviewService struct {
	listIDs     []string
	listErr     error
	approveErr  error
	rejectErr   error
	reopenErr   error
	listEsc     []review.Escalation
	listEscErr  error
	resolveErr  error
}

func (m *mockReviewService) ListPendingReviews(ctx context.Context, projectID string) ([]string, error) {
	return m.listIDs, m.listErr
}

func (m *mockReviewService) ApproveTicket(ctx context.Context, ticketID, reviewerID, notes string) error {
	return m.approveErr
}

func (m *mockReviewService) RejectTicket(ctx context.Context, ticketID, reviewerID, notes string) error {
	return m.rejectErr
}

func (m *mockReviewService) ReopenTicketForReview(ctx context.Context, ticketID, reviewerID, notes string) error {
	return m.reopenErr
}

func (m *mockReviewService) ListEscalations(ctx context.Context, projectID string) ([]review.Escalation, error) {
	return m.listEsc, m.listEscErr
}

func (m *mockReviewService) ResolveEscalation(ctx context.Context, ticketID, escalationID, reviewerID, answer string) error {
	return m.resolveErr
}

func traceHandlerRouter(h *TraceHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tickets/{ticketID}/trace", h.logStep)
	mux.HandleFunc("GET /tickets/{ticketID}/trace", h.getTrace)
	return mux
}

func reviewsHandlerRouter(h *ReviewsHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/{projectID}/reviews", h.listPendingReviews)
	mux.HandleFunc("POST /tickets/{ticketID}/reviews", h.createReview)
	mux.HandleFunc("GET /projects/{projectID}/escalations", h.listEscalations)
	mux.HandleFunc("POST /tickets/{ticketID}/escalations/{escalationID}/resolve", h.resolveEscalation)
	return mux
}

func TestTraceHandler_logStep_MissingLeaseToken(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &TraceHandler{
		TraceSvc:   &mockTraceService{},
		TicketSvc:  &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	body := `{"step":{"type":"tool_call","payload":{}}}`
	req := requestWithAgent(http.MethodPost, "http://test/tickets/t1/trace", "agent1")
	req.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	traceHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestTraceHandler_logStep_MissingStepType(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &TraceHandler{
		TraceSvc:   &mockTraceService{},
		TicketSvc:  &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	body := `{"lease_token":"tok","step":{"payload":{}}}`
	req := requestWithAgent(http.MethodPost, "http://test/tickets/t1/trace", "agent1")
	req.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	traceHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestTraceHandler_getTrace_401(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &TraceHandler{
		TraceSvc:   &mockTraceService{getTrace: &execution.ExecutionTrace{}},
		TicketSvc:  &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{},
		AgentStore: &mockAgentStore{},
	}
	req := requestWithAgent(http.MethodGet, "http://test/tickets/t1/trace", "")
	w := httptest.NewRecorder()
	traceHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestTraceHandler_getTrace_200(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	trace := &execution.ExecutionTrace{TicketID: "t1", AgentID: "a1", Steps: []execution.Step{{Type: execution.StepTypeToolCall}}}
	h := &TraceHandler{
		TraceSvc:   &mockTraceService{getTrace: trace},
		TicketSvc:  &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	req := requestWithAgent(http.MethodGet, "http://test/tickets/t1/trace", "agent1")
	w := httptest.NewRecorder()
	traceHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
	var out execution.ExecutionTrace
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.TicketID != "t1" || len(out.Steps) != 1 {
		t.Errorf("got %+v", out)
	}
}

func TestReviewsHandler_listPendingReviews_403(t *testing.T) {
	h := &ReviewsHandler{
		ReviewSvc:  &mockReviewService{listIDs: []string{}},
		TicketSvc: &mockTicketGetter{},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"other"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	req := requestWithAgent(http.MethodGet, "http://test/projects/proj1/reviews", "agent1")
	w := httptest.NewRecorder()
	reviewsHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want 403", w.Code)
	}
}

func TestReviewsHandler_listPendingReviews_200(t *testing.T) {
	h := &ReviewsHandler{
		ReviewSvc:  &mockReviewService{listIDs: []string{"t1"}},
		TicketSvc: &mockTicketGetter{ticket: &ticket.Ticket{ID: "t1", Title: "T1", ProjectID: "proj1"}},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	req := requestWithAgent(http.MethodGet, "http://test/projects/proj1/reviews", "agent1")
	w := httptest.NewRecorder()
	reviewsHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
	var out struct {
		Tickets []*ticket.Ticket `json:"tickets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tickets) != 1 || out.Tickets[0].ID != "t1" {
		t.Errorf("got %+v", out.Tickets)
	}
}

func TestReviewsHandler_createReview_Approve_200(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &ReviewsHandler{
		ReviewSvc:  &mockReviewService{},
		TicketSvc: &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	body := `{"decision":"approved","notes":"ok","reviewer_id":"rev1"}`
	req := requestWithAgent(http.MethodPost, "http://test/tickets/t1/reviews", "agent1")
	req.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	reviewsHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
}

func TestReviewsHandler_createReview_Reject_200(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &ReviewsHandler{
		ReviewSvc:  &mockReviewService{},
		TicketSvc: &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	body := `{"decision":"rejected","notes":"needs work"}`
	req := requestWithAgent(http.MethodPost, "http://test/tickets/t1/reviews", "agent1")
	req.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	reviewsHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got status %d body %s", w.Code, w.Body.String())
	}
}

func TestReviewsHandler_createReview_InvalidDecision_400(t *testing.T) {
	tick := &ticket.Ticket{ID: "t1", ProjectID: "proj1"}
	h := &ReviewsHandler{
		ReviewSvc:  &mockReviewService{},
		TicketSvc: &mockTicketGetter{ticket: tick},
		ProjectSvc: &mockProjectSvc{proj: &project.Project{ID: "proj1", OrgID: "org1"}},
		OrgSvc:     &mockOrgSvc{orgIDs: []string{"org1"}},
		AgentStore: &mockAgentStore{agent: &agent.Agent{ID: "a1", UserID: "u1"}},
	}
	body := `{"decision":"pending"}`
	req := requestWithAgent(http.MethodPost, "http://test/tickets/t1/reviews", "agent1")
	req.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	reviewsHandlerRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}
