package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// =============================================================================
// Posture Service: composable rules, evaluation, preview, posture management
// =============================================================================

// TicketGetter retrieves tickets for policy evaluation. Implemented by ticket.Service.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (TicketInfo, error)
}

// TicketLister lists tickets for policy preview. Implemented by a wrapper around ticket.Service.
type TicketLister interface {
	ListRecentTickets(ctx context.Context, projectID string, days int) ([]TicketInfo, error)
}

// TicketInfo is the subset of ticket data needed by the policy engine.
// Defined here to avoid circular imports with the ticket package.
type TicketInfo struct {
	ID          string
	ProjectID   string
	State       string
	Environment string
	Type        string
	Priority    int
	Services    []string // extracted from ticket context/metadata
}

// PostureService provides policy management, evaluation, and preview.
type PostureService struct {
	store  PostureStore
	engine *Engine
	bus    events.Bus
}

// NewPostureService returns a new PostureService.
func NewPostureService(store PostureStore, bus events.Bus) *PostureService {
	return &PostureService{
		store:  store,
		engine: NewEngine(),
		bus:    bus,
	}
}

// --- CRUD Operations ---

// CreatePolicySet creates a new policy set for a project.
// If fromPosture is non-empty, the rules and credential scopes are populated from the named posture.
func (s *PostureService) CreatePolicySet(ctx context.Context, projectID, name, description, fromPosture, changedBy string) (*PolicySet, error) {
	ps := &PolicySet{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		Rules:       []Rule{},
		CredentialScopes: CredentialScopes{
			CodeAccess:  CredReadOnly,
			DeployCreds: CredNone,
			InfraCreds:  CredNone,
			SecretAccess: CredNone,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if fromPosture != "" {
		posture := GetPosture(fromPosture)
		if posture == nil {
			return nil, fmt.Errorf("unknown posture %q", fromPosture)
		}
		ps.Posture = fromPosture
		ps.Rules = posture.Rules
		ps.CredentialScopes = posture.CredentialScopes
		if description == "" {
			ps.Description = posture.Description
		}
	}

	if err := ps.Validate(); err != nil {
		return nil, fmt.Errorf("invalid policy set: %w", err)
	}

	if err := s.store.CreatePolicySet(ctx, ps); err != nil {
		return nil, err
	}

	s.emitPolicyChange(ctx, projectID, ps.ID, ChangeCreated, changedBy, nil, ps.Rules)
	return ps, nil
}

// GetActivePolicySet returns the active policy set for a project, or nil if none.
func (s *PostureService) GetActivePolicySet(ctx context.Context, projectID string) (*PolicySet, error) {
	return s.store.GetActivePolicySet(ctx, projectID)
}

// GetPolicySet returns a policy set by ID.
func (s *PostureService) GetPolicySet(ctx context.Context, id string) (*PolicySet, error) {
	return s.store.GetPolicySet(ctx, id)
}

// ListPolicySets returns all policy sets for a project.
func (s *PostureService) ListPolicySets(ctx context.Context, projectID string) ([]*PolicySet, error) {
	return s.store.ListPolicySets(ctx, projectID)
}

// ActivatePolicySet makes the given policy set the active one for its project.
// Deactivates any previously active set. Emits change events for both.
func (s *PostureService) ActivatePolicySet(ctx context.Context, policySetID, changedBy string) error {
	ps, err := s.store.GetPolicySet(ctx, policySetID)
	if err != nil {
		return err
	}

	// Deactivate current active set (if any).
	current, err := s.store.GetActivePolicySet(ctx, ps.ProjectID)
	if err != nil {
		return err
	}
	if current != nil && current.ID != policySetID {
		if err := s.store.SetActive(ctx, current.ID, false); err != nil {
			return err
		}
		s.emitPolicyChange(ctx, ps.ProjectID, current.ID, ChangeDeactivated, changedBy, current.Rules, nil)
	}

	if err := s.store.SetActive(ctx, policySetID, true); err != nil {
		return err
	}
	s.emitPolicyChange(ctx, ps.ProjectID, policySetID, ChangeActivated, changedBy, nil, ps.Rules)
	return nil
}

// UpdateRules replaces the rules on a policy set. Emits a change event.
func (s *PostureService) UpdateRules(ctx context.Context, policySetID string, rules []Rule, changedBy string) error {
	ps, err := s.store.GetPolicySet(ctx, policySetID)
	if err != nil {
		return err
	}

	// Validate new rules.
	for i, r := range rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("rule[%d] %q: %w", i, r.Name, err)
		}
	}

	oldRules := ps.Rules
	if err := s.store.UpdateRules(ctx, policySetID, rules); err != nil {
		return err
	}

	s.emitPolicyChange(ctx, ps.ProjectID, policySetID, ChangeUpdated, changedBy, oldRules, rules)
	return nil
}

