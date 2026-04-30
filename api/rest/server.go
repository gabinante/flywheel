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
	OAuthHandler        *OAuthHandler
	MCPHandler          http.Handler
	MCPSSEHandler       http.Handler // SSE transport for older MCP clients
	AgentsHandler       *AgentsHandler
	EntitiesHandler     *EntitiesHandler
	DispatchHandler     *DispatchHandler
	OrchestratorHandler *OrchestratorHandler
	UsageHandler        *UsageHandler
	PlansHandler        *PlansHandler
	ObservationHandler  *ObservationHandler
	StreamsHandler      *StreamsHandler      // Foundational streams (entity, state, change) per spec v0.2 section 2.2
	CatalogHandler      *CatalogHandler      // Layer 14 project map
	PoliciesHandler     *PoliciesHandler     // Policy calibration feedback loop (not in OpenAPI spec yet)
	EnvironmentsHandler *EnvironmentsHandler // Environment CRUD (spec 4.1 compound tuple)
	StateIndexHandler   *StateIndexHandler   // Observed state index (spec v0.2 Layer 10)
	ClaimsHandler       *ClaimsHandler       // Claims registry for concurrency control (spec v0.2 §4.3)
	HooksHandler        *HooksHandler        // Change event webhook receiver (spec v0.2 §2.4)
	PillarsHandler      *PillarsHandler      // Pillar and strategy layer (Layer 15)
	DeliveryHandler     *DeliveryHandler     // Delivery integrations (pipeline, PR, config)
	WorkflowHandler     *WorkflowHandler     // Configurable SDLC workflow definitions
	InvitesHandler      *InvitesHandler      // Org invite links
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
// healthz and API from the spec-generated server, plus metrics, auth, oauth, mcp, agents.
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

	MountWebUI(mux, cfg.WebDist, cfg.WebDevProxyURL)

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
	if cfg.EntitiesHandler != nil {
		cfg.EntitiesHandler.Register(mux)
	}
	if cfg.DispatchHandler != nil {
		mux.HandleFunc("GET /api/dispatch/status", cfg.DispatchHandler.getStatus)
	}
	if cfg.OrchestratorHandler != nil {
		mux.HandleFunc("GET /api/command-center/projects/{projectID}/orchestrator", cfg.OrchestratorHandler.getThread)
		mux.HandleFunc("POST /api/command-center/projects/{projectID}/orchestrator/messages", cfg.OrchestratorHandler.createMessage)
	}
	if cfg.UsageHandler != nil {
		mux.HandleFunc("GET /api/projects/{projectID}/usage", cfg.UsageHandler.getProjectUsage)
	}
	if cfg.PlansHandler != nil {
		plans := cfg.PlansHandler
		mux.HandleFunc("POST /plans/{planID}/freshness-check", plans.freshnessCheck)
		mux.HandleFunc("GET /plans/staleness-config", plans.stalenessConfig)
	}
	if cfg.ObservationHandler != nil {
		cfg.ObservationHandler.RegisterRoutes(mux)
	}
	if cfg.StreamsHandler != nil {
		cfg.StreamsHandler.RegisterRoutes(mux)
	}
	if cfg.CatalogHandler != nil {
		cfg.CatalogHandler.RegisterRoutes(mux)
	}
	if cfg.PoliciesHandler != nil {
		mux.HandleFunc("POST /decisions/{decisionID}/outcome", cfg.PoliciesHandler.recordOutcome)
	}
	if cfg.StateIndexHandler != nil {
		cfg.StateIndexHandler.RegisterRoutes(mux)
	}
	if cfg.ClaimsHandler != nil {
		cfg.ClaimsHandler.RegisterRoutes(mux)
	}
	if cfg.HooksHandler != nil {
		cfg.HooksHandler.RegisterRoutes(mux)
	}
	if cfg.PillarsHandler != nil {
		cfg.PillarsHandler.RegisterRoutes(mux)
	}
	if cfg.DeliveryHandler != nil {
		cfg.DeliveryHandler.RegisterRoutes(mux)
	}
	if cfg.WorkflowHandler != nil {
		cfg.WorkflowHandler.RegisterRoutes(mux)
	}
	if cfg.InvitesHandler != nil {
		cfg.InvitesHandler.RegisterRoutes(mux)
	}
	if cfg.WorkerConfigHandler != nil {
		mux.HandleFunc("GET /worker-config", cfg.WorkerConfigHandler.getConfig)
	}

	h := http.Handler(mux)
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
