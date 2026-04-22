package rollback

import (
	"context"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// Stage classifies the lifecycle phase for rollback behavior.
type Stage string

const (
	StageExecution    Stage = "execution"     // executing
	StagePreDeploy    Stage = "pre_deploy"    // awaiting_validation, validated
	StagePostDeploy   Stage = "post_deploy"   // deploying, observing
	StagePostObserve  Stage = "post_observe"  // closed (creates new ticket, not a transition)
)

// ClassifyStage maps a ticket state to a rollback stage.
func ClassifyStage(state ticket.State) (Stage, error) {
	switch state {
	case ticket.StateExecuting:
		return StageExecution, nil
	case ticket.StateAwaitingValidation, ticket.StateValidated:
		return StagePreDeploy, nil
	case ticket.StateDeploying, ticket.StateObserving:
		return StagePostDeploy, nil
	case ticket.StateClosed:
		return StagePostObserve, nil
	default:
		return "", fmt.Errorf("rollback not available from state %q", state)
	}
}

// Result captures what happened during a rollback.
type Result struct {
	TicketID         string   `json:"ticket_id"`
	PreviousState    string   `json:"previous_state"`
	NewState         string   `json:"new_state"`
	Stage            Stage    `json:"stage"`
	Actions          []string `json:"actions"`
	WorktreeRemoved  bool     `json:"worktree_removed,omitempty"`
	ClaimsReleased   bool     `json:"claims_released"`
	IncidentTicketID string   `json:"incident_ticket_id,omitempty"`
	RollbackTicketID string   `json:"rollback_ticket_id,omitempty"`
}

// WorktreeRemover cleans up git worktrees for a ticket.
type WorktreeRemover interface {
	Remove(ticketID string) error
	Path(ticketID string) string
}

// LeaseRemover releases a ticket's lease in the queue.
type LeaseRemover interface {
	ForceReleaseLease(ctx context.Context, ticketID string) error
}

// TicketCreator creates new tickets (for incident/rollback tickets).
type TicketCreator interface {
	CreateTicket(ctx context.Context, projectID, title string, typ ticket.TicketType, priority ticket.Priority, createdBy string, dependsOn []string, workStreamID string, objective ticket.Objective, ticketContext ticket.TicketContext, idempotencyKey string, targetRepo ...string) (*ticket.Ticket, error)
}

// TicketTransitioner performs state transitions.
type TicketTransitioner interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
	TransitionTicket(ctx context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error
}

// Service coordinates rollback behavior across lifecycle stages.
type Service struct {
	tickets   TicketTransitioner
	creator   TicketCreator
	worktrees WorktreeRemover // nil-safe: if nil, worktree cleanup is skipped
	leases    LeaseRemover    // nil-safe: if nil, lease release is skipped
	bus       events.Bus
}

// NewService creates a rollback service.
func NewService(tickets TicketTransitioner, creator TicketCreator, bus events.Bus) *Service {
	return &Service{
		tickets: tickets,
		creator: creator,
		bus:     bus,
	}
}

// SetWorktreeRemover configures the worktree cleanup. Optional.
func (s *Service) SetWorktreeRemover(w WorktreeRemover) {
	s.worktrees = w
}

// SetLeaseRemover configures the lease release. Optional.
func (s *Service) SetLeaseRemover(l LeaseRemover) {
	s.leases = l
}

// Rollback initiates a stage-specific rollback for a ticket.
// For closed tickets (post-observation), it creates a new rollback ticket
// rather than transitioning the original.
func (s *Service) Rollback(ctx context.Context, ticketID string, actor ticket.Actor, reason string) (*Result, error) {
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("get ticket: %w", err)
	}

	stage, err := ClassifyStage(t.State)
	if err != nil {
		return nil, err
	}

	result := &Result{
		TicketID:      ticketID,
		PreviousState: string(t.State),
		Stage:         stage,
	}

	switch stage {
	case StageExecution:
		return s.rollbackExecution(ctx, t, actor, reason, result)
	case StagePreDeploy:
		return s.rollbackPreDeploy(ctx, t, actor, reason, result)
	case StagePostDeploy:
		return s.rollbackPostDeploy(ctx, t, actor, reason, result)
	case StagePostObserve:
		return s.rollbackPostObserve(ctx, t, actor, reason, result)
	default:
		return nil, fmt.Errorf("unknown rollback stage: %s", stage)
	}
}

