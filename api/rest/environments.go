package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/environment"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// EnvironmentsHandler handles environment CRUD REST endpoints.
type EnvironmentsHandler struct {
	EnvSvc     *environment.Service
	ProjectSvc *project.Service
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

// RegisterRoutes registers environment endpoints on the mux.
func (h *EnvironmentsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects/{projectID}/environments", h.listEnvironments)
	mux.HandleFunc("POST /projects/{projectID}/environments", h.createEnvironment)
	mux.HandleFunc("GET /environments/{environmentID}", h.getEnvironment)
	mux.HandleFunc("PUT /environments/{environmentID}", h.updateEnvironment)
	mux.HandleFunc("DELETE /environments/{environmentID}", h.deleteEnvironment)
	mux.HandleFunc("POST /environments/{environmentID}/set-default", h.setDefault)
}

func (h *EnvironmentsHandler) listEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	envs, err := h.EnvSvc.ListEnvironments(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, err.Error(), true))
		return
	}
	if envs == nil {
		envs = []*environment.Environment{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(envs)
}

func (h *EnvironmentsHandler) createEnvironment(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		Infrastructure  string `json:"infrastructure"`
		DataTenancy     string `json:"data_tenancy"`
		IntegrationMode string `json:"integration_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.Name == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "name is required", false))
		return
	}
	if body.Infrastructure == "" || body.DataTenancy == "" || body.IntegrationMode == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "infrastructure, data_tenancy, and integration_mode are required", false))
		return
	}

	env, err := h.EnvSvc.CreateEnvironment(r.Context(), projectID,
		body.Name, body.Slug,
		environment.Infrastructure(body.Infrastructure),
		environment.DataTenancy(body.DataTenancy),
		environment.IntegrationMode(body.IntegrationMode))
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(env)
}

func (h *EnvironmentsHandler) getEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := PathParam(r, "environmentID")
	env, err := h.EnvSvc.GetEnvironment(r.Context(), envID)
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, env.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

func (h *EnvironmentsHandler) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := PathParam(r, "environmentID")
	env, err := h.EnvSvc.GetEnvironment(r.Context(), envID)
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, env.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	var body struct {
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		Infrastructure  string `json:"infrastructure"`
		DataTenancy     string `json:"data_tenancy"`
		IntegrationMode string `json:"integration_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}

	updated, err := h.EnvSvc.UpdateEnvironment(r.Context(), envID,
		body.Name, body.Slug,
		environment.Infrastructure(body.Infrastructure),
		environment.DataTenancy(body.DataTenancy),
		environment.IntegrationMode(body.IntegrationMode))
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *EnvironmentsHandler) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	envID := PathParam(r, "environmentID")
	env, err := h.EnvSvc.GetEnvironment(r.Context(), envID)
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, env.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	if err := h.EnvSvc.DeleteEnvironment(r.Context(), envID); err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *EnvironmentsHandler) setDefault(w http.ResponseWriter, r *http.Request) {
	envID := PathParam(r, "environmentID")
	env, err := h.EnvSvc.GetEnvironment(r.Context(), envID)
	if err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, env.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	if err := h.EnvSvc.SetDefault(r.Context(), envID); err != nil {
		WriteStructuredError(w, envError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// envError maps environment package errors to structured API errors.
func envError(err error) *apierrors.StructuredError {
	switch err {
	case environment.ErrNotFound:
		return apierrors.New(apierrors.CodeNotFound, err.Error(), false)
	case environment.ErrNameRequired, environment.ErrSlugRequired,
		environment.ErrInvalidInfrastructure, environment.ErrInvalidDataTenancy,
		environment.ErrInvalidIntegrationMode, environment.ErrDuplicateSlug:
		return apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	case environment.ErrMinimumEnvironments, environment.ErrCannotDeleteDefault,
		environment.ErrLastEnvironment:
		return apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	default:
		return apierrors.New(apierrors.CodeInternal, err.Error(), true)
	}
}
