package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
)

// requestWithAgent builds a test request carrying an authenticated agent ID.
func requestWithAgent(method, path string, agentID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if agentID != "" {
		req = req.WithContext(context.WithValue(req.Context(), ContextKeyAgentID, agentID))
	}
	return req
}