// rollbackExecution handles rollback during execution:
// - Discard worktree
// - Release claims
// - Return ticket to draft (re-plannable)
func (s *Service) rollbackExecution(ctx context.Context, t *ticket.Ticket, actor ticket.Actor, reason string, result *Result) (*Result, error) {
	// Discard worktree.
	if s.worktrees != nil {
		if path := s.worktrees.Path(t.ID); path != "" {
			if err := s.worktrees.Remove(t.ID); err == nil {
				result.WorktreeRemoved = true
				result.Actions = append(result.Actions, "worktree discarded")
			}
		}
	}

	// Transition to draft (releases claims via service.go assignedTo clearing).
	payload := map[string]any{"reason": reason, "stage": string(StageExecution)}
	if err := s.tickets.TransitionTicket(ctx, t.ID, ticket.TriggerRollback, actor, payload); err != nil {
		return nil, fmt.Errorf("transition: %w", err)
	}
	result.NewState = string(ticket.StateDraft)
	result.ClaimsReleased = true
	result.Actions = append(result.Actions, "claims released", "returned to draft for re-planning")

	// Release lease in queue.
	s.releaseLease(ctx, t.ID, result)

	return result, nil
}

// rollbackPreDeploy handles rollback after execution but before deploy:
// - Revert diff (recorded as action; actual git revert is done by the re-planner)
// - Re-validate
// - Return ticket to draft
func (s *Service) rollbackPreDeploy(ctx context.Context, t *ticket.Ticket, actor ticket.Actor, reason string, result *Result) (*Result, error) {
	// Transition to draft.
	payload := map[string]any{"reason": reason, "stage": string(StagePreDeploy)}
	if err := s.tickets.TransitionTicket(ctx, t.ID, ticket.TriggerRollback, actor, payload); err != nil {
		return nil, fmt.Errorf("transition: %w", err)
	}
	result.NewState = string(ticket.StateDraft)
	result.ClaimsReleased = true
	result.Actions = append(result.Actions, "diff marked for revert", "claims released", "returned to draft for re-validation")

	// Clean up any leftover worktree.
	if s.worktrees != nil {
		if path := s.worktrees.Path(t.ID); path != "" {
			if err := s.worktrees.Remove(t.ID); err == nil {
				result.WorktreeRemoved = true
				result.Actions = append(result.Actions, "worktree cleaned up")
			}
		}
	}

	// Release lease in queue.
	s.releaseLease(ctx, t.ID, result)

	return result, nil
}

// rollbackPostDeploy handles rollback after deployment:
// - Redeploy previous version (recorded as action; actual redeploy is infrastructure concern)
// - For production: auto-create incident ticket, carry forward observation
// - Return ticket to draft
func (s *Service) rollbackPostDeploy(ctx context.Context, t *ticket.Ticket, actor ticket.Actor, reason string, result *Result) (*Result, error) {
	// Transition to draft.
	payload := map[string]any{"reason": reason, "stage": string(StagePostDeploy), "environment": string(t.Environment)}
	if err := s.tickets.TransitionTicket(ctx, t.ID, ticket.TriggerRollback, actor, payload); err != nil {
		return nil, fmt.Errorf("transition: %w", err)
	}
	result.NewState = string(ticket.StateDraft)
	result.ClaimsReleased = true
	result.Actions = append(result.Actions, "previous version redeploy requested", "claims released", "returned to draft")

	// For production deployments: create incident ticket.
	if t.Environment == ticket.EnvProduction {
		incidentID, err := s.createIncidentTicket(ctx, t, reason, actor)
		if err == nil && incidentID != "" {
			result.IncidentTicketID = incidentID
			result.Actions = append(result.Actions, fmt.Sprintf("incident ticket created: %s", incidentID))
		}
		result.Actions = append(result.Actions, "observation carried forward to incident")
	}

	// Release lease in queue.
	s.releaseLease(ctx, t.ID, result)

	return result, nil
}

