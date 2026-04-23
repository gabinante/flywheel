package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/environment"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/plan"
	"github.com/gabinante/flywheel/internal/policy"
)

func (s *StrictServer) CreatePlan(ctx context.Context, req generated.CreatePlanRequestObject) (generated.CreatePlanResponseObject, error) {
	if err := requirePlanService(s.PlanSvc); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	t, err := s.TicketSvc.GetTicket(ctx, req.Body.TicketId)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}

	backend := plan.Backend(req.Body.Backend)
	contentJSON, err := json.Marshal(req.Body.Content)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "invalid content", false)
	}
	content, err := parseContent(backend, contentJSON)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	}

	createdBy := firstNonEmpty(optionalValue(req.Body.CreatedBy), GetAgentID(ctx))
	if createdBy == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "created_by required", false)
	}

	freshnessStamp := time.Now().UTC()
	if req.Body.FreshnessStamp != nil {
		freshnessStamp = *req.Body.FreshnessStamp
	}

	p, err := s.PlanSvc.CreatePlan(ctx, req.Body.TicketId, backend, content, createdBy, freshnessStamp, req.Body.ExpiresAt)
	if err != nil {
		if ve, ok := err.(*plan.ValidationError); ok {
			return nil, apierrors.New(apierrors.CodeInvalidInput, ve.Error(), false)
		}
		return nil, apierrors.MapError(err)
	}

	genPlan, err := convertByJSON[generated.Plan](p)
	if err != nil {
		return nil, err
	}
	return generated.CreatePlan201JSONResponse(genPlan), nil
}

func (s *StrictServer) GetPlan(ctx context.Context, req generated.GetPlanRequestObject) (generated.GetPlanResponseObject, error) {
	p, err := s.authorizedPlan(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	genPlan, err := convertByJSON[generated.Plan](p)
	if err != nil {
		return nil, err
	}
	return generated.GetPlan200JSONResponse(genPlan), nil
}

func (s *StrictServer) UpdatePlanContent(ctx context.Context, req generated.UpdatePlanContentRequestObject) (generated.UpdatePlanContentResponseObject, error) {
	p, err := s.authorizedPlan(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	updatedBy := firstNonEmpty(optionalValue(req.Body.UpdatedBy), GetAgentID(ctx))
	if updatedBy == "" {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "updated_by required", false)
	}

	contentJSON, err := json.Marshal(req.Body.Content)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "invalid content", false)
	}
	content, err := parseContent(p.Backend, contentJSON)
	if err != nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false)
	}

	if err := s.PlanSvc.UpdateContent(ctx, req.PlanID, content, updatedBy); err != nil {
		if ve, ok := err.(*plan.ValidationError); ok {
			return nil, apierrors.New(apierrors.CodeInvalidInput, ve.Error(), false)
		}
		return nil, apierrors.MapError(err)
	}
	return generated.UpdatePlanContent204Response{}, nil
}

