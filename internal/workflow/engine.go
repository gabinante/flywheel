package workflow

import (
	"context"
	"fmt"
	"sync"
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
	mu            sync.Mutex
	store         DefinitionStore
	ticketUpdater TicketUpdater
	complete      func(context.Context, string) error
}

// NewEngine returns a new workflow engine.
func NewEngine(store DefinitionStore, ticketUpdater TicketUpdater) *Engine {
	return &Engine{store: store, ticketUpdater: ticketUpdater}
}

// SetCompletion connects the terminal ticket transition to the phase transaction.
// Configure this once before processing workflows.
func (e *Engine) SetCompletion(fn func(context.Context, string) error) { e.complete = fn }

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

	normalizeLegacyMerge(def)
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
	var next *Phase
	advance := func(ctx context.Context) error {
		var err error
		next, err = e.advancePhase(ctx, ticketID, workflowID, currentPhaseID, outcome, metadata, version)
		return err
	}
	var err error
	if tx, ok := e.store.(interface {
		WithAdvanceTransaction(context.Context, string, func(context.Context) error) error
	}); ok {
		err = tx.WithAdvanceTransaction(ctx, ticketID, advance)
	} else {
		e.mu.Lock()
		defer e.mu.Unlock()
		err = advance(ctx)
	}
	return next, err
}

func (e *Engine) advancePhase(ctx context.Context, ticketID, workflowID, currentPhaseID string, outcome string, metadata map[string]any, version int) (*Phase, error) {
	if outcome != "success" && outcome != "failed" && outcome != "skipped" {
		return nil, fmt.Errorf("invalid phase outcome %q", outcome)
	}

	def, err := e.getDefinition(ctx, workflowID, version)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	if expected, ok := metadata["phase_entered_at"].(string); ok && expected != "" {
		if updater, ok := e.ticketUpdater.(interface {
			WorkflowAttempt(context.Context, string) (*time.Time, error)
		}); ok {
			entered, err := updater.WorkflowAttempt(ctx, ticketID)
			if err != nil {
				return nil, err
			}
			parsed, parseErr := time.Parse(time.RFC3339Nano, expected)
			if entered == nil || parseErr != nil || !entered.Equal(parsed) {
				return nil, fmt.Errorf("stale phase attempt")
			}
		}
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

	if getter, ok := e.ticketUpdater.(interface {
		WorkflowAttemptStatus(context.Context, string) (string, error)
	}); ok {
		status, err := getter.WorkflowAttemptStatus(ctx, ticketID)
		if err != nil {
			return nil, err
		}
		if status == "failed" {
			return nil, fmt.Errorf("failed phase requires explicit retry")
		}
	}
	// Find current phase index
	currentIdx := -1
	normalizeLegacyMerge(def)
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

	current := def.Phases[currentIdx]
	if outcome == "failed" {
		cfg, _ := ParseAgentConfig(current.Config)
		completions, err := e.store.ListCompletions(ctx, ticketID)
		if err != nil {
			return &current, err
		}
		count := 0
		for _, c := range completions {
			if c.PhaseID == currentPhaseID {
				count++
			}
		}
		if current.OnFailure == "" || (cfg.MaxIterations > 0 && count >= cfg.MaxIterations) {
			// Failed work remains visible and requires an explicit operator retry.
			if err := e.ticketUpdater.UpdateWorkflowPhaseStatus(ctx, ticketID, "failed"); err != nil {
				return &current, err
			}
			return &current, nil
		}
		for _, p := range def.Phases {
			if p.ID == current.OnFailure {
				if err := e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, p.ID); err != nil {
					return &current, err
				}
				return &p, nil
			}
		}
		return &current, fmt.Errorf("on_failure target phase %s not found", current.OnFailure)
	}

	// Advance to next phase
	nextIdx := currentIdx + 1
	if nextIdx >= len(def.Phases) {
		// Workflow complete — clear phase
		if err := e.ticketUpdater.UpdateWorkflowPhase(ctx, ticketID, ""); err != nil {
			return nil, err
		}
		if e.complete != nil {
			if err := e.complete(ctx, ticketID); err != nil {
				return nil, err
			}
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

func normalizeLegacyMerge(def *Definition) {
	if def == nil {
		return
	}
	for i := range def.Phases {
		p := &def.Phases[i]
		if p.ID == "merge" && p.Type == PhaseExternal {
			cfg, _ := ParseExternalConfig(p.Config)
			if cfg != nil && cfg.URL == "" && cfg.PollURL == "" {
				p.Type = PhaseAction
				p.Config = map[string]any{"action": "merge_pr"}
			}
		}
	}
}
