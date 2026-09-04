package ticket

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
)

// ProjectGetter is used to resolve project slug for ticket IDs. Implemented by project.Service.
type ProjectGetter interface {
	GetProject(ctx context.Context, projectID string) (*project.Project, error)
}

// PolicyDecision represents the result of a policy evaluation.
type PolicyDecision struct {
	Action       string              // auto, notify, plan-only, open-pr-stop, approve, typed-confirm, human-required
	MatchedRules []MatchedRule       // which rules contributed to this decision
	Reason       string              // human-readable explanation
	Requirements []PolicyRequirement // automated gate conditions (unioned from matched rules)
}

// MatchedRule records a single rule that matched during policy evaluation.
type MatchedRule struct {
	RuleID   string
	RuleName string
	Action   string
	Reason   string
}

// PolicyRequirement is a gate condition that must be satisfied (mirrors policy.GateRequirement).
type PolicyRequirement struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// PolicyRequirementStatus reports whether a single requirement is satisfied.
type PolicyRequirementStatus struct {
	Requirement PolicyRequirement `json:"requirement"`
	Satisfied   bool              `json:"satisfied"`
	Reason      string            `json:"reason,omitempty"`
}

// PolicyGateBlockedError is returned when a transition is blocked by unsatisfied gate requirements.
type PolicyGateBlockedError struct {
	Action      string
	Unsatisfied []PolicyRequirementStatus
}

func (e *PolicyGateBlockedError) Error() string {
	msg := "policy gate blocked: "
	for i, u := range e.Unsatisfied {
		if i > 0 {
			msg += "; "
		}
		msg += u.Requirement.Type + ": " + u.Reason
	}
	return msg
}

// RequirementChecker checks automated gate requirements for a ticket.
type RequirementChecker interface {
	CheckRequirements(ctx context.Context, requirements []PolicyRequirement, ticketID, projectID, prURL string) []PolicyRequirementStatus
}

// PolicyEvaluator evaluates policy rules for a transition. Implemented by policy.Service.
type PolicyEvaluator interface {
	// EvaluateForTicket evaluates the active policy for a ticket's project against the given trigger.
	// Returns the policy decision. A nil return means no policy enforcement (auto).
	EvaluateForTicket(ctx context.Context, t *Ticket, trigger string) (*PolicyDecision, error)
}

// WorkflowResolver resolves the effective workflow for a project.
// Returns (workflowID, version, firstPhaseID, error). Empty workflowID means no workflow.
type WorkflowResolver interface {
	ResolveForProject(ctx context.Context, orgID, projectID string) (workflowID string, version int, firstPhaseID string, err error)
}

// ExternalRefLookup resolves external tracker projections for tickets. Implemented
// by the Linear store; optional.
type ExternalRefLookup interface {
	// RefsByTicketIDs returns projections keyed by ticket ID.
	RefsByTicketIDs(ctx context.Context, ticketIDs []string) (map[string]*ExternalRef, error)
	// TicketIDByIdentifier resolves an external identifier (e.g. RLETD-465) to a ticket ID, or "".
	TicketIDByIdentifier(ctx context.Context, identifier string) (string, error)
}

// Service provides ticket operations.
type Service struct {
	store              TicketStore
	transitionStore    *TransitionStore
	sm                 *StateMachine
	bus                events.Bus
	project            ProjectGetter
	policyEvaluator    PolicyEvaluator
	requirementChecker RequirementChecker
	acceptanceRunner   AcceptanceRunner
	autoApproveOnPass  bool
	workflowResolver   WorkflowResolver
	externalRefs       ExternalRefLookup
}

// SetExternalRefLookup enables external-tracker projections on read paths and
// lets GetTicket accept external identifiers such as RLETD-465.
func (s *Service) SetExternalRefLookup(l ExternalRefLookup) {
	s.externalRefs = l
}

