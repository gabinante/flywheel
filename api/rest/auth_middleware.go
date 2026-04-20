package rest

import (
	"context"
	"net/http"
	"strings"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
)

// ContextKey type for auth context values.
type ContextKey string

const (
	// ContextKeyAgentID is the context key for the authenticated agent ID (from JWT or API key).
	ContextKeyAgentID ContextKey = "agent_id"
)

// AuthMiddleware optionally authenticates via Bearer JWT or X-API-Key. Sets agent_id in context when valid.
// Does not block unauthenticated requests; use RequireAuth for that.
func AuthMiddleware(jwtSecret string, agentSvc *agent.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			agentID := ""
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if id, err := auth.VerifyJWT(jwtSecret, token); err == nil {
					agentID = id
				}
			}
			if agentID == "" {
				if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
					if a, err := agentSvc.AuthenticateAgent(r.Context(), apiKey); err == nil && a != nil {
						agentID = a.ID
					}
				}
			}
			if agentID != "" {
				ctx := context.WithValue(r.Context(), ContextKeyAgentID, agentID)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetAgentID returns the authenticated agent ID from context, or "".
func GetAgentID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyAgentID).(string); ok {
		return id
	}
	return ""
}
