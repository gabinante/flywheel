package rest

import (
	"encoding/json"
	"net/http"
	"time"

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
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/workflow", h.getOrgWorkflow)
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

	// Workflow library
	mux.HandleFunc("GET /api/v1/workflows/{id}", h.getWorkflowByID)
	mux.HandleFunc("PUT /api/v1/workflows/{id}", h.updateWorkflowByID)
	mux.HandleFunc("GET /api/v1/orgs/{orgID}/workflow-library", h.listWorkflowLibrary)
	mux.HandleFunc("POST /api/v1/orgs/{orgID}/workflow-library", h.createWorkflowLibraryEntry)
	mux.HandleFunc("DELETE /api/v1/orgs/{orgID}/workflow-library/{id}", h.deleteWorkflowLibraryEntry)

	// Prefix-free aliases for the frontend openapi-fetch client
	mux.HandleFunc("GET /projects/{projectID}/workflow", h.getProjectWorkflow)
	mux.HandleFunc("GET /projects/{projectID}/workflow/layers", h.getWorkflowLayers)
	mux.HandleFunc("PUT /projects/{projectID}/workflow", h.upsertProjectWorkflow)
	mux.HandleFunc("DELETE /projects/{projectID}/workflow", h.deleteProjectWorkflow)
	mux.HandleFunc("GET /orgs/{orgID}/workflow", h.getOrgWorkflow)
	mux.HandleFunc("PUT /orgs/{orgID}/workflow", h.upsertOrgWorkflow)
	mux.HandleFunc("DELETE /orgs/{orgID}/workflow", h.deleteOrgWorkflow)
	mux.HandleFunc("GET /orgs/{orgID}/workflow-library", h.listWorkflowLibrary)
	mux.HandleFunc("POST /orgs/{orgID}/workflow-library", h.createWorkflowLibraryEntry)
	mux.HandleFunc("DELETE /orgs/{orgID}/workflow-library/{id}", h.deleteWorkflowLibraryEntry)
	mux.HandleFunc("GET /workflow/system", h.getSystemWorkflow)
	mux.HandleFunc("PUT /workflow/system", h.upsertSystemWorkflow)
	mux.HandleFunc("GET /workflow/templates", h.listTemplates)
	mux.HandleFunc("GET /tickets/{ticketID}/workflow", h.getTicketWorkflow)
	mux.HandleFunc("POST /workflow/callback/{token}", h.handleCallback)
	mux.HandleFunc("POST /gate/callback/{token}", h.handleGateCallback)
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
		suggested := workflow.StandardSDLC()
		writeJSON(w, http.StatusOK, map[string]any{
			"workflow":  nil,
			"suggested": suggested,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workflow": def,
		"source":   def.Scope,
	})
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

func (h *WorkflowHandler) getOrgWorkflow(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	def, err := h.Store.GetByScope(r.Context(), "org", orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if def == nil {
		suggested := workflow.StandardSDLC()
		writeJSON(w, http.StatusOK, map[string]any{
			"workflow":  nil,
			"suggested": suggested,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workflow": def,
		"source":   "org",
	})
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

func (h *WorkflowHandler) listWorkflowLibrary(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")

	// Built-in templates
	builtins := workflow.BuiltinTemplates()
	type libraryEntry struct {
		ID          string           `json:"id"`
		Name        string           `json:"name"`
		Description string           `json:"description,omitempty"`
		Phases      []workflow.Phase `json:"phases"`
		PhaseCount  int              `json:"phase_count"`
		Source      string           `json:"source"`
	}
	var entries []libraryEntry
	for _, b := range builtins {
		entries = append(entries, libraryEntry{
			ID:          "builtin:" + slugify(b.Name),
			Name:        b.Name,
			Description: b.Description,
			Phases:      b.Phases,
			PhaseCount:  len(b.Phases),
			Source:      "builtin",
		})
	}

	// Org library entries
	orgEntries, err := h.Store.ListLibrary(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	for _, e := range orgEntries {
		entries = append(entries, libraryEntry{
			ID:          e.ID,
			Name:        e.Name,
			Description: e.Description,
			Phases:      e.Phases,
			PhaseCount:  len(e.Phases),
			Source:      "library",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (h *WorkflowHandler) createWorkflowLibraryEntry(w http.ResponseWriter, r *http.Request) {
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
	def.ScopeID = orgID
	if err := h.Store.CreateLibraryEntry(r.Context(), &def); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusCreated, def)
}

func (h *WorkflowHandler) deleteWorkflowLibraryEntry(w http.ResponseWriter, r *http.Request) {
	id := PathParam(r, "id")
	if err := h.Store.DeleteLibraryEntry(r.Context(), id); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getWorkflowByID returns any stored definition (library entry, org or project default).
func (h *WorkflowHandler) getWorkflowByID(w http.ResponseWriter, r *http.Request) {
	def, err := h.Store.GetByID(r.Context(), PathParam(r, "id"))
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if def == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, "workflow not found", false))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflow": def, "source": def.Scope})
}

// updateWorkflowByID edits a stored definition's name, description and phases (new version).
func (h *WorkflowHandler) updateWorkflowByID(w http.ResponseWriter, r *http.Request) {
	def, err := h.Store.GetByID(r.Context(), PathParam(r, "id"))
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if def == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, "workflow not found", false))
		return
	}
	var in workflow.Definition
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := validateDefinition(&in); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	def.Name, def.Description, def.Phases = in.Name, in.Description, in.Phases
	if err := h.Store.Update(r.Context(), def); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func slugify(s string) string {
	var result []byte
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c+32)
		case c == ' ':
			result = append(result, '-')
		}
	}
	return string(result)
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
	pos, err := h.Engine.GetPosition(r.Context(), ticketID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
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

func (h *WorkflowHandler) handleGateCallback(w http.ResponseWriter, r *http.Request) {
	token := PathParam(r, "token")
	if h.CallbackHandler == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "callbacks not configured", false))
		return
	}

	// Look up token to get ticket/phase info, then delete it.
	store := h.CallbackHandler.Store()
	ticketID, _, phaseID, err := store.LookupCallbackToken(r.Context(), token)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeNotFound, "invalid or expired gate callback token", false))
		return
	}
	if err := store.DeleteCallbackToken(r.Context(), token); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	// Mark the webhook condition as satisfied by patching ticket outputs.
	key := "_gate_webhook_" + phaseID
	if err := h.TicketSvc.PatchOutputs(r.Context(), ticketID, map[string]any{key: true}); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	// Reset phase status to "ready" so the reconcile loop re-evaluates all conditions.
	if err := h.TicketSvc.UpdateWorkflowPhaseStatus(r.Context(), ticketID, "ready"); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "ticket_id": ticketID, "phase_id": phaseID})
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

		// Validate phase config for type-specific errors.
		if configErrs := workflow.ValidatePhaseConfig(p.Type, p.Config); len(configErrs) > 0 {
			return apierrors.New(apierrors.CodeInvalidInput, "phase "+p.ID+": "+configErrs[0], false)
		}

		// Validate timeout parses as duration if present.
		if p.Timeout != "" {
			if _, err := time.ParseDuration(p.Timeout); err != nil {
				return apierrors.New(apierrors.CodeInvalidInput, "phase "+p.ID+": timeout "+p.Timeout+" is not a valid duration", false)
			}
		}
	}
	return nil
}
