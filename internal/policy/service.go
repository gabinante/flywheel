package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

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

// Service provides policy management, evaluation, and preview.
type Service struct {
	store  Store
	engine *Engine
	bus    events.Bus
}

// NewService returns a new policy Service.
func NewService(store Store, bus events.Bus) *Service {
	return &Service{
		store:  store,
		engine: NewEngine(),
		bus:    bus,
	}
}

// --- CRUD Operations ---

// CreatePolicySet creates a new policy set for a project.
// If fromPosture is non-empty, the rules and credential scopes are populated from the named posture.
func (s *Service) CreatePolicySet(ctx context.Context, projectID, name, description, fromPosture, changedBy string) (*PolicySet, error) {
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
func (s *Service) GetActivePolicySet(ctx context.Context, projectID string) (*PolicySet, error) {
	return s.store.GetActivePolicySet(ctx, projectID)
}

// GetPolicySet returns a policy set by ID.
func (s *Service) GetPolicySet(ctx context.Context, id string) (*PolicySet, error) {
	return s.store.GetPolicySet(ctx, id)
}

// ListPolicySets returns all policy sets for a project.
func (s *Service) ListPolicySets(ctx context.Context, projectID string) ([]*PolicySet, error) {
	return s.store.ListPolicySets(ctx, projectID)
}

// ActivatePolicySet makes the given policy set the active one for its project.
// Deactivates any previously active set. Emits change events for both.
func (s *Service) ActivatePolicySet(ctx context.Context, policySetID, changedBy string) error {
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
func (s *Service) UpdateRules(ctx context.Context, policySetID string, rules []Rule, changedBy string) error {
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
func (s *Service) DeletePolicySet(ctx context.Context, policySetID, changedBy string) error {
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
func (s *Service) EvaluateTransition(ctx context.Context, projectID string, tctx TransitionContext) (*PolicyDecision, error) {
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
func (s *Service) GetEffectivePolicy(ctx context.Context, projectID string, ticketInfo TicketInfo) (*EffectivePolicy, error) {
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
func (s *Service) PreviewPolicyChange(ctx context.Context, projectID string, candidateRules []Rule, lister TicketLister, days int) ([]PreviewResult, error) {
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
func (s *Service) ApplyPosture(ctx context.Context, projectID, postureName, changedBy string) (*PolicySet, error) {
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

func (s *Service) emitPolicyChange(ctx context.Context, projectID, policySetID string, changeType ChangeType, changedBy string, oldRules, newRules []Rule) {
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