// rollbackPostObserve handles rollback after observation (ticket is closed):
// - Creates a new rollback ticket for structural code revert
// - Original ticket stays closed
func (s *Service) rollbackPostObserve(ctx context.Context, t *ticket.Ticket, actor ticket.Actor, reason string, result *Result) (*Result, error) {
	// Don't transition the original ticket — it stays closed.
	result.NewState = string(ticket.StateClosed)
	result.ClaimsReleased = false

	// Create a new ticket for the structural revert.
	revertTitle := fmt.Sprintf("Rollback: revert changes from %s", t.ID)
	revertObjective := ticket.Objective{
		Description: fmt.Sprintf(
			"Structural code revert of changes from ticket %s (%s). Reason: %s. "+
				"The original ticket's changes need to be reverted. Review the original PR and outputs, "+
				"then create a revert commit that undoes the changes.",
			t.ID, t.Title, reason,
		),
		SuccessCriteria: []string{
			fmt.Sprintf("Changes from %s are reverted", t.ID),
			"Revert commit passes all tests",
			"No regressions introduced by the revert",
		},
	}
	revertContext := ticket.TicketContext{
		Constraints: []string{
			fmt.Sprintf("This is a rollback of ticket %s", t.ID),
			"Revert should be structural (code changes), not just a deploy rollback",
		},
	}

	revertTicket, err := s.creator.CreateTicket(
		ctx,
		t.ProjectID,
		revertTitle,
		ticket.TypeTask,
		ticket.P0, // Rollback tickets are high priority.
		actor.ID,
		nil, // No dependencies — rollback should start immediately.
		t.WorkStreamID,
		revertObjective,
		revertContext,
		"", // No idempotency key.
	)
	if err != nil {
		return nil, fmt.Errorf("create rollback ticket: %w", err)
	}

	result.RollbackTicketID = revertTicket.ID
	result.Actions = append(result.Actions,
		fmt.Sprintf("rollback ticket created: %s", revertTicket.ID),
		"original ticket remains closed",
		"structural code revert assigned as new work",
	)

	// Emit rollback event (no state transition on the original, so we emit manually).
	_ = s.bus.Publish(ctx, events.Event{
		Type: events.EventTicketRolledBack,
		Payload: map[string]any{
			"ticket_id":          t.ID,
			"project_id":        t.ProjectID,
			"stage":             string(StagePostObserve),
			"rollback_ticket_id": revertTicket.ID,
			"reason":            reason,
		},
	})

	return result, nil
}

// createIncidentTicket creates an incident ticket for production rollbacks.
func (s *Service) createIncidentTicket(ctx context.Context, t *ticket.Ticket, reason string, actor ticket.Actor) (string, error) {
	incidentTitle := fmt.Sprintf("Incident: prod rollback of %s", t.ID)
	incidentObjective := ticket.Objective{
		Description: fmt.Sprintf(
			"Production rollback of ticket %s (%s) triggered. Reason: %s. "+
				"Previous version redeployed. Investigate root cause, confirm rollback is stable, "+
				"and determine whether the original changes can be re-applied after fixes.",
			t.ID, t.Title, reason,
		),
		SuccessCriteria: []string{
			"Root cause identified",
			"Rollback confirmed stable in production",
			"Decision made on re-applying original changes",
		},
	}
	incidentContext := ticket.TicketContext{
		Constraints: []string{
			fmt.Sprintf("Triggered by rollback of %s", t.ID),
			"Production environment — handle with urgency",
			"Carry forward any observation data from the original deployment",
		},
	}

	incidentTicket, err := s.creator.CreateTicket(
		ctx,
		t.ProjectID,
		incidentTitle,
		ticket.TypeBug, // Incidents map to bug type.
		ticket.P0,      // Production incidents are P0.
		actor.ID,
		nil, // No dependencies.
		t.WorkStreamID,
		incidentObjective,
		incidentContext,
		fmt.Sprintf("rollback-incident-%s-%d", t.ID, time.Now().Unix()), // Idempotency key.
	)
	if err != nil {
		return "", err
	}

	// Emit incident event.
	_ = s.bus.Publish(ctx, events.Event{
		Type: events.EventRollbackIncident,
		Payload: map[string]any{
			"ticket_id":          t.ID,
			"incident_ticket_id": incidentTicket.ID,
			"project_id":        t.ProjectID,
			"environment":       string(t.Environment),
			"reason":            reason,
		},
	})

	return incidentTicket.ID, nil
}

// releaseLease attempts to release the lease in the queue system (best-effort).
func (s *Service) releaseLease(ctx context.Context, ticketID string, result *Result) {
	if s.leases == nil {
		return
	}
	if err := s.leases.ForceReleaseLease(ctx, ticketID); err == nil {
		result.Actions = append(result.Actions, "lease released in queue")
	}
}
