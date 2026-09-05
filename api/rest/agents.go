package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// AgentsHandler registers and inspects local MCP agents.
type AgentsHandler struct {
	AgentSvc *agent.Service
}

func (h *AgentsHandler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string     `json:"name"`
		Type agent.Type `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.Name == "" {
		body.Name = "agent"
	}
	if body.Type == "" {
		body.Type = agent.TypeCustom
	}
	operator, err := h.AgentSvc.GetAgent(r.Context(), GetAgentID(r.Context()))
	if err != nil || operator == nil || operator.UserID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "local operator identity required", false))
		return
	}
	a, apiKey, err := h.AgentSvc.RegisterAgentForUser(r.Context(), body.Name, body.Type, operator.UserID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"agent": a, "api_key": apiKey})
}

func (h *AgentsHandler) getAgent(w http.ResponseWriter, r *http.Request) {
	agentID := PathParam(r, "agentID")
	callerID := GetAgentID(r.Context())
	if callerID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "authentication required", false))
		return
	}
	if callerID != agentID && !auth.IsOperator(r.Context()) {
		WriteStructuredError(w, apierrors.New(apierrors.CodeForbidden, "you may only access your own agent record", false))
		return
	}
	a, err := h.AgentSvc.GetAgent(r.Context(), agentID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}
