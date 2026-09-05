package rest

import (
	"context"
	"net/http"

	"github.com/gabinante/flywheel/internal/auth"
)

// ContextKey type for request identity values.
type ContextKey string

// ContextKeyAgentID identifies the local operator or authenticated MCP agent.
const ContextKeyAgentID ContextKey = "agent_id"

// LocalOperatorMiddleware supplies the startup-provisioned identity to local REST
// controls. MCP transports authenticate their own credentials and never inherit it.
// The boundary must run before identity injection, including for SPA navigation.
func LocalOperatorMiddleware(operatorID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return LocalBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMCPPath(r.URL.Path) {
				ctx := context.WithValue(r.Context(), ContextKeyAgentID, operatorID)
				r = r.WithContext(auth.WithOperator(ctx))
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// GetAgentID returns the request identity from context, or "".
func GetAgentID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyAgentID).(string); ok {
		return id
	}
	return ""
}
