package rest

import (
	"context"
	"net/http"
	"strings"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
)

// MCPHTTPHandler authenticates harness requests independently of the local UI.
// Workers use X-API-Key; existing programmatic bearer credentials remain valid.
type MCPHTTPHandler struct {
	Handler     http.Handler
	JWTSecret   string
	AgentSvc    *agent.Service
	ValidateRun func(context.Context, auth.RunGrant) error
}

// ServeHTTP injects only the identity and capabilities proven by MCP credentials.
func (h *MCPHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agentID := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if id, err := auth.VerifyJWT(h.JWTSecret, token); err == nil {
			agentID = id
			r = r.WithContext(auth.WithOperator(r.Context()))
		}
	}
	if agentID == "" {
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if grant, ok := auth.LookupRun(apiKey); ok {
				if h.ValidateRun != nil {
					if err := h.ValidateRun(r.Context(), grant); err != nil {
						http.Error(w, "workflow attempt is no longer active", http.StatusForbidden)
						return
					}
				}
				apiKey = grant.ParentKey
				r = r.WithContext(auth.WithRun(r.Context(), grant))
			}
			if a, err := h.AgentSvc.AuthenticateAgent(r.Context(), apiKey); err == nil && a != nil {
				agentID = a.ID
			}
		}
	}
	if agentID == "" {
		http.Error(w, "MCP credentials required: configure X-API-Key", http.StatusUnauthorized)
		return
	}
	if _, err := h.AgentSvc.GetAgent(r.Context(), agentID); err != nil {
		http.Error(w, "MCP agent no longer exists: register a new API key", http.StatusUnauthorized)
		return
	}
	ctx := context.WithValue(r.Context(), ContextKeyAgentID, agentID)
	h.Handler.ServeHTTP(w, r.WithContext(ctx))
}

func isMCPPath(path string) bool {
	return path == "/mcp" || strings.HasPrefix(path, "/mcp/") || path == "/sse" || strings.HasPrefix(path, "/sse/")
}
