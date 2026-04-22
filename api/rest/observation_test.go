package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/observation"
)

func newTestObservationHandler() *ObservationHandler {
	store := observation.NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := observation.NewService(store, bus)
	return &ObservationHandler{Svc: svc}
}

func TestObservationHandler_OpenWindow(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"ticket_id":"ticket-1","project_id":"project-1","scope":{"services":["api"],"files":["pkg/api/handler.go"]}}`
	req := httptest.NewRequest("POST", "/api/v1/observation/windows", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var window observation.ObservationWindow
	if err := json.NewDecoder(rr.Body).Decode(&window); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if window.ID == "" {
		t.Fatal("expected window ID")
	}
	if window.TicketID != "ticket-1" {
		t.Fatalf("expected ticket-1, got %s", window.TicketID)
	}
}

func TestObservationHandler_ListOpenWindows(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Open a window first.
	body := `{"ticket_id":"ticket-1","project_id":"project-1","scope":{"services":["api"]}}`
	req := httptest.NewRequest("POST", "/api/v1/observation/windows", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// List open windows.
	req = httptest.NewRequest("GET", "/api/v1/observation/windows?project_id=project-1", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var windows []*observation.ObservationWindow
	if err := json.NewDecoder(rr.Body).Decode(&windows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
}

func TestObservationHandler_IngestSignal(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Open a window.
	body := `{"ticket_id":"ticket-1","project_id":"project-1","scope":{"services":["api"]}}`
	req := httptest.NewRequest("POST", "/api/v1/observation/windows", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Ingest a signal.
	signal := observation.Signal{
		ProjectID:  "project-1",
		Source:     "test",
		SignalType: observation.SignalRegression,
		Severity:   observation.SeverityHigh,
		Title:      "API latency spike",
		Scope:      observation.Scope{Services: []string{"api"}},
		OccurredAt: time.Now().UTC(),
	}
	sigBody, _ := json.Marshal(signal)
	req = httptest.NewRequest("POST", "/api/v1/observation/signals", bytes.NewBuffer(sigBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		SignalID    string                  `json:"signal_id"`
		Attribution *observation.Attribution `json:"attribution"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Attribution == nil {
		t.Fatal("expected attribution")
	}
	if len(resp.Attribution.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Attribution.Candidates))
	}
}

func TestObservationHandler_AmbiguityMetrics(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/observation/metrics/ambiguity?project_id=project-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var metrics observation.AmbiguityMetrics
	if err := json.NewDecoder(rr.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if metrics.ProjectID != "project-1" {
		t.Fatalf("expected project-1, got %s", metrics.ProjectID)
	}
}

func TestObservationHandler_ResolveAttribution(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Open a window and ingest a signal.
	body := `{"ticket_id":"ticket-1","project_id":"project-1","scope":{"services":["api"]}}`
	req := httptest.NewRequest("POST", "/api/v1/observation/windows", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	signal := observation.Signal{
		ProjectID:  "project-1",
		Source:     "test",
		SignalType: observation.SignalRegression,
		Severity:   observation.SeverityHigh,
		Title:      "API latency spike",
		Scope:      observation.Scope{Services: []string{"api"}},
		OccurredAt: time.Now().UTC(),
	}
	sigBody, _ := json.Marshal(signal)
	req = httptest.NewRequest("POST", "/api/v1/observation/signals", bytes.NewBuffer(sigBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var sigResp struct {
		Attribution *observation.Attribution `json:"attribution"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&sigResp)

	// Resolve the attribution.
	resolveBody := `{"resolved_by":"user-1","resolution":"confirmed: ticket-1 caused the regression"}`
	req = httptest.NewRequest("POST", "/api/v1/observation/attributions/"+sigResp.Attribution.ID+"/resolve",
		bytes.NewBufferString(resolveBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify it's no longer in unresolved list.
	req = httptest.NewRequest("GET", "/api/v1/observation/attributions?project_id=project-1", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var attrs []*observation.Attribution
	_ = json.NewDecoder(rr.Body).Decode(&attrs)
	if len(attrs) != 0 {
		t.Fatalf("expected 0 unresolved attributions, got %d", len(attrs))
	}
}

func TestObservationHandler_Validation(t *testing.T) {
	h := newTestObservationHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Missing project_id in query.
	req := httptest.NewRequest("GET", "/api/v1/observation/windows", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	// Missing required fields in signal.
	req = httptest.NewRequest("POST", "/api/v1/observation/signals", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", rr.Code)
	}
}