// DeletePolicySet deletes a policy set. Cannot delete an active policy set.
func (s *PostureService) DeletePolicySet(ctx context.Context, policySetID, changedBy string) error {
	ps, err := s.store.GetPolicySet(ctx, policySetID)
	if err != nil {
		return err
	}
	if ps.IsActive {
		return fmt.Errorf("cannot delete active policy set; deactivate first")
	}
	if err := s.store.DeletePolicySet(ctx, policySetID); err != nil {
		return err
	}
	s.emitPolicyChange(ctx, ps.ProjectID, policySetID, ChangeDeleted, changedBy, ps.Rules, nil)
	return nil
}

// --- Evaluation ---

// EvaluateTransition evaluates the active policy for a project against a transition context.
// Returns the policy decision including which rules matched and the effective action.
func (s *PostureService) EvaluateTransition(ctx context.Context, projectID string, tctx TransitionContext) (*PolicyDecision, error) {
	ps, err := s.store.GetActivePolicySet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if ps == nil {
		// No active policy: conservative default.
		decision := PolicyDecision{
			Action: ActionApprove,
			EffectiveRule: &MatchedRule{
				RuleID:   "_no_policy",
				RuleName: "no-active-policy",
				Action:   ActionApprove,
				Reason:   "no active policy set for project; conservative default requires approval",
			},
		}
		return &decision, nil
	}

	decision := s.engine.Evaluate(ps.Rules, tctx)
	return &decision, nil
}

// GetEffectivePolicy returns the complete policy view for a specific ticket,
// showing all gates it will encounter on its remaining happy path.
func (s *PostureService) GetEffectivePolicy(ctx context.Context, projectID string, ticketInfo TicketInfo) (*EffectivePolicy, error) {
	ps, err := s.store.GetActivePolicySet(ctx, projectID)
	if err != nil {
		return nil, err
	}

	ep := &EffectivePolicy{
		TicketID: ticketInfo.ID,
	}

	if ps == nil {
		// No active policy.
		return ep, nil
	}

	ep.PolicySetID = ps.ID
	ep.PostureName = ps.Posture

	// Build the base transition context from ticket info.
	baseCtx := TransitionContext{
		Environment:    ticketInfo.Environment,
		TicketType:     ticketInfo.Type,
		TicketPriority: ticketInfo.Priority,
		Services:       ticketInfo.Services,
	}

	// Compute remaining happy path from current state.
	happyPath := RemainingHappyPath(ticketInfo.State)

	// Evaluate gates for each remaining transition.
	ep.Gates = s.engine.EvaluateGates(ps.Rules, baseCtx, happyPath)

	// Set current gate (first gate in remaining path).
	if len(ep.Gates) > 0 {
		ep.CurrentGate = &ep.Gates[0]
	}

	return ep, nil
}

// --- Preview ---

