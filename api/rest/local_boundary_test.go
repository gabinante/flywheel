package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabinante/flywheel/internal/auth"
)

func TestLocalBoundary(t *testing.T) {
	h := LocalBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		name, host, origin, site string
		want                     int
	}{
		{name: "local controls without login", host: "localhost:8090", want: 204},
		{name: "same origin", host: "localhost:8090", origin: "http://localhost:8090", want: 204},
		{name: "IPv4", host: "127.0.0.1:8090", want: 204},
		{name: "IPv6", host: "[::1]:8090", want: 204},
		{name: "cross origin", host: "localhost:8090", origin: "https://attacker.test", want: 403},
		{name: "another local port", host: "localhost:8090", origin: "http://localhost:9090", want: 403},
		{name: "opaque origin", host: "localhost:8090", origin: "null", want: 403},
		{name: "rebinding", host: "attacker.test", want: 403},
		{name: "cross-site fetch", host: "localhost:8090", site: "cross-site", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://"+tc.host+"/settings", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLocalOperatorNeverPromotesMCP(t *testing.T) {
	for _, path := range []string{"/settings", "/projects/p/tickets", "/mcp", "/mcp/", "/sse", "/sse/message"} {
		t.Run(path, func(t *testing.T) {
			h := LocalOperatorMiddleware("operator")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantID := "operator"
				if isMCPPath(path) {
					wantID = ""
				}
				if got := GetAgentID(r.Context()); got != wantID {
					t.Fatalf("identity = %q, want %q", got, wantID)
				}
				if auth.IsOperator(r.Context()) != (wantID != "") {
					t.Fatal("operator permissions leaked into MCP")
				}
			}))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost:8090"+path, nil))
		})
	}
	h := LocalOperatorMiddleware("operator")(&MCPHTTPHandler{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "http://localhost:8090/mcp", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous MCP got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("MCP advertised a removed browser login")
	}
}
