package workflow

import (
	"context"
	"fmt"
	"time"
)

// TicketUpdater is the minimal interface the engine needs to update ticket workflow state.
type TicketUpdater interface {
	UpdateWorkflowPhase(ctx context.Context, id string, workflowPhase string) error
	UpdateWorkflowPhaseStatus(ctx context.Context, id string, status string) error
	GetWorkflowPhase(ctx context.Context, id string) (string, error)
}

// DefinitionStore is the persistence interface used by the Engine.
type DefinitionStore interface {
	GetByID(ctx context.Context, id string) (*Definition, error)
	GetByIDAndVersion(ctx context.Context, id string, version int) (*Definition, error)
	GetByScope(ctx context.Context, scope, scopeID string) (*Definition, error)
	ListCompletions(ctx context.Context, ticketID string) ([]PhaseCompletion, error)
	RecordCompletion(ctx context.Context, c *PhaseCompletion) error
}

// Engine orchestrates ticket progression through workflow phases.
// It sits above the state machine and calls TransitionTicket — never modifies
// the state machine directly.
type Engine struct {
	store         DefinitionStore
	ticketUpdater TicketUpdater
}

// NewEngine returns a new workflow engine.
func NewEngine(store DefinitionStore, ticketUpdater TicketUpdater) *Engine {
	return &Engine{store: store, ticketUpdater: ticketUpdater}
}

// Resolve returns the effective workflow for a project by checking
// project → org → system scope (most specific wins).
// Returns nil if no active workflow is found at any level.
func (e *Engine) Resolve(ctx context.Context, orgID, projectID string) (*Definition, error) {
	// Project-level override
	if projectID != "" {
		d, err := e.store.GetByScope(ctx, "project", projectID)
		if err != nil {
			return nil, err
		}
		if d != nil {
			return d, nil
		}
	}
	// Org-level default
	if orgID != "" {
		d, err := e.store.GetByScope(ctx, "org", orgID)
		if err != nil {
			return nil, err
		}
		if d != nil {
			return d, nil
		}
	}
	// System-level default
	d, err := e.store.GetByScope(ctx, "system", "")
	if err != nil {
		return nil, err
	}
	return d, nil
}

// GetDefinition returns a workflow definition by ID.
func (e *Engine) GetDefinition(ctx context.Context, id string) (*Definition, error) {
	return e.store.GetByID(ctx, id)
}

// GetPosition returns the current workflow position for a ticket.
// If version > 0, uses the pinned version's phases.
func (e *Engine) GetPosition(ctx context.Context, ticketID, workflowID, workflowPhase string, version int) (*Position, error) {
	if workflowID == "" {
		return nil, nil
	}
	def, err := e.getDefinition(ctx, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	completions, err := e.store.ListCompletions(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list completions: %w", err)
	}

	pos := &Position{
		WorkflowID:   def.ID,
		WorkflowName: def.Name,
		TotalPhases:  len(def.Phases),
		Phases:       def.Phases,
		History:      completions,
	}

	for i, p := range def.Phases {
		if p.ID == workflowPhase {
			phase := p
			pos.CurrentPhase = &phase
			pos.PhaseIndex = i
			break
		}
	}
	return pos, nil
}

// StartPhase sets the ticket's workflow_phase to the first phase of its workflow.
// Called when a ticket is first assigned a workflow.
// If version > 0, uses the pinned version's phases.
func (e *Engine) StartPhase(ctx context.Context, ticketID, workflowID string, version int) error {
	def, err := e.getDefinition(ctx, workflowID, version)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}
	if len(def.Phases) == 0 {
		return fmt.Errorf("workflow %s has no phases", workflowID)
	}
	return e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, def.Phases[0].ID)
}

// AdvancePhase records completion of the current phase and moves to the next.
// Returns the next phase (nil if workflow is complete).
// Idempotent: if the ticket has already moved past currentPhaseID, returns the
// current phase as a no-op (no duplicate completion recorded).
// If version > 0, uses the pinned version's phases.
func (e *Engine) AdvancePhase(ctx context.Context, ticketID, workflowID, currentPhaseID string, outcome string, metadata map[string]any, version int) (*Phase, error) {
	def, err := e.getDefinition(ctx, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	// Idempotency guard: check if the ticket has already advanced past this phase.
	actualPhase, err := e.ticketUpdater.GetWorkflowPhase(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("get current phase: %w", err)
	}
	if actualPhase != currentPhaseID {
		// Already advanced — return current phase as no-op.
		for _, p := range def.Phases {
			if p.ID == actualPhase {
				return &p, nil
			}
		}
		return nil, nil // workflow complete
	}

	// Find current phase index
	currentIdx := -1
	for i, p := range def.Phases {
		if p.ID == currentPhaseID {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		return nil, fmt.Errorf("phase %s not found in workflow %s", currentPhaseID, workflowID)
	}

	// Record completion with started_at from the ticket's phase entry time.
	now := time.Now().UTC()
	if metadata == nil {
		metadata = map[string]any{}
	}
	err = e.store.RecordCompletion(ctx, &PhaseCompletion{
		TicketID:    ticketID,
		WorkflowID:  workflowID,
		PhaseID:     currentPhaseID,
		StartedAt:   now, // store will use workflow_phase_entered_at if available
		CompletedAt: now,
		Outcome:     outcome,
		Metadata:    metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("record completion: %w", err)
	}

	// Handle failure with on_failure jump
	if outcome == "failed" && def.Phases[currentIdx].OnFailure != "" {
		// Check max_iterations for agent phases before applying on_failure
		agentCfg, _ := ParseAgentConfig(def.Phases[currentIdx].Config)
		if agentCfg != nil && agentCfg.MaxIterations > 0 {
			completions, _ := e.store.ListCompletions(ctx, ticketID)
			count := 0
			for _, c := range completions {
				if c.PhaseID == currentPhaseID {
					count++
				}
			}
			if count >= agentCfg.MaxIterations {
				// Max iterations exhausted — treat as success, advance normally
				outcome = "success"
				if metadata == nil {
					metadata = map[string]any{}
				}
				metadata["max_iterations_reached"] = true
				goto advance
			}
		}

		targetID := def.Phases[currentIdx].OnFailure
		for _, p := range def.Phases {
			if p.ID == targetID {
				if err := e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, p.ID); err != nil {
					return nil, err
				}
				return &p, nil
			}
		}
		return nil, fmt.Errorf("on_failure target phase %s not found", targetID)
	}

advance:
	// Advance to next phase
	nextIdx := currentIdx + 1
	if nextIdx >= len(def.Phases) {
		// Workflow complete — clear phase
		if err := e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, ""); err != nil {
			return nil, err
		}
		return nil, nil
	}

	next := def.Phases[nextIdx]
	if err := e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, next.ID); err != nil {
		return nil, err
	}
	return &next, nil
}

// getDefinition returns a definition, using the pinned version if > 0.
func (e *Engine) getDefinition(ctx context.Context, workflowID string, version int) (*Definition, error) {
	if version > 0 {
		return e.store.GetByIDAndVersion(ctx, workflowID, version)
	}
	return e.store.GetByID(ctx, workflowID)
}

// PhaseTypeForTrigger returns the phase type that should handle the given
// internal state machine trigger. Used by the dispatcher to decide whether
// to delegate to the workflow engine.
func PhaseTypeForTrigger(trigger string) PhaseType {
	switch trigger {
	case "submit", "start", "plan", "claim":
		return PhaseAgent
	case "approve", "validate":
		return PhaseGate
	case "deploy":
		return PhaseExternal
	case "observe":
		return PhaseExternal
	default:
		return ""
	}
}