// PreviewPolicyChange simulates candidate rules against recent historical tickets.
// Returns a list of diffs showing what would change.
func (s *PostureService) PreviewPolicyChange(ctx context.Context, projectID string, candidateRules []Rule, lister TicketLister, days int) ([]PreviewResult, error) {
	if days <= 0 {
		days = 30
	}

	// Get current active rules.
	currentPS, err := s.store.GetActivePolicySet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var currentRules []Rule
	if currentPS != nil {
		currentRules = currentPS.Rules
	}

	// Get recent tickets.
	tickets, err := lister.ListRecentTickets(ctx, projectID, days)
	if err != nil {
		return nil, err
	}

	var results []PreviewResult

	// For each ticket, simulate all happy-path transitions with both old and new rules.
	for _, t := range tickets {
		baseCtx := TransitionContext{
			Environment:    t.Environment,
			TicketType:     t.Type,
			TicketPriority: t.Priority,
			Services:       t.Services,
		}

		for _, step := range FullHappyPath() {
			stepCtx := baseCtx
			stepCtx.Transition = step.Trigger

			oldDecision := s.engine.Evaluate(currentRules, stepCtx)
			newDecision := s.engine.Evaluate(candidateRules, stepCtx)

			if oldDecision.Action != newDecision.Action {
				delta := "unchanged"
				if newDecision.Action.Restrictiveness() > oldDecision.Action.Restrictiveness() {
					delta = "stricter"
				} else if newDecision.Action.Restrictiveness() < oldDecision.Action.Restrictiveness() {
					delta = "looser"
				}
				results = append(results, PreviewResult{
					TicketID:   t.ID,
					Transition: step.Trigger,
					OldAction:  oldDecision.Action,
					NewAction:  newDecision.Action,
					Delta:      delta,
				})
			}
		}
	}

	return results, nil
}

// --- Postures ---

// ApplyPosture creates a new policy set from a named posture and activates it.
// This is the primary method for first-run setup.
func (s *PostureService) ApplyPosture(ctx context.Context, projectID, postureName, changedBy string) (*PolicySet, error) {
	posture := GetPosture(postureName)
	if posture == nil {
		return nil, fmt.Errorf("unknown posture %q; available: plan-only, sandbox, prod-gate, graduated-risk, paranoid-service", postureName)
	}

	ps, err := s.CreatePolicySet(ctx, projectID, posture.DisplayName, posture.Description, postureName, changedBy)
	if err != nil {
		return nil, err
	}

	if err := s.ActivatePolicySet(ctx, ps.ID, changedBy); err != nil {
		return nil, err
	}

	return ps, nil
}

// --- Event Emission ---

func (s *PostureService) emitPolicyChange(ctx context.Context, projectID, policySetID string, changeType ChangeType, changedBy string, oldRules, newRules []Rule) {
	change := PolicyChange{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   projectID,
		PolicySetID: policySetID,
		ChangeType:  changeType,
		ChangedBy:   changedBy,
		CreatedAt:   time.Now().UTC(),
	}
	if oldRules != nil {
		change.OldRules, _ = json.Marshal(oldRules)
	}
	if newRules != nil {
		change.NewRules, _ = json.Marshal(newRules)
	}

	// Store the change record.
	_ = s.store.RecordPolicyChange(ctx, &change)

	// Publish to event bus.
	_ = s.bus.Publish(ctx, events.Event{
		Type: events.EventPolicyChanged,
		Payload: map[string]any{
			"project_id":    projectID,
			"policy_set_id": policySetID,
			"change_type":   string(changeType),
			"changed_by":    changedBy,
		},
	})
}

// --- Happy Path Computation ---

// RemainingHappyPath returns the transitions remaining on the happy path
// from the given state to closed.
func RemainingHappyPath(currentState string) []PathStep {
	full := FullHappyPath()
	var remaining []PathStep
	found := false
	for _, step := range full {
		if step.FromState == currentState {
			found = true
		}
		if found {
			remaining = append(remaining, step)
		}
	}
	return remaining
}

