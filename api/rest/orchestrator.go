package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/orchestrator"
)

type OrchestratorServiceForHandler interface {
	GetThread(ctx context.Context, projectID string) (*orchestrator.Thread, error)
	SendUserMessage(ctx context.Context, projectID, content string) (*orchestrator.Thread, error)
}

type OrchestratorHandler struct {
	Service    OrchestratorServiceForHandler
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

func (h *OrchestratorHandler) getThread(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeServiceUnavailable(w, "orchestrator not enabled")
		return
	}
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	thread, err := h.Service.GetThread(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeOrchestratorJSON(w, http.StatusOK, thread)
}

func (h *OrchestratorHandler) createMessage(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeServiceUnavailable(w, "orchestrator not enabled")
		return
	}
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}

	thread, err := h.Service.SendUserMessage(r.Context(), projectID, body.Content)
	if err != nil {
		switch {
		case errors.Is(err, orchestrator.ErrMessageContentRequired):
			WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		case errors.Is(err, orchestrator.ErrWorkerNotConfigured):
			writeServiceUnavailable(w, err.Error())
		default:
			WriteStructuredError(w, apierrors.MapError(err))
		}
		return
	}
	writeOrchestratorJSON(w, http.StatusOK, thread)
}

func writeServiceUnavailable(w http.ResponseWriter, message string) {
	writeOrchestratorJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":     message,
		"code":      "service_unavailable",
		"retriable": false,
	})
}

func writeOrchestratorJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