// NewService returns a new Service. The store parameter accepts any TicketStore
// implementation (Postgres *Store, embedded SQLite, etc.).
func NewService(store TicketStore, bus events.Bus, project ProjectGetter) *Service {
	return &Service{
		store:   store,
		sm:      NewStateMachine(),
		bus:     bus,
		project: project,
	}
}

// SetTransitionStore sets the optional store for recording state transitions.
func (s *Service) SetTransitionStore(ts *TransitionStore) {
	s.transitionStore = ts
}

// GetTransitions returns the state transition history for a ticket.
func (s *Service) GetTransitions(ctx context.Context, ticketID string) ([]StateTransition, error) {
	if s.transitionStore == nil {
		return nil, nil
	}
	return s.transitionStore.ListByTicket(ctx, ticketID)
}

// SetPolicyEvaluator sets the optional policy evaluator. When set, TransitionTicket
// evaluates policy rules before executing the transition and includes the policy
// decision in the transition event payload.
func (s *Service) SetPolicyEvaluator(pe PolicyEvaluator) {
	s.policyEvaluator = pe
}

// SetRequirementChecker sets the optional checker for automated gate requirements.
// When set, TransitionTicket checks requirements before executing and returns
// PolicyGateBlockedError if any requirement is unsatisfied.
func (s *Service) SetRequirementChecker(rc RequirementChecker) {
	s.requirementChecker = rc
}

// SetAcceptanceRunner sets the optional runner for acceptance_test on submit. When set and the ticket has objective.acceptance_test, SubmitTicket runs it and rejects on failure.
func (s *Service) SetAcceptanceRunner(r AcceptanceRunner) {
	s.acceptanceRunner = r
}

// SetAutoApproveOnPass enables automatic approval when acceptance tests pass on submit.
// Only applies to tickets that have an acceptance_test defined.
func (s *Service) SetAutoApproveOnPass(enabled bool) {
	s.autoApproveOnPass = enabled
}

// SetWorkflowResolver sets the optional workflow resolver. When set, CreateTicket
// auto-attaches the resolved workflow to new tickets.
func (s *Service) SetWorkflowResolver(wr WorkflowResolver) {
	s.workflowResolver = wr
}

// ErrAcceptanceCriteriaRequired is returned when a task or bug is created without success_criteria or acceptance_test.
var ErrAcceptanceCriteriaRequired = fmt.Errorf("tasks and bugs require at least one of: success_criteria or acceptance_test")

// CreateTicket creates a ticket with ID <project-slug>-<seq>. CreatedBy is the principal (user or agent ID).
// If idempotencyKey is non-empty and (projectID, idempotencyKey) was used before, returns the existing ticket.
// workStreamID is optional; caller must validate it exists and belongs to project.
func (s *Service) CreateTicket(ctx context.Context, projectID, title string, typ TicketType, priority Priority, createdBy string, dependsOn []string, workStreamID string, objective Objective, ticketContext TicketContext, idempotencyKey string, targetRepo ...string) (*Ticket, error) {
	// Normalize nil to empty slice so depends_on is always persisted as a
	// valid array (not NULL) and serializes as [] rather than null.
	if dependsOn == nil {
		dependsOn = []string{}
	}
	if typ == TypeTask || typ == TypeBug {
		hasCriteria := len(objective.SuccessCriteria) > 0 || objective.AcceptanceTest != ""
		if !hasCriteria {
			return nil, ErrAcceptanceCriteriaRequired
		}
	}
	if idempotencyKey != "" {
		existingID, err := s.store.GetTicketIDByCreateIdempotency(ctx, projectID, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existingID != "" {
			return s.store.GetByID(ctx, existingID)
		}
	}
	p, err := s.project.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	slug := p.Slug
	seq, err := s.store.NextSequence(ctx, projectID)
	if err != nil {
		return nil, err
	}
	id := slug + "-" + strconv.FormatInt(seq, 10)
	now := time.Now().UTC()
	repo := ""
	if len(targetRepo) > 0 {
		repo = targetRepo[0]
	}
	t := &Ticket{
		ID:           id,
		ProjectID:    projectID,
		Title:        title,
		Type:         typ,
		Priority:     priority,
		State:        StateDraft,
		Version:      0,
		Objective:    objective,
		Context:      ticketContext,
		Inputs:       make(map[string]any),
		Outputs:      make(map[string]any),
		DependsOn:    dependsOn,
		WorkStreamID: workStreamID,
		TargetRepo:   repo,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	// Auto-attach workflow if resolver is configured, pinning the version.
	if s.workflowResolver != nil {
		wfID, wfVersion, phaseID, wfErr := s.workflowResolver.ResolveForProject(ctx, p.OrgID, projectID)
		if wfErr == nil && wfID != "" {
			t.WorkflowID = wfID
			t.WorkflowVersion = wfVersion
			t.WorkflowPhase = phaseID
		}
	}
	if err := s.store.Create(ctx, t); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		_ = s.store.SetCreateIdempotency(ctx, projectID, idempotencyKey, id)
	}
	_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketCreated, map[string]any{
		"ticket_id":  id,
		"project_id": projectID,
		"title":      title,
		"state":      string(StateDraft),
		"created_by": createdBy,
	}).WithEntityKey("ticket:"+id))
	return t, nil
}

