package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/api/rest/middleware"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// RouterConfig configures the main HTTP router (std net/http only).
type RouterConfig struct {
	StrictServer   *StrictServer
	AuthMiddleware func(http.Handler) http.Handler
	AuthHandler    *AuthHandler
	OAuthHandler   *OAuthHandler
	MCPHandler     http.Handler
	MCPSSEHandler  http.Handler // SSE transport for older MCP clients
	AgentsHandler  *AgentsHandler
	// WebDist is the Vite outDir (contains index.html and assets/). Empty skips SPA routes.
	WebDist string
}

// NewRouter returns an http.Handler with global middleware and all routes:
// healthz and API from the spec-generated server, plus metrics, auth, oauth, mcp, agents.
func NewRouter(cfg RouterConfig) http.Handler {
	mux := http.NewServeMux()

	// Metrics (not in OpenAPI spec)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# Warrant metrics\n# Expose Prometheus or other metrics here when needed.\n"))
	})

	// Spec-generated API (healthz + all spec routes) — registers onto mux
	responseErrHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		if se, ok := err.(*apierrors.StructuredError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(se.HTTPStatus())
			w.Write([]byte(se.JSON()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "code": "internal", "retriable": false})
	}
	requestErrHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "code": "invalid_input", "retriable": false})
	}
	strictHandler := generated.NewStrictHandlerWithOptions(cfg.StrictServer, nil, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrHandler,
		ResponseErrorHandlerFunc: responseErrHandler,
	})
	_ = generated.HandlerWithOptions(strictHandler, generated.StdHTTPServerOptions{BaseRouter: mux})

	MountWebUI(mux, cfg.WebDist)

	// Auth routes (when configured)
	if cfg.AuthHandler != nil {
		auth := cfg.AuthHandler
		mux.HandleFunc("GET /auth/github", auth.githubRedirect)
		mux.HandleFunc("GET /auth/github/callback", auth.githubCallback)
	}
	if cfg.OAuthHandler != nil {
		oauth := cfg.OAuthHandler
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", oauth.serveProtectedResourceMetadata)
		mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauth.serveAuthorizationServerMetadata)
		mux.HandleFunc("GET /oauth/authorize", oauth.oauthAuthorize)
		mux.HandleFunc("POST /oauth/token", oauth.oauthToken)
		mux.HandleFunc("POST /oauth/register", oauth.oauthRegister)
	}
	if cfg.MCPHandler != nil {
		mux.Handle("/mcp", cfg.MCPHandler)
		mux.Handle("/mcp/", cfg.MCPHandler)
	}
	if cfg.MCPSSEHandler != nil {
		mux.Handle("/sse", cfg.MCPSSEHandler)
		mux.Handle("/sse/", cfg.MCPSSEHandler)
	}
	if cfg.AgentsHandler != nil {
		agents := cfg.AgentsHandler
		mux.HandleFunc("POST /agents", agents.register)
		mux.HandleFunc("GET /agents/{agentID}", agents.getAgent)
	}

	h := http.Handler(mux)
	h = middleware.Recoverer(h)
	h = middleware.Logger(h)
	h = middleware.RealIP(h)
	h = middleware.RequestID(h)
	if cfg.AuthMiddleware != nil {
		h = cfg.AuthMiddleware(h)
	}
	return h
}
