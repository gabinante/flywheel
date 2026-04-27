package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouter_Healthz_200 verifies the main router serves GET /healthz (spec-generated).
func TestRouter_Healthz_200(t *testing.T) {
	router := NewRouter(RouterConfig{
		StrictServer: &StrictServer{}, // nil deps; healthz uses none
	})
	req := httptest.NewRequest(http.MethodGet, "http://test/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /healthz: got status %d, want 200", w.Code)
	}
}

// TestRouter_Metrics_200 verifies the main router serves GET /metrics (Prometheus format).
func TestRouter_Metrics_200(t *testing.T) {
	router := NewRouter(RouterConfig{
		StrictServer: &StrictServer{},
	})
	req := httptest.NewRequest(http.MethodGet, "http://test/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /metrics: got status %d, want 200", w.Code)
	}
}

// TestRouter_Readyz_200 verifies the main router serves GET /readyz with no health checkers.
func TestRouter_Readyz_200(t *testing.T) {
	router := NewRouter(RouterConfig{
		StrictServer: &StrictServer{},
	})
	req := httptest.NewRequest(http.MethodGet, "http://test/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /readyz: got status %d, want 200", w.Code)
	}
}

// TestRouter_OrgList_401_NoAuth verifies GET /orgs returns 401 when no agent is in context.
func TestRouter_OrgList_401_NoAuth(t *testing.T) {
	router := NewRouter(RouterConfig{
		StrictServer: &StrictServer{}, // no auth middleware -> no agent ID
	})
	req := httptest.NewRequest(http.MethodGet, "http://test/orgs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /orgs (no auth): got status %d, want 401", w.Code)
	}
}

