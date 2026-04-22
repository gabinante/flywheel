package rest

import (
	"context"
	"encoding/json"
	"net/http"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/policy"
)

// PolicyServiceForHandler is the interface the handler needs.
type PolicyServiceForHandler interface {
	CreateCalibrationPolicy(ctx context.Context, projectID, name, description, actorID string, rules policy.CalibrationRules, minSample int) (*policy.CalibrationPolicy, error)
	GetCalibrationPolicy(ctx context.Context, id string) (*policy.CalibrationPolicy, error)
	ListCalibrationPolicies(ctx context.Context, projectID string, enabledOnly bool) ([]policy.CalibrationPolicy, error)
	UpdateCalibrationPolicy(ctx context.Context, id, name, description, actorID string, rules policy.CalibrationRules, enabled bool, minSample int) error
	RecordDecision(ctx context.Context, policyID, ticketID string, decision policy.Decision, reason string) (*policy.CalibrationDecision, error)
	RecordOutcome(ctx context.Context, decisionID string, outcome policy.Outcome) error
	RecordOutcomeByTicket(ctx context.Context, ticketID string, outcome policy.Outcome) error
	GetMetrics(ctx context.Context, policyID string) (*policy.Metrics, error)
	GetPolicyHealth(ctx context.Context, policyID string) (*policy.PolicyHealth, error)
	GetProjectHealth(ctx context.Context, projectID string) ([]policy.PolicyHealth, error)
	RunCalibration(ctx context.Context, projectID string) ([]policy.PolicyProposal, error)
	ResolveProposal(ctx context.Context, proposalID string, status policy.ProposalStatus, resolvedBy string) error
	SimulateRuleChange(ctx context.Context, policyID string, candidateRules policy.CalibrationRules, days int) (*policy.SimulationResult, error)
	ListDecisions(ctx context.Context, policyID string, limit int) ([]policy.CalibrationDecision, error)
	ListChangeEvents(ctx context.Context, policyID string, limit int) ([]policy.PolicyChangeEvent, error)
}

// PoliciesHandler handles policy calibration REST endpoints.
type PoliciesHandler struct {
	PolicySvc  PolicyServiceForHandler
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

// RegisterRoutes registers policy routes on the mux.
func (h *PoliciesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects/{projectID}/policies", h.listPolicies)
	mux.HandleFunc("POST /projects/{projectID}/policies", h.createPolicy)
	mux.HandleFunc("GET /projects/{projectID}/policies/health", h.getProjectHealth)
	mux.HandleFunc("POST /projects/{projectID}/policies/calibrate", h.runCalibration)
	mux.HandleFunc("GET /policies/{policyID}", h.getPolicy)
	mux.HandleFunc("PUT /policies/{policyID}", h.updatePolicy)
	mux.HandleFunc("GET /policies/{policyID}/health", h.getPolicyHealth)
	mux.HandleFunc("GET /policies/{policyID}/metrics", h.getMetrics)
	mux.HandleFunc("GET /policies/{policyID}/decisions", h.listDecisions)
	mux.HandleFunc("GET /policies/{policyID}/history", h.listChangeEvents)
	mux.HandleFunc("POST /policies/{policyID}/decisions", h.recordDecision)
	mux.HandleFunc("POST /policies/{policyID}/simulate", h.simulate)
	mux.HandleFunc("POST /decisions/{decisionID}/outcome", h.recordOutcome)
	mux.HandleFunc("POST /tickets/{ticketID}/outcome", h.recordOutcomeByTicket)
	mux.HandleFunc("POST /proposals/{proposalID}/resolve", h.resolveProposal)
}

func (h *PoliciesHandler) listPolicies(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	enabledOnly := r.URL.Query().Get("enabled") == "true"
	list, err := h.PolicySvc.ListCalibrationPolicies(r.Context(), projectID, enabledOnly)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if list == nil {
		list = []policy.CalibrationPolicy{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"policies": list})
}

func (h *PoliciesHandler) createPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Rules       policy.CalibrationRules `json:"rules"`
		MinSample   int          `json:"min_sample"`
		ActorID     string       `json:"actor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.ActorID == "" {
		body.ActorID = "api"
	}
	p, err := h.PolicySvc.CreateCalibrationPolicy(r.Context(), projectID, body.Name, body.Description, body.ActorID, body.Rules, body.MinSample)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PoliciesHandler) getPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *PoliciesHandler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Rules       policy.CalibrationRules `json:"rules"`
		Enabled     bool         `json:"enabled"`
		MinSample   int          `json:"min_sample"`
		ActorID     string       `json:"actor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.ActorID == "" {
		body.ActorID = "api"
	}
	if err := h.PolicySvc.UpdateCalibrationPolicy(r.Context(), policyID, body.Name, body.Description, body.ActorID, body.Rules, body.Enabled, body.MinSample); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *PoliciesHandler) getMetrics(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	m, err := h.PolicySvc.GetMetrics(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}

func (h *PoliciesHandler) getPolicyHealth(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	health, err := h.PolicySvc.GetPolicyHealth(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(health)
}

func (h *PoliciesHandler) getProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	health, err := h.PolicySvc.GetProjectHealth(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if health == nil {
		health = []policy.PolicyHealth{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"policies": health})
}

func (h *PoliciesHandler) runCalibration(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	proposals, err := h.PolicySvc.RunCalibration(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if proposals == nil {
		proposals = []policy.PolicyProposal{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"proposals": proposals})
}

func (h *PoliciesHandler) recordDecision(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		TicketID string          `json:"ticket_id"`
		Decision policy.Decision `json:"decision"`
		Reason   string          `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	d, err := h.PolicySvc.RecordDecision(r.Context(), policyID, body.TicketID, body.Decision, body.Reason)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(d)
}

func (h *PoliciesHandler) recordOutcome(w http.ResponseWriter, r *http.Request) {
	decisionID := PathParam(r, "decisionID")
	var body struct {
		Outcome policy.Outcome `json:"outcome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.PolicySvc.RecordOutcome(r.Context(), decisionID, body.Outcome); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *PoliciesHandler) recordOutcomeByTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	var body struct {
		Outcome policy.Outcome `json:"outcome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.PolicySvc.RecordOutcomeByTicket(r.Context(), ticketID, body.Outcome); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *PoliciesHandler) listDecisions(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	decisions, err := h.PolicySvc.ListDecisions(r.Context(), policyID, 100)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if decisions == nil {
		decisions = []policy.CalibrationDecision{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"decisions": decisions})
}

func (h *PoliciesHandler) listChangeEvents(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	changeEvents, err := h.PolicySvc.ListChangeEvents(r.Context(), policyID, 50)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if changeEvents == nil {
		changeEvents = []policy.PolicyChangeEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": changeEvents})
}

func (h *PoliciesHandler) simulate(w http.ResponseWriter, r *http.Request) {
	policyID := PathParam(r, "policyID")
	p, err := h.PolicySvc.GetCalibrationPolicy(r.Context(), policyID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, p.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		CandidateRules policy.CalibrationRules `json:"candidate_rules"`
		Days           int          `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	result, err := h.PolicySvc.SimulateRuleChange(r.Context(), policyID, body.CandidateRules, body.Days)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *PoliciesHandler) resolveProposal(w http.ResponseWriter, r *http.Request) {
	proposalID := PathParam(r, "proposalID")
	var body struct {
		Status     policy.ProposalStatus `json:"status"`
		ResolvedBy string                `json:"resolved_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.ResolvedBy == "" {
		body.ResolvedBy = "api"
	}
	if err := h.PolicySvc.ResolveProposal(r.Context(), proposalID, body.Status, body.ResolvedBy); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
