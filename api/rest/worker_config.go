package rest

import (
	"encoding/json"
	"net/http"
)

// WorkerConfigHandler serves MCP config JSON for local Claude Code workers.
type WorkerConfigHandler struct {
	BaseURL string
}

func (h *WorkerConfigHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	agentID := GetAgentID(r.Context())
	if agentID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
		return
	}
	baseURL := h.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}

	// Build the MCP config that a user would paste into their Claude Code settings.
	config := map[string]any{
		"mcpServers": map[string]any{
			"flywheel": map[string]any{
				"type": "streamable-http",
				"url":  baseURL + "/mcp",
				"headers": map[string]string{
					"X-API-Key": "<your-agent-api-key>",
				},
				"note": "Register an agent with POST /agents and replace <your-agent-api-key> with the returned api_key. Dispatched workers receive credentials automatically.",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(config)
}
