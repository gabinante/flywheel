package rest

import (
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/projecttemplate"
)

type ProjectTemplatesHandler struct {
	Svc        *projecttemplate.Service
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

func (h *ProjectTemplatesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/project-templates", h.listProjectTemplates)
	mux.HandleFunc("GET /api/v1/workstream-templates", h.listWorkstreamTemplates)
	// Prefix-free aliases for frontend openapi-fetch client
	mux.HandleFunc("GET /project-templates", h.listProjectTemplates)
	mux.HandleFunc("GET /workstream-templates", h.listWorkstreamTemplates)
}

func (h *ProjectTemplatesHandler) listProjectTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := resolveOrgID(r, h.AgentStore, h.OrgSvc)
	templates, err := h.Svc.ListProjectTemplatesExpanded(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

func (h *ProjectTemplatesHandler) listWorkstreamTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := resolveOrgID(r, h.AgentStore, h.OrgSvc)
	templates, err := h.Svc.ListWorkstreamTemplates(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

// resolveOrgID tries to find the org for the authenticated user; returns "" for system-level templates.
func resolveOrgID(r *http.Request, agentStore agent.AgentStore, orgSvc *org.Service) string {
	agentID, ok := r.Context().Value(ContextKeyAgentID).(string)
	if !ok || agentID == "" {
		return ""
	}
	ag, err := agentStore.GetByID(r.Context(), agentID)
	if err != nil || ag == nil || ag.UserID == "" {
		return ""
	}
	orgs, err := orgSvc.ListOrgsForUser(r.Context(), ag.UserID)
	if err != nil || len(orgs) == 0 {
		return ""
	}
	return orgs[0].ID
}