func (s *StrictServer) CheckPlanFreshness(ctx context.Context, req generated.CheckPlanFreshnessRequestObject) (generated.CheckPlanFreshnessResponseObject, error) {
	p, err := s.authorizedPlan(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	fresh, err := s.PlanSvc.IsFresh(ctx, req.PlanID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.CheckPlanFreshness200JSONResponse{
		Fresh:  &fresh,
		PlanId: &p.ID,
	}, nil
}

func (s *StrictServer) TransitionPlan(ctx context.Context, req generated.TransitionPlanRequestObject) (generated.TransitionPlanResponseObject, error) {
	if _, err := s.authorizedPlan(ctx, req.PlanID); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	var err error
	switch string(req.Body.Action) {
	case "submit":
		err = s.PlanSvc.SubmitPlan(ctx, req.PlanID)
	case "classify":
		err = s.PlanSvc.ClassifyPlan(ctx, req.PlanID)
	case "approve":
		err = s.PlanSvc.ApprovePlan(ctx, req.PlanID)
	case "apply":
		err = s.PlanSvc.ApplyPlan(ctx, req.PlanID)
	case "reject":
		err = s.PlanSvc.RejectPlan(ctx, req.PlanID)
	default:
		return nil, apierrors.New(apierrors.CodeInvalidInput, "action must be one of: submit, classify, approve, apply, reject", false)
	}
	if err != nil {
		return nil, apierrors.MapError(err)
	}

	p, err := s.PlanSvc.GetPlan(ctx, req.PlanID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genPlan, err := convertByJSON[generated.Plan](p)
	if err != nil {
		return nil, err
	}
	return generated.TransitionPlan200JSONResponse(genPlan), nil
}

func (s *StrictServer) ListPlanVersions(ctx context.Context, req generated.ListPlanVersionsRequestObject) (generated.ListPlanVersionsResponseObject, error) {
	if _, err := s.authorizedPlan(ctx, req.PlanID); err != nil {
		return nil, err
	}
	versions, err := s.PlanSvc.GetVersions(ctx, req.PlanID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if versions == nil {
		versions = []*plan.PlanVersion{}
	}
	genVersions, err := convertByJSON[[]generated.PlanVersion](versions)
	if err != nil {
		return nil, err
	}
	return generated.ListPlanVersions200JSONResponse(genVersions), nil
}

func (s *StrictServer) ListPlansByTicket(ctx context.Context, req generated.ListPlansByTicketRequestObject) (generated.ListPlansByTicketResponseObject, error) {
	if err := requirePlanService(s.PlanSvc); err != nil {
		return nil, err
	}
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}

	var plans []*plan.Plan
	if req.Params.Backend != nil && string(*req.Params.Backend) != "" {
		plans, err = s.PlanSvc.ListPlansByTicketAndBackend(ctx, req.TicketID, plan.Backend(*req.Params.Backend))
	} else {
		plans, err = s.PlanSvc.ListPlansByTicket(ctx, req.TicketID)
	}
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if plans == nil {
		plans = []*plan.Plan{}
	}

	genPlans, err := convertByJSON[[]generated.Plan](plans)
	if err != nil {
		return nil, err
	}
	return generated.ListPlansByTicket200JSONResponse(genPlans), nil
}

func (s *StrictServer) ListEnvironments(ctx context.Context, req generated.ListEnvironmentsRequestObject) (generated.ListEnvironmentsResponseObject, error) {
	if err := requireEnvironmentService(s.EnvSvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}

	envs, err := s.EnvSvc.ListEnvironments(ctx, req.ProjectID)
	if err != nil {
		return nil, envError(err)
	}
	if envs == nil {
		envs = []*environment.Environment{}
	}
	genEnvs, err := convertByJSON[[]generated.Environment](envs)
	if err != nil {
		return nil, err
	}
	return generated.ListEnvironments200JSONResponse(genEnvs), nil
}

func (s *StrictServer) CreateEnvironment(ctx context.Context, req generated.CreateEnvironmentRequestObject) (generated.CreateEnvironmentResponseObject, error) {
	if err := requireEnvironmentService(s.EnvSvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	env, err := s.EnvSvc.CreateEnvironment(
		ctx,
		req.ProjectID,
		req.Body.Name,
		optionalValue(req.Body.Slug),
		environment.Infrastructure(req.Body.Infrastructure),
		environment.DataTenancy(req.Body.DataTenancy),
		environment.IntegrationMode(req.Body.IntegrationMode),
	)
	if err != nil {
		return nil, envError(err)
	}

	genEnv, err := convertByJSON[generated.Environment](env)
	if err != nil {
		return nil, err
	}
	return generated.CreateEnvironment201JSONResponse(genEnv), nil
}

func (s *StrictServer) GetEnvironment(ctx context.Context, req generated.GetEnvironmentRequestObject) (generated.GetEnvironmentResponseObject, error) {
	env, err := s.authorizedEnvironment(ctx, req.EnvironmentID)
	if err != nil {
		return nil, err
	}
	genEnv, err := convertByJSON[generated.Environment](env)
	if err != nil {
		return nil, err
	}
	return generated.GetEnvironment200JSONResponse(genEnv), nil
}

func (s *StrictServer) UpdateEnvironment(ctx context.Context, req generated.UpdateEnvironmentRequestObject) (generated.UpdateEnvironmentResponseObject, error) {
	if _, err := s.authorizedEnvironment(ctx, req.EnvironmentID); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	env, err := s.EnvSvc.UpdateEnvironment(
		ctx,
		req.EnvironmentID,
		optionalValue(req.Body.Name),
		optionalValue(req.Body.Slug),
		environment.Infrastructure(optionalValue(req.Body.Infrastructure)),
		environment.DataTenancy(optionalValue(req.Body.DataTenancy)),
		environment.IntegrationMode(optionalValue(req.Body.IntegrationMode)),
	)
	if err != nil {
		return nil, envError(err)
	}

	genEnv, err := convertByJSON[generated.Environment](env)
	if err != nil {
		return nil, err
	}
	return generated.UpdateEnvironment200JSONResponse(genEnv), nil
}

func (s *StrictServer) DeleteEnvironment(ctx context.Context, req generated.DeleteEnvironmentRequestObject) (generated.DeleteEnvironmentResponseObject, error) {
	if _, err := s.authorizedEnvironment(ctx, req.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.EnvSvc.DeleteEnvironment(ctx, req.EnvironmentID); err != nil {
		return nil, envError(err)
	}
	return generated.DeleteEnvironment204Response{}, nil
}

func (s *StrictServer) SetDefaultEnvironment(ctx context.Context, req generated.SetDefaultEnvironmentRequestObject) (generated.SetDefaultEnvironmentResponseObject, error) {
	if _, err := s.authorizedEnvironment(ctx, req.EnvironmentID); err != nil {
		return nil, err
	}
	if err := s.EnvSvc.SetDefault(ctx, req.EnvironmentID); err != nil {
		return nil, envError(err)
	}
	return generated.SetDefaultEnvironment204Response{}, nil
}

func (s *StrictServer) ListPolicies(ctx context.Context, req generated.ListPoliciesRequestObject) (generated.ListPoliciesResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}

	enabledOnly := req.Params.Enabled != nil && string(*req.Params.Enabled) == "true"
	policies, err := s.PolicySvc.ListCalibrationPolicies(ctx, req.ProjectID, enabledOnly)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genPolicies, err := convertByJSON[[]generated.Policy](policies)
	if err != nil {
		return nil, err
	}
	return generated.ListPolicies200JSONResponse(generated.PolicyListResponse{Policies: &genPolicies}), nil
}

func (s *StrictServer) CreatePolicy(ctx context.Context, req generated.CreatePolicyRequestObject) (generated.CreatePolicyResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	rules := policy.CalibrationRules{}
	var err error
	if req.Body.Rules != nil {
		rules, err = convertByJSON[policy.CalibrationRules](req.Body.Rules)
		if err != nil {
			return nil, err
		}
	}
	actorID := firstNonEmpty(optionalValue(req.Body.ActorId), GetAgentID(ctx), "api")

	p, err := s.PolicySvc.CreateCalibrationPolicy(ctx, req.ProjectID, req.Body.Name, optionalValue(req.Body.Description), actorID, rules, optionalInt(req.Body.MinSample))
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genPolicy, err := convertByJSON[generated.Policy](p)
	if err != nil {
		return nil, err
	}
	return generated.CreatePolicy201JSONResponse(genPolicy), nil
}

func (s *StrictServer) GetPolicy(ctx context.Context, req generated.GetPolicyRequestObject) (generated.GetPolicyResponseObject, error) {
	p, err := s.authorizedPolicy(ctx, req.PolicyID)
	if err != nil {
		return nil, err
	}
	genPolicy, err := convertByJSON[generated.Policy](p)
	if err != nil {
		return nil, err
	}
	return generated.GetPolicy200JSONResponse(genPolicy), nil
}

func (s *StrictServer) UpdatePolicy(ctx context.Context, req generated.UpdatePolicyRequestObject) (generated.UpdatePolicyResponseObject, error) {
	current, err := s.authorizedPolicy(ctx, req.PolicyID)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	name := current.Name
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	description := current.Description
	if req.Body.Description != nil {
		description = *req.Body.Description
	}
	rules := current.Rules
	if req.Body.Rules != nil {
		rules, err = convertByJSON[policy.CalibrationRules](req.Body.Rules)
		if err != nil {
			return nil, err
		}
	}
	enabled := current.Enabled
	if req.Body.Enabled != nil {
		enabled = *req.Body.Enabled
	}
	minSample := current.MinSample
	if req.Body.MinSample != nil {
		minSample = *req.Body.MinSample
	}
	actorID := firstNonEmpty(optionalValue(req.Body.ActorId), GetAgentID(ctx), "api")

	if err := s.PolicySvc.UpdateCalibrationPolicy(ctx, req.PolicyID, name, description, actorID, rules, enabled, minSample); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.UpdatePolicy200Response{}, nil
}

func (s *StrictServer) ListPolicyDecisions(ctx context.Context, req generated.ListPolicyDecisionsRequestObject) (generated.ListPolicyDecisionsResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	decisions, err := s.PolicySvc.ListDecisions(ctx, req.PolicyID, 100)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genDecisions, err := convertByJSON[[]generated.PolicyDecision](decisions)
	if err != nil {
		return nil, err
	}
	return generated.ListPolicyDecisions200JSONResponse(generated.PolicyDecisionsResponse{Decisions: &genDecisions}), nil
}

func (s *StrictServer) RecordPolicyDecision(ctx context.Context, req generated.RecordPolicyDecisionRequestObject) (generated.RecordPolicyDecisionResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	decision, err := s.PolicySvc.RecordDecision(ctx, req.PolicyID, req.Body.TicketId, policy.Decision(req.Body.Decision), optionalValue(req.Body.Reason))
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genDecision, err := convertByJSON[generated.PolicyDecision](decision)
	if err != nil {
		return nil, err
	}
	return generated.RecordPolicyDecision201JSONResponse(genDecision), nil
}

func (s *StrictServer) GetPolicyHealth(ctx context.Context, req generated.GetPolicyHealthRequestObject) (generated.GetPolicyHealthResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	health, err := s.PolicySvc.GetPolicyHealth(ctx, req.PolicyID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genHealth, err := convertByJSON[generated.PolicyHealth](health)
	if err != nil {
		return nil, err
	}
	return generated.GetPolicyHealth200JSONResponse(genHealth), nil
}

func (s *StrictServer) ListPolicyChangeEvents(ctx context.Context, req generated.ListPolicyChangeEventsRequestObject) (generated.ListPolicyChangeEventsResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	events, err := s.PolicySvc.ListChangeEvents(ctx, req.PolicyID, 50)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genEvents, err := convertByJSON[[]generated.PolicyChangeEvent](events)
	if err != nil {
		return nil, err
	}
	return generated.ListPolicyChangeEvents200JSONResponse(generated.PolicyChangeEventsResponse{Events: &genEvents}), nil
}

func (s *StrictServer) GetPolicyMetrics(ctx context.Context, req generated.GetPolicyMetricsRequestObject) (generated.GetPolicyMetricsResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	metrics, err := s.PolicySvc.GetMetrics(ctx, req.PolicyID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genMetrics, err := convertByJSON[generated.PolicyMetrics](metrics)
	if err != nil {
		return nil, err
	}
	return generated.GetPolicyMetrics200JSONResponse(genMetrics), nil
}

func (s *StrictServer) SimulateRuleChange(ctx context.Context, req generated.SimulateRuleChangeRequestObject) (generated.SimulateRuleChangeResponseObject, error) {
	if _, err := s.authorizedPolicy(ctx, req.PolicyID); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}

	candidateRules, err := convertByJSON[policy.CalibrationRules](req.Body.CandidateRules)
	if err != nil {
		return nil, err
	}
	result, err := s.PolicySvc.SimulateRuleChange(ctx, req.PolicyID, candidateRules, optionalInt(req.Body.Days))
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genResult, err := convertByJSON[generated.SimulationResult](result)
	if err != nil {
		return nil, err
	}
	return generated.SimulateRuleChange200JSONResponse(genResult), nil
}

func (s *StrictServer) RunCalibration(ctx context.Context, req generated.RunCalibrationRequestObject) (generated.RunCalibrationResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	proposals, err := s.PolicySvc.RunCalibration(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genProposals, err := convertByJSON[[]generated.PolicyProposal](proposals)
	if err != nil {
		return nil, err
	}
	return generated.RunCalibration200JSONResponse(generated.CalibrationResponse{Proposals: &genProposals}), nil
}

func (s *StrictServer) GetProjectPolicyHealth(ctx context.Context, req generated.GetProjectPolicyHealthRequestObject) (generated.GetProjectPolicyHealthResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if err := CheckProjectAccess(ctx, req.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	health, err := s.PolicySvc.GetProjectHealth(ctx, req.ProjectID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	genHealth, err := convertByJSON[[]generated.PolicyHealth](health)
	if err != nil {
		return nil, err
	}
	return generated.GetProjectPolicyHealth200JSONResponse(generated.ProjectHealthResponse{Policies: &genHealth}), nil
}

func (s *StrictServer) ResolveProposal(ctx context.Context, req generated.ResolveProposalRequestObject) (generated.ResolveProposalResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}
	resolvedBy := firstNonEmpty(optionalValue(req.Body.ResolvedBy), GetAgentID(ctx), "api")
	if err := s.PolicySvc.ResolveProposal(ctx, req.ProposalID, policy.ProposalStatus(req.Body.Status), resolvedBy); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.ResolveProposal200Response{}, nil
}

func (s *StrictServer) RecordTicketOutcome(ctx context.Context, req generated.RecordTicketOutcomeRequestObject) (generated.RecordTicketOutcomeResponseObject, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apierrors.New(apierrors.CodeInvalidInput, "body required", false)
	}
	t, err := s.TicketSvc.GetTicket(ctx, req.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	if err := s.PolicySvc.RecordOutcomeByTicket(ctx, req.TicketID, policy.Outcome(req.Body.Outcome)); err != nil {
		return nil, apierrors.MapError(err)
	}
	return generated.RecordTicketOutcome200Response{}, nil
}

func (s *StrictServer) authorizedPlan(ctx context.Context, planID string) (*plan.Plan, error) {
	if err := requirePlanService(s.PlanSvc); err != nil {
		return nil, err
	}
	p, err := s.PlanSvc.GetPlan(ctx, planID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	t, err := s.TicketSvc.GetTicket(ctx, p.TicketID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, t.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *StrictServer) authorizedEnvironment(ctx context.Context, environmentID string) (*environment.Environment, error) {
	if err := requireEnvironmentService(s.EnvSvc); err != nil {
		return nil, err
	}
	env, err := s.EnvSvc.GetEnvironment(ctx, environmentID)
	if err != nil {
		return nil, envError(err)
	}
	if err := CheckProjectAccess(ctx, env.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	return env, nil
}

func (s *StrictServer) authorizedPolicy(ctx context.Context, policyID string) (*policy.CalibrationPolicy, error) {
	if err := requirePolicyService(s.PolicySvc); err != nil {
		return nil, err
	}
	p, err := s.PolicySvc.GetCalibrationPolicy(ctx, policyID)
	if err != nil {
		return nil, apierrors.MapError(err)
	}
	if err := CheckProjectAccess(ctx, p.ProjectID, s.AgentStore, s.OrgSvc, s.ProjectSvc); err != nil {
		return nil, err
	}
	return p, nil
}

func convertByJSON[T any](src any) (T, error) {
	var dst T
	raw, err := json.Marshal(src)
	if err != nil {
		return dst, apierrors.New(apierrors.CodeInternal, fmt.Sprintf("marshal %T: %v", src, err), true)
	}
	if err := json.Unmarshal(raw, &dst); err != nil {
		return dst, apierrors.New(apierrors.CodeInternal, fmt.Sprintf("unmarshal into %T: %v", dst, err), true)
	}
	return dst, nil
}

func requirePlanService(svc *plan.Service) error {
	if svc == nil {
		return apierrors.New(apierrors.CodeInternal, "plan service unavailable", true)
	}
	return nil
}

func requireEnvironmentService(svc *environment.Service) error {
	if svc == nil {
		return apierrors.New(apierrors.CodeInternal, "environment service unavailable", true)
	}
	return nil
}

func requirePolicyService(svc PolicyServiceForHandler) error {
	if svc == nil {
		return apierrors.New(apierrors.CodeInternal, "policy service unavailable", true)
	}
	return nil
}

func optionalValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
