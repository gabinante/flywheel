package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/orchestrator"
)

type OrchestratorServiceForHandler interface {
	GetThread(ctx context.Context, projectID string) (*orchestrator.Thread, error)
	SendUserMessage(ctx context.Context, projectID, content string) (*orchestrator.Thread, error)
	CancelRun(ctx context.Context, projectID, runID string) error
	SubscribeRunEvents(ctx context.Context, projectID string) <-chan orchestrator.RunEvent
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
	writeOrchestratorJSON(w, http.StatusAccepted, thread)
}

func (h *OrchestratorHandler) cancelRun(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeServiceUnavailable(w, "orchestrator not enabled")
		return
	}
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	runID := PathParam(r, "runID")
	if err := h.Service.CancelRun(r.Context(), projectID, runID); err != nil {
		if errors.Is(err, orchestrator.ErrRunNotActive) {
			WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, err.Error(), false))
		} else {
			WriteStructuredError(w, apierrors.MapError(err))
		}
		return
	}
	writeOrchestratorJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (h *OrchestratorHandler) streamEvents(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeServiceUnavailable(w, "orchestrator not enabled")
		return
	}
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.Service.SubscribeRunEvents(r.Context(), projectID)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
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