// FullHappyPath returns the complete happy-path transitions from draft to closed.
func FullHappyPath() []PathStep {
	return []PathStep{
		{Trigger: "spec", FromState: "draft", ToState: "specced"},
		{Trigger: "plan", FromState: "specced", ToState: "planning"},
		{Trigger: "start", FromState: "planning", ToState: "executing"},
		{Trigger: "submit", FromState: "executing", ToState: "awaiting_validation"},
		{Trigger: "approve", FromState: "awaiting_validation", ToState: "validated"},
		{Trigger: "deploy", FromState: "validated", ToState: "deploying"},
		{Trigger: "observe", FromState: "deploying", ToState: "observing"},
		{Trigger: "close", FromState: "observing", ToState: "closed"},
	}
}

// =============================================================================
// Calibration Service: decision tracking, outcome recording, metrics, proposals
// =============================================================================

// Thresholds for trend analysis.
const (
	// GoodTrendMaxRollbackRate: rollback rate must be below this to propose broadening.
	GoodTrendMaxRollbackRate = 0.05
	// GoodTrendMaxIncidentRate: incident rate must be zero for broadening proposals.
	GoodTrendMaxIncidentRate = 0.0
	// GoodTrendMinSuccessRate: success rate must be above this for broadening.
	GoodTrendMinSuccessRate = 0.90
	// BadTrendMinRollbackRate: rollback rate above this triggers review flagging.
	BadTrendMinRollbackRate = 0.15
	// BadTrendMinIncidentRate: any incident rate above zero triggers review flagging.
	BadTrendMinIncidentRate = 0.01
)

// CalibrationService provides policy calibration operations.
type CalibrationService struct {
	store *CalibrationStore
	bus   events.Bus
}

// NewCalibrationService returns a new CalibrationService.
func NewCalibrationService(store *CalibrationStore, bus events.Bus) *CalibrationService {
	svc := &CalibrationService{store: store, bus: bus}

	// Subscribe to ticket lifecycle events to record outcomes.
	bus.Subscribe(events.EventTicketClosed, func(ctx context.Context, ev events.Event) {
		ticketID, _ := ev.Payload["ticket_id"].(string)
		if ticketID != "" {
			_ = store.RecordOutcomeByTicket(ctx, ticketID, OutcomeSuccess)
		}
	})

	return svc
}

// CreateCalibrationPolicy creates a new policy and records a change event.
func (s *CalibrationService) CreateCalibrationPolicy(ctx context.Context, projectID, name, description, actorID string, rules CalibrationRules, minSample int) (*CalibrationPolicy, error) {
	if name == "" {
		return nil, fmt.Errorf("policy name required")
	}
	if minSample <= 0 {
		minSample = 20
	}
	now := time.Now().UTC()
	p := &CalibrationPolicy{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		Rules:       rules,
		Enabled:     true,
		MinSample:   minSample,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreatePolicy(ctx, p); err != nil {
		return nil, err
	}

	// Record change event
	_ = s.store.RecordChangeEvent(ctx, &PolicyChangeEvent{
		PolicyID:   p.ID,
		ActorID:    actorID,
		ChangeType: ChangeCreated,
		NewRules:   &rules,
		Notes:      "Policy created",
		CreatedAt:  now,
	})

	// Emit event
	_ = s.bus.Publish(ctx, events.Event{
		Type: EventPolicyCreated,
		Payload: map[string]any{
			"policy_id":  p.ID,
			"project_id": projectID,
			"actor_id":   actorID,
		},
	})

	return p, nil
}

// GetCalibrationPolicy returns a policy by ID.
func (s *CalibrationService) GetCalibrationPolicy(ctx context.Context, id string) (*CalibrationPolicy, error) {
	return s.store.GetPolicy(ctx, id)
}

// ListCalibrationPolicies returns all policies for a project.
func (s *CalibrationService) ListCalibrationPolicies(ctx context.Context, projectID string, enabledOnly bool) ([]CalibrationPolicy, error) {
	return s.store.ListPolicies(ctx, projectID, enabledOnly)
}

// UpdateCalibrationPolicy updates a policy and records the change event.
func (s *CalibrationService) UpdateCalibrationPolicy(ctx context.Context, id, name, description, actorID string, rules CalibrationRules, enabled bool, minSample int) error {
	prev, err := s.store.GetPolicy(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.UpdatePolicy(ctx, id, name, description, rules, enabled, minSample); err != nil {
		return err
	}

	// Determine change type
	changeType := ChangeUpdated
	if prev.Enabled && !enabled {
		changeType = ChangeDisabled
	} else if !prev.Enabled && enabled {
		changeType = ChangeEnabled
	}

	_ = s.store.RecordChangeEvent(ctx, &PolicyChangeEvent{
		PolicyID:   id,
		ActorID:    actorID,
		ChangeType: changeType,
		PrevRules:  &prev.Rules,
		NewRules:   &rules,
		Notes:      "Policy updated",
		CreatedAt:  time.Now().UTC(),
	})

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventPolicyUpdated,
		Payload: map[string]any{
			"policy_id":   id,
			"project_id":  prev.ProjectID,
			"actor_id":    actorID,
			"change_type": string(changeType),
		},
	})

	return nil
}

