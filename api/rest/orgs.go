package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// OrgsHandler handles org and project-under-org REST endpoints.
type OrgsHandler struct {
	OrgSvc     *org.Service
	ProjectSvc *project.Service
	AgentStore *agent.Store
}

func (h *OrgsHandler) createOrg(w http.ResponseWriter, r *http.Request) {
	agentID := GetAgentID(r.Context())
	if agentID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "authentication required to create an organization", false))
		return
	}
	agent, err := h.AgentStore.GetByID(r.Context(), agentID)
	if err != nil || agent == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	// When the agent has OAuth (UserID set), we create the org and add them as owner so they see it in list orgs and pass access checks. When the agent is API-key-only (UserID empty), we create the org with no membership: the creator is not in org_members and will not see this org via ListOrgsForUser or EnsureOrgAccess. Use OAuth-linked agents for normal org/project workflows; API-key-only creation is for automation where the caller may not need to list or access the org via membership.
	var o *org.Org
	if agent.UserID != "" {
		o, err = h.OrgSvc.CreateOrgWithOwner(r.Context(), body.Name, body.Slug, agent.UserID)
	} else {
		o, err = h.OrgSvc.CreateOrg(r.Context(), body.Name, body.Slug)
	}
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, err.Error(), false))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(o)
}

func (h *OrgsHandler) listOrgs(w http.ResponseWriter, r *http.Request) {
	agentID := GetAgentID(r.Context())
	if agentID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "authentication required to list organizations", false))
		return
	}
	agent, err := h.AgentStore.GetByID(r.Context(), agentID)
	if err != nil || agent == nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
		return
	}
	if agent.UserID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "list orgs requires OAuth (agent must be linked to a user)", false))
		return
	}
	orgs, err := h.OrgSvc.ListOrgsForUser(r.Context(), agent.UserID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if orgs == nil {
		orgs = []*org.Org{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orgs)
}

func (h *OrgsHandler) getOrg(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	if !EnsureOrgAccess(r.Context(), w, orgID, h.AgentStore, h.OrgSvc) {
		return
	}
	o, err := h.OrgSvc.GetOrg(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(o)
}

func (h *OrgsHandler) createProject(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	if !EnsureOrgAccess(r.Context(), w, orgID, h.AgentStore, h.OrgSvc) {
		return
	}
	var body struct {
		Name      string   `json:"name"`
		Slug      string   `json:"slug"`
		RepoURL   string   `json:"repo_url"`
		TechStack []string `json:"tech_stack"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	p, err := h.ProjectSvc.CreateProject(r.Context(), orgID, body.Name, body.Slug, body.RepoURL, body.TechStack)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *OrgsHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	if !EnsureOrgAccess(r.Context(), w, orgID, h.AgentStore, h.OrgSvc) {
		return
	}
	statusFilter := r.URL.Query().Get("status")
	if statusFilter != "active" && statusFilter != "closed" && statusFilter != "all" {
		statusFilter = "active"
	}
	list, err := h.ProjectSvc.ListByOrgID(r.Context(), orgID, statusFilter)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}
