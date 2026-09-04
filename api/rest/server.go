package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/api/rest/middleware"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouterConfig configures the main HTTP router (std net/http only).
type RouterConfig struct {
	StrictServer        *StrictServer
	AuthMiddleware      func(http.Handler) http.Handler
	AuthHandler         *AuthHandler
	MCPHandler          http.Handler
	MCPSSEHandler       http.Handler // SSE transport for older MCP clients
	AgentsHandler       *AgentsHandler
	DispatchHandler     *DispatchHandler
	OrchestratorHandler *OrchestratorHandler
	WorkflowHandler     *WorkflowHandler     // Configurable SDLC workflow definitions
	WorkerConfigHandler *WorkerConfigHandler // MCP config for local Claude Code workers
	// HealthCheckers are called by /readyz for deep readiness checks.
	HealthCheckers []HealthChecker
	// WebDist is the Vite outDir (contains index.html and assets/). Empty skips SPA routes.
	WebDist string
	// WebDevProxyURL reverse-proxies frontend requests to a running Vite dev server.
	WebDevProxyURL string
}

// HealthChecker reports whether a subsystem is ready to serve traffic.
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
}

// NewRouter returns an http.Handler with global middleware and all routes:
// healthz and API from the spec-generated server, plus metrics, auth, mcp, agents.
func NewRouter(cfg RouterConfig) http.Handler {
	mux := http.NewServeMux()

	// Metrics (not in OpenAPI spec)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Deep readiness check: pings all configured subsystems (DB, Redis, etc.).
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		results := make(map[string]string, len(cfg.HealthCheckers))
		allOK := true
		for _, hc := range cfg.HealthCheckers {
			if err := hc.Check(ctx); err != nil {
				results[hc.Name()] = err.Error()
				allOK = false
			} else {
				results[hc.Name()] = "ok"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if allOK {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": allOK, "checks": results})
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

	spa := MountWebUI(mux, cfg.WebDist, cfg.WebDevProxyURL)

	// Auth routes (when configured)
	// Local operator sign-in: issues a JWT for the single local identity.
	if cfg.AuthHandler != nil {
		mux.HandleFunc("GET /auth/login", cfg.AuthHandler.login)
	}
	if cfg.MCPHandler != nil {
		// Streamable HTTP transport uses GET (SSE stream), POST (messages), DELETE (session end).
		// Explicit methods avoid conflict with the SPA catch-all "GET /".
		mux.Handle("GET /mcp", cfg.MCPHandler)
		mux.Handle("POST /mcp", cfg.MCPHandler)
		mux.Handle("DELETE /mcp", cfg.MCPHandler)
		mux.Handle("GET /mcp/", cfg.MCPHandler)
		mux.Handle("POST /mcp/", cfg.MCPHandler)
		mux.Handle("DELETE /mcp/", cfg.MCPHandler)
	}
	if cfg.MCPSSEHandler != nil {
		// Legacy SSE transport: GET for connection, POST for messages via /sse/ prefix.
		mux.Handle("GET /sse", cfg.MCPSSEHandler)
		mux.Handle("GET /sse/", cfg.MCPSSEHandler)
		mux.Handle("POST /sse/", cfg.MCPSSEHandler)
	}
	if cfg.AgentsHandler != nil {
		agents := cfg.AgentsHandler
		mux.HandleFunc("POST /agents", agents.register)
		mux.HandleFunc("GET /agents/{agentID}", agents.getAgent)
	}
	if cfg.DispatchHandler != nil {
		mux.HandleFunc("GET /api/dispatch/status", cfg.DispatchHandler.getStatus)
	}
	if cfg.OrchestratorHandler != nil {
		mux.HandleFunc("GET /api/command-center/projects/{projectID}/orchestrator", cfg.OrchestratorHandler.getThread)
		mux.HandleFunc("POST /api/command-center/projects/{projectID}/orchestrator/messages", cfg.OrchestratorHandler.createMessage)
		mux.HandleFunc("DELETE /api/command-center/projects/{projectID}/orchestrator/runs/{runID}", cfg.OrchestratorHandler.cancelRun)
		mux.HandleFunc("GET /api/command-center/projects/{projectID}/orchestrator/events", cfg.OrchestratorHandler.streamEvents)
	}
	if cfg.WorkflowHandler != nil {
		cfg.WorkflowHandler.RegisterRoutes(mux)
	}
	if cfg.WorkerConfigHandler != nil {
		mux.HandleFunc("GET /worker-config", cfg.WorkerConfigHandler.getConfig)
	}

	h := WebUINavigation(spa, mux)
	h = middleware.Recoverer(h)
	h = middleware.Metrics(h)
	h = middleware.Logger(h)
	h = middleware.RealIP(h)
	h = middleware.RequestID(h)
	if cfg.AuthMiddleware != nil {
		h = cfg.AuthMiddleware(h)
	}
	return h
}