// GetTicket returns a ticket by ID.
func (s *Service) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	if s.externalRefs != nil && externalIdentifierRe.MatchString(id) {
		if resolved, err := s.externalRefs.TicketIDByIdentifier(ctx, id); err == nil && resolved != "" {
			id = resolved
		}
	}
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.attachExternal(ctx, []*Ticket{t})
	return t, nil
}

// ListTickets returns all tickets for a project. If workStreamID is non-empty, filters by work stream. If state is non-empty, filters by state.
func (s *Service) ListTickets(ctx context.Context, projectID string, workStreamID string, state State) ([]*Ticket, error) {
	list, err := s.store.GetByProject(ctx, projectID, workStreamID, state)
	if err != nil {
		return nil, err
	}
	s.attachExternal(ctx, list)
	return list, nil
}

// externalIdentifierRe matches Linear-style identifiers (KEY-123).
var externalIdentifierRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-\d{1,6}$`)

// attachExternal fills Ticket.External from the lookup; failures are ignored.
func (s *Service) attachExternal(ctx context.Context, tickets []*Ticket) {
	if s.externalRefs == nil || len(tickets) == 0 {
		return
	}
	ids := make([]string, 0, len(tickets))
	for _, t := range tickets {
		if t != nil {
			ids = append(ids, t.ID)
		}
	}
	refs, err := s.externalRefs.RefsByTicketIDs(ctx, ids)
	if err != nil {
		return
	}
	for _, t := range tickets {
		if t != nil {
			t.External = refs[t.ID]
		}
	}
}

// ExternalImport describes an issue imported from an external tracker.
type ExternalImport struct {
	ProjectID  string
	Title      string
	Type       TicketType
	Priority   Priority
	State      State
	Objective  Objective
	Provider   string
	Identifier string
	CreatedAt  time.Time
	TargetRepo string
}

// ImportExternal creates a ticket mirroring an external issue. Unlike CreateTicket
// it does not require acceptance criteria and starts in the mapped state. The
// ticket.created event carries external_provider so sync subscribers can ignore it.
func (s *Service) ImportExternal(ctx context.Context, in ExternalImport) (*Ticket, error) {
	p, err := s.project.GetProject(ctx, in.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	seq, err := s.store.NextSequence(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	created := in.CreatedAt
	if created.IsZero() {
		created = now
	}
	state := in.State
	if !isKnownState(state) {
		state = StateDraft
	}
	typ := in.Type
	if typ == "" {
		typ = TypeTask
	}
	t := &Ticket{
		ID:         p.Slug + "-" + strconv.FormatInt(seq, 10),
		ProjectID:  in.ProjectID,
		Title:      in.Title,
		Type:       typ,
		Priority:   in.Priority,
		State:      state,
		Version:    1,
		Objective:  in.Objective,
		Inputs:     map[string]any{},
		Outputs:    map[string]any{},
		DependsOn:  []string{},
		TargetRepo: in.TargetRepo,
		CreatedBy:  in.Provider,
		CreatedAt:  created,
		UpdatedAt:  now,
	}
	if err := s.store.Create(ctx, t); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketCreated, map[string]any{
		"ticket_id":           t.ID,
		"project_id":          t.ProjectID,
		"state":               string(t.State),
		"external_provider":   in.Provider,
		"external_identifier": in.Identifier,
	}))
	return t, nil
}

// SetStateFromExternal applies a state observed in the external tracker without
// running the Flywheel state machine. It records the transition and publishes
// ticket.updated so the UI and command center see the change.
func (s *Service) SetStateFromExternal(ctx context.Context, id string, newState State, provider string) error {
	if !isKnownState(newState) {
		return fmt.Errorf("invalid state %q", newState)
	}
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if t.State == newState {
		return nil
	}
	if err := s.store.UpdateState(ctx, t.ID, t.Version, newState, t.AssignedTo); err != nil {
		return err
	}
	if s.transitionStore != nil {
		_ = s.transitionStore.Record(ctx, t.ID, t.State, newState, "external_sync", Actor{Type: "system", ID: provider})
	}
	_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketUpdated, map[string]any{
		"ticket_id":         t.ID,
		"project_id":        t.ProjectID,
		"from_state":        string(t.State),
		"state":             string(newState),
		"external_provider": provider,
	}))
	return nil
}

// UpdateTitleAndObjective replaces a ticket's title and objective (used by external sync).
func (s *Service) UpdateTitleAndObjective(ctx context.Context, id, title string, obj Objective) error {
	return s.store.UpdateTitleAndObjective(ctx, id, title, obj)
}

// CountTicketsCreatedBy returns how many tickets the given agent created (lifetime).
func (s *Service) CountTicketsCreatedBy(ctx context.Context, agentID string) (int, error) {
	return s.store.CountByCreatedBy(ctx, agentID)
}

// CountTicketsCreatedByPerDay returns daily counts for the given agent for the last days (oldest first).
func (s *Service) CountTicketsCreatedByPerDay(ctx context.Context, agentID string, days int) ([]int, error) {
	return s.store.CountByCreatedByPerDay(ctx, agentID, days)
}

// ListByState returns tickets in a given state for a project (for queue).
func (s *Service) ListByState(ctx context.Context, projectID string, state State) ([]*Ticket, error) {
	return s.store.ListByState(ctx, projectID, state)
}

// GetTicketsByIDs returns tickets by IDs (for DAG).
func (s *Service) GetTicketsByIDs(ctx context.Context, ids []string) ([]*Ticket, error) {
	return s.store.GetByIDs(ctx, ids)
}

// ListByWorkStream returns all tickets in a work stream (any state).
func (s *Service) ListByWorkStream(ctx context.Context, projectID string, workStreamID string) ([]*Ticket, error) {
	return s.store.GetByProject(ctx, projectID, workStreamID, "")
}

// ListStaleTickets returns tickets in the given states whose updated_at is older
// than the staleness threshold. Used by the DB staleness sweep (Layer 3 recovery).
func (s *Service) ListStaleTickets(ctx context.Context, states []State, threshold time.Duration) ([]*Ticket, error) {
	return s.store.ListStaleTickets(ctx, states, threshold)
}

// UpdateDependsOn sets the dependency list for a ticket. Caller must ensure dep IDs are valid and in the same project; no cycle check.
func (s *Service) UpdateDependsOn(ctx context.Context, ticketID string, dependsOn []string) error {
	if dependsOn == nil {
		dependsOn = []string{}
	}
	return s.store.UpdateDependsOn(ctx, ticketID, dependsOn)
}

// UpdateWorkStreamID sets the work_stream_id for a ticket. Caller must validate work stream exists and belongs to ticket's project.
func (s *Service) UpdateWorkStreamID(ctx context.Context, ticketID string, workStreamID string) error {
	return s.store.UpdateWorkStreamID(ctx, ticketID, workStreamID)
}

// UpdateEnvironmentID sets the environment_id for a ticket. Caller must validate environment exists and belongs to ticket's project.
func (s *Service) UpdateEnvironmentID(ctx context.Context, ticketID string, environmentID string) error {
	return s.store.UpdateEnvironmentID(ctx, ticketID, environmentID)
}

// UpdateTargetRepo sets the target_repo alias for a ticket. Caller must validate the alias exists in project_repositories.
func (s *Service) UpdateTargetRepo(ctx context.Context, ticketID string, targetRepo string) error {
	return s.store.UpdateTargetRepo(ctx, ticketID, targetRepo)
}

// UpdateWorkflow sets workflow_id, workflow_version, and workflow_phase for a ticket.
func (s *Service) UpdateWorkflow(ctx context.Context, ticketID string, workflowID string, workflowVersion int, workflowPhase string) error {
	return s.store.UpdateWorkflow(ctx, ticketID, workflowID, workflowVersion, workflowPhase)
}

// PatchInputs merges the given keys into existing inputs without overwriting unrelated keys.
func (s *Service) PatchInputs(ctx context.Context, ticketID string, patch map[string]any) error {
	return s.store.PatchInputs(ctx, ticketID, patch)
}

// PatchOutputs merges the given keys into existing outputs without overwriting unrelated keys.
func (s *Service) PatchOutputs(ctx context.Context, ticketID string, patch map[string]any) error {
	return s.store.PatchOutputs(ctx, ticketID, patch)
}

// UpdateWorkflowPhaseStatus sets the workflow phase status for a ticket.
func (s *Service) UpdateWorkflowPhaseStatus(ctx context.Context, ticketID string, status string) error {
	return s.store.UpdateWorkflowPhaseStatus(ctx, ticketID, status)
}

// PatchTicketMetadata merges optional title and objective fields into a ticket. Only non-nil patch fields from objective are applied.
func (s *Service) PatchTicketMetadata(ctx context.Context, ticketID string, title *string, desc *string, successCriteria *[]string, acceptanceTest *string) error {
	t, err := s.store.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	beforeObjective := t.Objective
	obj := t.Objective
	if desc != nil {
		obj.Description = *desc
	}
	if successCriteria != nil {
		obj.SuccessCriteria = *successCriteria
	}
	if acceptanceTest != nil {
		obj.AcceptanceTest = *acceptanceTest
	}
	if t.Type == TypeTask || t.Type == TypeBug {
		hasCriteria := len(obj.SuccessCriteria) > 0 || obj.AcceptanceTest != ""
		if !hasCriteria {
			return ErrAcceptanceCriteriaRequired
		}
	}
	newTitle := t.Title
	if title != nil && *title != "" {
		newTitle = *title
	}

	changedFields := make([]string, 0, 4)
	if newTitle != t.Title {
		changedFields = append(changedFields, "title")
	}
	if beforeObjective.Description != obj.Description {
		changedFields = append(changedFields, "description")
	}
	if !reflect.DeepEqual(beforeObjective.SuccessCriteria, obj.SuccessCriteria) {
		changedFields = append(changedFields, "success_criteria")
	}
	if beforeObjective.AcceptanceTest != obj.AcceptanceTest {
		changedFields = append(changedFields, "acceptance_test")
	}
	if len(changedFields) == 0 {
		return nil
	}
	if err := s.store.UpdateTitleAndObjective(ctx, ticketID, newTitle, obj); err != nil {
		return err
	}
	_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketUpdated, map[string]any{
		"ticket_id":      ticketID,
		"project_id":     t.ProjectID,
		"title":          newTitle,
		"state":          string(t.State),
		"changed_fields": changedFields,
	}).WithEntityKey("ticket:"+ticketID))
	return nil
}

// TransitionTicket applies a state transition (single entry point for all state changes).
// If a PolicyEvaluator is set, it evaluates policy rules before executing the transition.
// The policy decision is included in the transition event payload for audit purposes.
func (s *Service) TransitionTicket(ctx context.Context, id string, trigger string, actor Actor, payload map[string]any) error {
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Evaluate policy if evaluator is set.
	var policyDecision *PolicyDecision
	if s.policyEvaluator != nil {
		pd, err := s.policyEvaluator.EvaluateForTicket(ctx, t, trigger)
		if err != nil {
			return fmt.Errorf("policy evaluation: %w", err)
		}
		policyDecision = pd
	}

	// Check automated gate requirements if checker is configured and decision has requirements.
	if s.requirementChecker != nil && policyDecision != nil && len(policyDecision.Requirements) > 0 {
		prURL, _ := t.Outputs["pr_url"].(string)
		statuses := s.requirementChecker.CheckRequirements(ctx, policyDecision.Requirements, t.ID, t.ProjectID, prURL)
		var unsatisfied []PolicyRequirementStatus
		for _, st := range statuses {
			if !st.Satisfied {
				unsatisfied = append(unsatisfied, st)
			}
		}
		if len(unsatisfied) > 0 {
			return &PolicyGateBlockedError{
				Action:      policyDecision.Action,
				Unsatisfied: unsatisfied,
			}
		}
	}

	deps, err := ResolveDependencies(s.store, ctx, t)
	if err != nil {
		return err
	}
	fromState := t.State
	newState, err := s.sm.Transition(t, trigger, actor, payload, deps)
	if err != nil {
		return err
	}
	assignedTo := t.AssignedTo
	if trigger == TriggerClaim && payload["agent_id"] != nil {
		if aid, ok := payload["agent_id"].(string); ok {
			assignedTo = aid
		}
	}
	if trigger == TriggerLeaseExpired || trigger == TriggerReject || trigger == TriggerRollback || trigger == TriggerCancel {
		assignedTo = ""
	}
	if err := s.store.UpdateState(ctx, id, t.Version, newState, assignedTo); err != nil {
		return err
	}
	s.recordTransition(ctx, id, fromState, newState, trigger, actor)

	// Include policy decision in the event payload for audit trail.
	eventExtra := payload
	if policyDecision != nil {
		if eventExtra == nil {
			eventExtra = make(map[string]any)
		}
		eventExtra["policy_action"] = policyDecision.Action
		eventExtra["policy_reason"] = policyDecision.Reason
		if len(policyDecision.MatchedRules) > 0 {
			ruleNames := make([]string, len(policyDecision.MatchedRules))
			for i, mr := range policyDecision.MatchedRules {
				ruleNames[i] = mr.RuleName
			}
			eventExtra["policy_rules"] = ruleNames
		}
	}
	s.emitTransitionEvent(trigger, id, t.Title, newState, t.ProjectID, actor, eventExtra)
	return nil
}

// GetPolicyDecision returns the policy decision for a ticket's next transition
// without executing the transition. Used by API to show which gates a ticket will hit.
func (s *Service) GetPolicyDecision(ctx context.Context, id string, trigger string) (*PolicyDecision, error) {
	if s.policyEvaluator == nil {
		return nil, nil
	}
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.policyEvaluator.EvaluateForTicket(ctx, t, trigger)
}

// SubmitTicket validates outputs and transitions to awaiting_validation. Lease token is validated by caller (queue) if needed.
// If an AcceptanceRunner is set and the ticket has objective.acceptance_test, the test is run first; on failure the submit is rejected with AcceptanceTestFailure.
func (s *Service) SubmitTicket(ctx context.Context, id string, leaseToken string, outputs map[string]any) error {
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s.acceptanceRunner != nil && t.Objective.AcceptanceTest != "" {
		passed, stdout, _, runErr := s.acceptanceRunner.Run(ctx, t.Objective.AcceptanceTest)
		if runErr != nil || !passed {
			if fail, ok := runErr.(*AcceptanceTestFailure); ok {
				return fail
			}
			if runErr != nil {
				return runErr
			}
			return &AcceptanceTestFailure{Stdout: stdout, Stderr: ""}
		}
	}
	// Normalize pr_url: extract from artifacts if missing.
	if _, has := outputs["pr_url"]; !has {
		if artifacts, ok := outputs["artifacts"].([]any); ok {
			for _, a := range artifacts {
				if m, ok := a.(map[string]any); ok {
					if t, _ := m["type"].(string); t == "pr" || t == "pull_request" {
						if u, _ := m["url"].(string); u != "" {
							outputs["pr_url"] = u
							break
						}
					}
				}
			}
		}
	}
	// Strip non-URL pr_url values.
	if prURL, ok := outputs["pr_url"].(string); ok {
		if prURL != "" && !strings.HasPrefix(prURL, "http://") && !strings.HasPrefix(prURL, "https://") {
			delete(outputs, "pr_url")
		}
	}

	payload := map[string]any{"outputs": outputs}
	fromState := t.State
	newState, err := s.sm.Transition(t, TriggerSubmit, Actor{ID: t.AssignedTo, Type: ActorAgent}, payload, nil)
	if err != nil {
		return err
	}
	if err := s.store.UpdateOutputs(ctx, id, t.Version, outputs); err != nil {
		return err
	}
	if err := s.store.UpdateState(ctx, id, t.Version+1, newState, t.AssignedTo); err != nil {
		return err
	}
	_ = leaseToken
	s.recordTransition(ctx, id, fromState, newState, TriggerSubmit, Actor{ID: t.AssignedTo, Type: ActorAgent})
	s.emitTransitionEvent(
		TriggerSubmit,
		id,
		t.Title,
		newState,
		t.ProjectID,
		Actor{ID: t.AssignedTo, Type: ActorAgent},
		nil,
	)

	// Auto-approve if enabled and acceptance test was present and passed.
	if s.autoApproveOnPass && t.Objective.AcceptanceTest != "" && s.acceptanceRunner != nil {
		// Re-read ticket at new version for the approve transition.
		t2, err := s.store.GetByID(ctx, id)
		if err == nil && t2.State == StateAwaitingValidation {
			approveState, err := s.sm.Transition(t2, TriggerApprove, Actor{ID: "system", Type: ActorSystem}, nil, nil)
			if err == nil {
				if err := s.store.UpdateState(ctx, id, t2.Version, approveState, t2.AssignedTo); err == nil {
					s.recordTransition(ctx, id, t2.State, approveState, TriggerApprove, Actor{ID: "system", Type: ActorSystem})
					s.emitTransitionEvent(
						TriggerApprove,
						id,
						t2.Title,
						approveState,
						t.ProjectID,
						Actor{ID: "system", Type: ActorSystem},
						nil,
					)
				}
			}
		}
	}

	return nil
}

// EscalateTicket transitions to awaiting_input with reason and question.
func (s *Service) EscalateTicket(ctx context.Context, id string, leaseToken string, reason, question string) error {
	payload := map[string]any{"reason": reason, "question": question}
	t, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	_ = leaseToken
	return s.TransitionTicket(ctx, id, TriggerEscalate, Actor{ID: t.AssignedTo, Type: ActorAgent}, payload)
}

// AppendRejectionNotes appends a prior attempt summary after a human reject (for retry context).
func (s *Service) AppendRejectionNotes(ctx context.Context, ticketID, reviewerID, notes string) error {
	t, err := s.store.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	t.Context.PriorAttempts = append(t.Context.PriorAttempts, AttemptSummary{
		AgentID:   reviewerID,
		Outcome:   "rejected",
		Summary:   notes,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return s.store.UpdateContext(ctx, ticketID, t.Context)
}

// AppendFailureSummary appends a failure context entry after a worker exits without completing.
// This provides the next agent with context about why the previous attempt failed.
func (s *Service) AppendFailureSummary(ctx context.Context, ticketID, reason string) error {
	t, err := s.store.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	t.Context.PriorAttempts = append(t.Context.PriorAttempts, AttemptSummary{
		AgentID:   "system",
		Outcome:   "worker_exit",
		Summary:   reason,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return s.store.UpdateContext(ctx, ticketID, t.Context)
}

// InjectEscalationAnswer appends the human's answer to context (after resolving an escalation).
func (s *Service) InjectEscalationAnswer(ctx context.Context, ticketID, answer string) error {
	t, err := s.store.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	t.Context.HumanAnswers = append(t.Context.HumanAnswers, answer)
	return s.store.UpdateContext(ctx, ticketID, t.Context)
}

// emitTransitionEvent publishes a typed event for the given trigger/transition.
// Events are keyed by ticket ID for ordered delivery within an entity.
func (s *Service) emitTransitionEvent(trigger, ticketID, title string, newState State, projectID string, actor Actor, extra map[string]any) {
	payload := map[string]any{"ticket_id": ticketID, "state": string(newState)}
	if projectID != "" {
		payload["project_id"] = projectID
	}
	if title != "" {
		payload["title"] = title
	}
	if actor.ID != "" {
		payload["actor_id"] = actor.ID
		payload["actor_type"] = string(actor.Type)
	}
	for k, v := range extra {
		payload[k] = v
	}
	eventType := triggerToEventType(trigger, newState)
	if eventType != "" {
		event := events.NewEvent(eventType, payload).WithEntityKey("ticket:" + ticketID)
		_ = s.bus.Publish(context.Background(), event)
	}
}

// recordTransition persists a state transition record if the transition store is configured.
func (s *Service) recordTransition(ctx context.Context, ticketID string, from, to State, trigger string, actor Actor) {
	if s.transitionStore == nil {
		return
	}
	_ = s.transitionStore.Record(ctx, ticketID, from, to, trigger, actor)
}

// triggerToEventType maps a trigger+state to the correct event type to emit.
func triggerToEventType(trigger string, newState State) string {
	switch trigger {
	case TriggerClaim:
		return events.EventTicketClaimed
	case TriggerStart:
		return events.EventTicketStarted
	case TriggerSubmit:
		return events.EventTicketSubmitted
	case TriggerValidate:
		return events.EventTicketValidated
	case TriggerApprove:
		if newState == StateValidated {
			return events.EventTicketApproved
		}
		// Resolving awaiting_input → executing
		return events.EventTicketInputProvided
	case TriggerClose:
		return events.EventTicketClosed
	case TriggerRequestInput:
		return events.EventTicketAwaitingInput
	case TriggerEscalate:
		return events.EventTicketEscalated
	case TriggerProvideInput:
		return events.EventTicketInputProvided
	case TriggerReplan:
		return events.EventTicketReplanned
	case TriggerInvalidate:
		return events.EventTicketInvalidated
	case TriggerReject:
		return events.EventTicketRejected
	case TriggerRollback:
		return events.EventTicketRolledBack
	case TriggerCancel:
		return events.EventTicketCancelled
	case TriggerFail:
		return events.EventTicketFailed
	case TriggerLeaseExpired:
		return events.EventLeaseExpired
	case TriggerReopen:
		return events.EventTicketReopened
	default:
		return ""
	}
}

// isKnownState reports whether s is one of the canonical ticket states.
func isKnownState(s State) bool {
	for _, k := range AllStates() {
		if k == s {
			return true
		}
	}
	return false
}
