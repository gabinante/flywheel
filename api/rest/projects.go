package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// ProjectsHandler handles project-level REST endpoints (get project, context pack).
type ProjectsHandler struct {
	ProjectSvc *project.Service
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

func (h *ProjectsHandler) getProject(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	p, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *ProjectsHandler) updateContextPack(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var pack project.ContextPack
	if err := json.NewDecoder(r.Body).Decode(&pack); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.ProjectSvc.UpdateContextPack(r.Context(), projectID, pack); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ProjectsHandler) patchProject(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.ProjectSvc.UpdateStatus(r.Context(), projectID, body.Status); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	p, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(p)
}