// RecordDecision records a policy decision on a ticket.
func (s *CalibrationService) RecordDecision(ctx context.Context, policyID, ticketID string, decision Decision, reason string) (*CalibrationDecision, error) {
	d := &CalibrationDecision{
		PolicyID:  policyID,
		TicketID:  ticketID,
		Decision:  decision,
		Reason:    reason,
		DecidedAt: time.Now().UTC(),
	}
	if err := s.store.RecordDecision(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// RecordOutcome records the outcome for a specific decision.
func (s *CalibrationService) RecordOutcome(ctx context.Context, decisionID string, outcome Outcome) error {
	return s.store.RecordOutcome(ctx, decisionID, outcome)
}

// RecordOutcomeByTicket records the outcome for all decisions on a ticket.
func (s *CalibrationService) RecordOutcomeByTicket(ctx context.Context, ticketID string, outcome Outcome) error {
	return s.store.RecordOutcomeByTicket(ctx, ticketID, outcome)
}

// GetMetrics computes current metrics for a policy.
func (s *CalibrationService) GetMetrics(ctx context.Context, policyID string) (*Metrics, error) {
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	return s.store.ComputeMetrics(ctx, policyID, p.MinSample)
}

// GetPolicyHealth returns the full health view for a policy.
func (s *CalibrationService) GetPolicyHealth(ctx context.Context, policyID string) (*PolicyHealth, error) {
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	metrics, err := s.store.ComputeMetrics(ctx, policyID, p.MinSample)
	if err != nil {
		return nil, err
	}
	proposals, err := s.store.ListProposals(ctx, policyID, true)
	if err != nil {
		return nil, err
	}
	history, err := s.store.ListChangeEvents(ctx, policyID, 20)
	if err != nil {
		return nil, err
	}
	return &PolicyHealth{
		Policy:    *p,
		Metrics:   *metrics,
		Proposals: proposals,
		History:   history,
	}, nil
}

// GetProjectHealth returns health views for all active policies in a project.
func (s *CalibrationService) GetProjectHealth(ctx context.Context, projectID string) ([]PolicyHealth, error) {
	policies, err := s.store.ListPolicies(ctx, projectID, false)
	if err != nil {
		return nil, err
	}
	var health []PolicyHealth
	for _, p := range policies {
		metrics, err := s.store.ComputeMetrics(ctx, p.ID, p.MinSample)
		if err != nil {
			continue
		}
		proposals, err := s.store.ListProposals(ctx, p.ID, true)
		if err != nil {
			proposals = nil
		}
		history, err := s.store.ListChangeEvents(ctx, p.ID, 5)
		if err != nil {
			history = nil
		}
		health = append(health, PolicyHealth{
			Policy:    p,
			Metrics:   *metrics,
			Proposals: proposals,
			History:   history,
		})
	}
	return health, nil
}

// RunCalibration checks all active policies in a project and generates proposals.
// This should be called periodically (e.g., daily or on outcome recording).
func (s *CalibrationService) RunCalibration(ctx context.Context, projectID string) ([]PolicyProposal, error) {
	policies, err := s.store.ListPolicies(ctx, projectID, true)
	if err != nil {
		return nil, err
	}
	var generated []PolicyProposal
	for _, p := range policies {
		metrics, err := s.store.ComputeMetrics(ctx, p.ID, p.MinSample)
		if err != nil {
			continue
		}
		if !metrics.SampleSizeSufficient {
			continue
		}
		proposal := s.analyzeMetrics(&p, metrics)
		if proposal != nil {
			if err := s.store.CreateProposal(ctx, proposal); err == nil {
				generated = append(generated, *proposal)
				_ = s.bus.Publish(ctx, events.Event{
					Type: EventPolicyProposalCreated,
					Payload: map[string]any{
						"policy_id":     p.ID,
						"project_id":    projectID,
						"proposal_type": string(proposal.ProposalType),
						"proposal_id":   proposal.ID,
					},
				})
			}
		}
	}
	return generated, nil
}

// analyzeMetrics checks whether outcomes are trending well or badly and generates appropriate proposal.
func (s *CalibrationService) analyzeMetrics(p *CalibrationPolicy, m *Metrics) *PolicyProposal {
	// Good trend: low rollback, no incidents, high success over sufficient sample
	if m.RollbackRate <= GoodTrendMaxRollbackRate &&
		m.IncidentRate <= GoodTrendMaxIncidentRate &&
		m.SuccessRate >= GoodTrendMinSuccessRate {
		return &PolicyProposal{
			PolicyID:     p.ID,
			ProposalType: ProposalBroaden,
			Suggestion: map[string]any{
				"recommendation": "Consider broadening auto-approval scope",
				"rationale":      fmt.Sprintf("%.0f%% success rate with %.1f%% rollback rate over %d decisions", m.SuccessRate*100, m.RollbackRate*100, m.TotalDecisions),
				"current_auto_approve_conditions": p.Rules.AutoApproveConditions,
			},
			Statistics: map[string]any{
				"total_decisions":    m.TotalDecisions,
				"auto_approval_rate": m.AutoApprovalRate,
				"rollback_rate":      m.RollbackRate,
				"incident_rate":      m.IncidentRate,
				"success_rate":       m.SuccessRate,
			},
			Status:    ProposalPending,
			CreatedAt: time.Now().UTC(),
		}
	}

	// Bad trend: high rollback or incidents
	if m.RollbackRate >= BadTrendMinRollbackRate || m.IncidentRate >= BadTrendMinIncidentRate {
		return &PolicyProposal{
			PolicyID:     p.ID,
			ProposalType: ProposalReview,
			Suggestion: map[string]any{
				"recommendation": "Policy requires review - outcomes trending badly",
				"concerns":       buildConcerns(m),
			},
			Statistics: map[string]any{
				"total_decisions":    m.TotalDecisions,
				"auto_approval_rate": m.AutoApprovalRate,
				"rollback_rate":      m.RollbackRate,
				"incident_rate":      m.IncidentRate,
				"success_rate":       m.SuccessRate,
			},
			Status:    ProposalPending,
			CreatedAt: time.Now().UTC(),
		}
	}

	return nil
}

func buildConcerns(m *Metrics) []string {
	var concerns []string
	if m.RollbackRate >= BadTrendMinRollbackRate {
		concerns = append(concerns, fmt.Sprintf("Rollback rate %.1f%% exceeds threshold %.1f%%", m.RollbackRate*100, BadTrendMinRollbackRate*100))
	}
	if m.IncidentRate >= BadTrendMinIncidentRate {
		concerns = append(concerns, fmt.Sprintf("Incident attribution rate %.1f%% above zero tolerance", m.IncidentRate*100))
	}
	return concerns
}

// ResolveProposal resolves a pending proposal.
func (s *CalibrationService) ResolveProposal(ctx context.Context, proposalID string, status ProposalStatus, resolvedBy string) error {
	return s.store.ResolveProposal(ctx, proposalID, status, resolvedBy)
}

// SimulateRuleChange runs candidate rules against the last N days of historical tickets.
func (s *CalibrationService) SimulateRuleChange(ctx context.Context, policyID string, candidateRules CalibrationRules, days int) (*SimulationResult, error) {
	if days <= 0 {
		days = 30
	}
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}

	// Get historical decisions for this policy
	decisions, err := s.store.ListRecentDecisionsByProject(ctx, p.ProjectID, days)
	if err != nil {
		return nil, err
	}

	// Filter to this policy's decisions
	var policyDecisions []CalibrationDecision
	for _, d := range decisions {
		if d.PolicyID == policyID {
			policyDecisions = append(policyDecisions, d)
		}
	}

	result := &SimulationResult{
		PolicyID:         policyID,
		CandidateRules:   candidateRules,
		TicketsSimulated: len(policyDecisions),
	}

	var autoApprove, requireReview, block, changed int
	for _, d := range policyDecisions {
		// Simulate: evaluate candidate rules against this ticket's context
		newDecision := simulateDecision(candidateRules, d)
		sd := SimulationDecision{
			TicketID:        d.TicketID,
			CurrentDecision: d.Decision,
			NewDecision:     newDecision,
			Changed:         d.Decision != newDecision,
		}
		result.Results = append(result.Results, sd)
		switch newDecision {
		case DecisionAutoApproved:
			autoApprove++
		case DecisionRequiredReview:
			requireReview++
		case DecisionBlocked:
			block++
		}
		if sd.Changed {
			changed++
		}
	}

	total := len(policyDecisions)
	changeRate := 0.0
	if total > 0 {
		changeRate = float64(changed) / float64(total)
	}
	result.Summary = SimulationSummary{
		TotalTickets:       total,
		WouldAutoApprove:   autoApprove,
		WouldRequireReview: requireReview,
		WouldBlock:         block,
		ChangedDecisions:   changed,
		ChangeRate:         changeRate,
	}

	return result, nil
}

// simulateDecision evaluates candidate rules against a historical decision.
// In a full implementation this would re-evaluate conditions against ticket metadata;
// for now it uses rule count heuristics (more auto-approve conditions → more auto-approvals).
func simulateDecision(rules CalibrationRules, d CalibrationDecision) Decision {
	// If candidate has more restrictive block conditions and the original was auto_approved,
	// simulate as blocked. If candidate has more permissive auto-approve conditions and
	// original was required_review, simulate as auto_approved. Otherwise keep the same.
	if len(rules.BlockConditions) > 0 && d.Decision == DecisionAutoApproved {
		return DecisionBlocked
	}
	if len(rules.AutoApproveConditions) > 0 && d.Decision == DecisionRequiredReview {
		return DecisionAutoApproved
	}
	return d.Decision
}

// ListDecisions returns recent decisions for a policy.
func (s *CalibrationService) ListDecisions(ctx context.Context, policyID string, limit int) ([]CalibrationDecision, error) {
	return s.store.ListDecisions(ctx, policyID, limit)
}

// ListChangeEvents returns the change history for a policy.
func (s *CalibrationService) ListChangeEvents(ctx context.Context, policyID string, limit int) ([]PolicyChangeEvent, error) {
	return s.store.ListChangeEvents(ctx, policyID, limit)
}
