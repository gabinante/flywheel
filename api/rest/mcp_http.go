package rest

import (
	"context"
	"net/http"
	"strings"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
)

// MCPHTTPHandler wraps an MCP Streamable HTTP handler and returns 401 with
// WWW-Authenticate when the request has no valid Bearer or X-API-Key, so Cursor
// can discover the OAuth metadata and start the sign-in flow.
type MCPHTTPHandler struct {
	Handler   http.Handler
	BaseURL   string
	JWTSecret string
	AgentSvc  *agent.Service
}

// ServeHTTP authenticates the request; if unauthenticated, returns 401 with
// resource_metadata so the client can perform OAuth. Otherwise injects agent_id
// into context and delegates to the MCP handler.
func (h *MCPHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agentID := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if id, err := auth.VerifyJWT(h.JWTSecret, token); err == nil {
			agentID = id
		}
	}
	if agentID == "" {
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if a, err := h.AgentSvc.AuthenticateAgent(r.Context(), apiKey); err == nil && a != nil {
				agentID = a.ID
			}
		}
	}
	if agentID == "" {
		// MCP client (e.g. Cursor) will use resource_metadata to discover OAuth and open browser.
		metadataURL := h.BaseURL + "/.well-known/oauth-protected-resource"
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// Verify agent exists (e.g. after DB reset, JWT may reference a deleted agent).
	// Return 401 so the client re-authenticates and gets a fresh agent.
	if _, err := h.AgentSvc.GetAgent(r.Context(), agentID); err != nil {
		metadataURL := h.BaseURL + "/.well-known/oauth-protected-resource"
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+metadataURL+`"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx := context.WithValue(r.Context(), ContextKeyAgentID, agentID)
	h.Handler.ServeHTTP(w, r.WithContext(ctx))
}
