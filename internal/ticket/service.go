package ticket

import (
	"context"
	"fmt"
	"strconv"
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
	Action       string        // auto, notify, plan-only, open-pr-stop, approve, typed-confirm, human-required
	MatchedRules []MatchedRule // which rules contributed to this decision
	Reason       string        // human-readable explanation
}

// MatchedRule records a single rule that matched during policy evaluation.
type MatchedRule struct {
	RuleID   string
	RuleName string
	Action   string
	Reason   string
}

// PolicyEvaluator evaluates policy rules for a transition. Implemented by policy.Service.
type PolicyEvaluator interface {
	// EvaluateForTicket evaluates the active policy for a ticket's project against the given trigger.
	// Returns the policy decision. A nil return means no policy enforcement (auto).
	EvaluateForTicket(ctx context.Context, t *Ticket, trigger string) (*PolicyDecision, error)
}

// Service provides ticket operations.
type Service struct {
	store             TicketStore
	sm                *StateMachine
	bus               events.Bus
	project           ProjectGetter
	policyEvaluator   PolicyEvaluator
	acceptanceRunner  AcceptanceRunner
	autoApproveOnPass bool
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

// SetPolicyEvaluator sets the optional policy evaluator. When set, TransitionTicket
// evaluates policy rules before executing the transition and includes the policy
// decision in the transition event payload.
func (s *Service) SetPolicyEvaluator(pe PolicyEvaluator) {
	s.policyEvaluator = pe
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

// ErrAcceptanceCriteriaRequired is returned when a task or bug is created without success_criteria or acceptance_test.
var ErrAcceptanceCriteriaRequired = fmt.Errorf("tasks and bugs require at least one of: success_criteria or acceptance_test")

// CreateTicket creates a ticket with ID <project-slug>-<seq>. CreatedBy is the principal (user or agent ID).
// If idempotencyKey is non-empty and (projectID, idempotencyKey) was used before, returns the existing ticket.
// workStreamID is optional; caller must validate it exists and belongs to project.
func (s *Service) CreateTicket(ctx context.Context, projectID, title string, typ TicketType, priority Priority, createdBy string, dependsOn []string, workStreamID string, objective Objective, ticketContext TicketContext, idempotencyKey string, targetRepo ...string) (*Ticket, error) {
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
	if err := s.store.Create(ctx, t); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		_ = s.store.SetCreateIdempotency(ctx, projectID, idempotencyKey, id)
	}
	_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketCreated, map[string]any{"ticket_id": id}).WithEntityKey("ticket:"+id))
	return t, nil
}

// GetTicket returns a ticket by ID.
func (s *Service) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	return s.store.GetByID(ctx, id)
}

// ListTickets returns all tickets for a project. If workStreamID is non-empty, filters by work stream. If state is non-empty, filters by state.
func (s *Service) ListTickets(ctx context.Context, projectID string, workStreamID string, state State) ([]*Ticket, error) {
	return s.store.GetByProject(ctx, projectID, workStreamID, state)
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

// UpdateTargetRepo sets the target_repo alias for a ticket. Caller must validate the alias exists in project_repositories.
func (s *Service) UpdateTargetRepo(ctx context.Context, ticketID string, targetRepo string) error {
	return s.store.UpdateTargetRepo(ctx, ticketID, targetRepo)
}

// PatchTicketMetadata merges optional title and objective fields into a ticket. Only non-nil patch fields from objective are applied.
func (s *Service) PatchTicketMetadata(ctx context.Context, ticketID string, title *string, desc *string, successCriteria *[]string, acceptanceTest *string) error {
	t, err := s.store.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
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
	return s.store.UpdateTitleAndObjective(ctx, ticketID, newTitle, obj)
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

	deps, err := ResolveDependencies(s.store, ctx, t)
	if err != nil {
		return err
	}
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
	if trigger == TriggerLeaseExpired || trigger == TriggerReject {
		assignedTo = ""
	}
	if err := s.store.UpdateState(ctx, id, t.Version, newState, assignedTo); err != nil {
		return err
	}

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
	s.emitTransitionEvent(trigger, id, newState, t.ProjectID, eventExtra)
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
	payload := map[string]any{"outputs": outputs}
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
	s.emitTransitionEvent(TriggerSubmit, id, newState, t.ProjectID, nil)

	// Auto-approve if enabled and acceptance test was present and passed.
	if s.autoApproveOnPass && t.Objective.AcceptanceTest != "" && s.acceptanceRunner != nil {
		// Re-read ticket at new version for the approve transition.
		t2, err := s.store.GetByID(ctx, id)
		if err == nil && t2.State == StateAwaitingValidation {
			approveState, err := s.sm.Transition(t2, TriggerApprove, Actor{ID: "system", Type: ActorSystem}, nil, nil)
			if err == nil {
				if err := s.store.UpdateState(ctx, id, t2.Version, approveState, t2.AssignedTo); err == nil {
					s.emitTransitionEvent(TriggerApprove, id, approveState, t.ProjectID, nil)
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
func (s *Service) emitTransitionEvent(trigger, ticketID string, newState State, projectID string, extra map[string]any) {
	payload := map[string]any{"ticket_id": ticketID, "state": string(newState)}
	if projectID != "" {
		payload["project_id"] = projectID
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

// triggerToEventType maps a trigger+state to the correct event type to emit.
func triggerToEventType(trigger string, newState State) string {
	switch trigger {
	case TriggerSpec:
		return events.EventTicketSpecced
	case TriggerPlan:
		return events.EventTicketPlanning
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
	case TriggerDeploy:
		return events.EventTicketDeploying
	case TriggerObserve:
		return events.EventTicketObserving
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
