package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workflow"
)

// WorkflowHandler handles workflow REST endpoints.
type WorkflowHandler struct {
	Engine          *workflow.Engine
	Store           *workflow.Store
	TicketSvc       *ticket.Service
	ProjectSvc      *project.Service
	OrgSvc          *org.Service
	AgentStore      agent.AgentStore
	CallbackHandler *workflow.CallbackHandler
}

// RegisterRoutes registers workflow REST routes.
func (h *WorkflowHandler) RegisterRoutes(mux *http.ServeMux) {
	// Project-level workflow
	mux.HandleFunc("GET /api/v1/projects/{projectID}/workflow", h.getProjectWorkflow)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/workflow/layers", h.getWorkflowLayers)
	mux.HandleFunc("PUT /api/v1/projects/{projectID}/workflow", h.upsertProjectWorkflow)
	mux.HandleFunc("DELETE /api/v1/projects/{projectID}/workflow", h.deleteProjectWorkflow)

	// Org-level workflow
	mux.HandleFunc("PUT /api/v1/orgs/{orgID}/workflow", h.upsertOrgWorkflow)
	mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/workflow", h.deleteOrgWorkflow)

	// System-level workflow
	mux.HandleFunc("GET /api/v1/workflow/system", h.getSystemWorkflow)
	mux.HandleFunc("PUT /api/v1/workflow/system", h.upsertSystemWorkflow)

	// Templates
	mux.HandleFunc("GET /api/v1/workflow/templates", h.listTemplates)

	// Ticket workflow position
	mux.HandleFunc("GET /api/v1/tickets/{ticketID}/workflow", h.getTicketWorkflow)

	// Workflow callback (external async phases)
	mux.HandleFunc("POST /api/v1/workflow/callback/{token}", h.handleCallback)
}

func (h *WorkflowHandler) getProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	def, err := h.Engine.Resolve(r.Context(), proj.OrgID, projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if def == nil {
		writeJSON(w, http.StatusOK, map[string]any{"workflow": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflow": def})
}

func (h *WorkflowHandler) getWorkflowLayers(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	layers := map[string]any{}
	if sys, err := h.Store.GetByScope(r.Context(), "system", ""); err == nil {
		layers["system"] = sys
	}
	if proj.OrgID != "" {
		if orgDef, err := h.Store.GetByScope(r.Context(), "org", proj.OrgID); err == nil {
			layers["org"] = orgDef
		}
	}
	if projDef, err := h.Store.GetByScope(r.Context(), "project", projectID); err == nil {
		layers["project"] = projDef
	}
	writeJSON(w, http.StatusOK, layers)
}

func (h *WorkflowHandler) upsertProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var def workflow.Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := validateDefinition(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	def.Scope = "project"
	def.ScopeID = projectID

	// Deactivate any existing project-level workflows, then create/activate new one.
	if err := h.Store.DeactivateScope(r.Context(), "project", projectID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	def.IsActive = true
	if err := h.Store.Create(r.Context(), &def); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *WorkflowHandler) deleteProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	if err := h.Store.DeactivateScope(r.Context(), "project", projectID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkflowHandler) upsertOrgWorkflow(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	var def workflow.Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := validateDefinition(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	def.Scope = "org"
	def.ScopeID = orgID

	if err := h.Store.DeactivateScope(r.Context(), "org", orgID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	def.IsActive = true
	if err := h.Store.Create(r.Context(), &def); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *WorkflowHandler) deleteOrgWorkflow(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	if err := h.Store.DeactivateScope(r.Context(), "org", orgID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkflowHandler) getSystemWorkflow(w http.ResponseWriter, r *http.Request) {
	def, err := h.Store.GetByScope(r.Context(), "system", "")
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflow": def})
}

func (h *WorkflowHandler) upsertSystemWorkflow(w http.ResponseWriter, r *http.Request) {
	var def workflow.Definition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := validateDefinition(&def); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	def.Scope = "system"
	def.ScopeID = ""

	if err := h.Store.DeactivateScope(r.Context(), "system", ""); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	def.IsActive = true
	if err := h.Store.Create(r.Context(), &def); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (h *WorkflowHandler) listTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, workflow.BuiltinTemplates())
}

func (h *WorkflowHandler) getTicketWorkflow(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	if t.WorkflowID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"position": nil})
		return
	}
	pos, err := h.Engine.GetPosition(r.Context(), ticketID, t.WorkflowID, t.WorkflowPhase)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"position": pos})
}

func (h *WorkflowHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	token := PathParam(r, "token")
	if h.CallbackHandler == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "callbacks not configured", false))
		return
	}

	var body struct {
		Outcome  string         `json:"outcome"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}

	if err := h.CallbackHandler.HandleCallback(r.Context(), token, body.Outcome, body.Metadata); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func validateDefinition(def *workflow.Definition) error {
	if def.Name == "" {
		return apierrors.New(apierrors.CodeInvalidInput, "name required", false)
	}
	if len(def.Phases) == 0 {
		return apierrors.New(apierrors.CodeInvalidInput, "at least one phase required", false)
	}
	seen := make(map[string]bool)
	for _, p := range def.Phases {
		if p.ID == "" || p.Name == "" {
			return apierrors.New(apierrors.CodeInvalidInput, "each phase needs id and name", false)
		}
		if !workflow.IsValidPhaseType(p.Type) {
			return apierrors.New(apierrors.CodeInvalidInput, "invalid phase type: "+string(p.Type), false)
		}
		if seen[p.ID] {
			return apierrors.New(apierrors.CodeInvalidInput, "duplicate phase id: "+p.ID, false)
		}
		seen[p.ID] = true
	}
	return nil
}
