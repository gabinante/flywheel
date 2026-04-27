package rest

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/delivery"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

type DeliveryHandler struct {
	DeliverySvc *delivery.Service
	ProjectSvc  *project.Service
	OrgSvc      *org.Service
	AgentStore  agent.AgentStore
}

func (h *DeliveryHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/pull-requests", h.listPullRequests)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/pipeline", h.getPipeline)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/integrations", h.getConfig)
	mux.HandleFunc("PUT /api/v1/projects/{projectID}/integrations", h.updateConfig)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/delivery-config", h.getConfig)
	mux.HandleFunc("PUT /api/v1/projects/{projectID}/delivery-config", h.updateConfig)
}

func (h *DeliveryHandler) listPullRequests(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	overview, err := h.DeliverySvc.ListOpenPullRequests(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (h *DeliveryHandler) getPipeline(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	refresh := true
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("refresh"))) {
	case "0", "false", "no":
		refresh = false
	}
	overview, err := h.DeliverySvc.Pipeline(r.Context(), projectID, refresh)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (h *DeliveryHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	// Return masked config — never expose raw credentials.
	resp, err := h.DeliverySvc.GetMaskedConfig(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *DeliveryHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var cfg delivery.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.DeliverySvc.UpdateConfig(r.Context(), projectID, cfg); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	// Return masked response — never echo raw credentials back.
	resp, err := h.DeliverySvc.GetMaskedConfig(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
