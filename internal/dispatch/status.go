package dispatch

import (
	"context"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/ticket"
)

// StatusDiagnostics contains ticket counts by state for diagnostic display.
type StatusDiagnostics struct {
	DraftCount              int `json:"draft_count"`
	PlanningCount           int `json:"planning_count"`
	ExecutingCount          int `json:"executing_count"`
	AwaitingValidationCount int `json:"awaiting_validation_count"`
	ValidatedCount          int `json:"validated_count"`
	AwaitingInputCount      int `json:"awaiting_input_count"`
	ClosedCount             int `json:"closed_count"`
}

// Status represents the current state of the dispatcher.
type Status struct {
	Enabled         bool               `json:"enabled"`
	ActiveWorkers   int                `json:"active_workers"`
	MaxWorkers      int                `json:"max_workers"`
	ActiveTicketIDs []string           `json:"active_ticket_ids"`
	Timestamp       time.Time          `json:"timestamp"`
	IdleReason      string             `json:"idle_reason,omitempty"`
	Diagnostics     *StatusDiagnostics `json:"diagnostics,omitempty"`
}

// GetStatus returns the current dispatcher status with optional project-scoped diagnostics.
func (d *Dispatcher) GetStatus(ctx context.Context, projectID string) Status {
	s := d.Summary()

	// Collect diagnostics and determine idle reason.
	diag := d.collectDiagnostics(ctx, projectID)
	s.Diagnostics = diag

	if s.ActiveWorkers == 0 {
		s.IdleReason = d.determineIdleReason(ctx, projectID, &s, diag)
		if !s.Enabled {
			s.IdleReason = "dispatch_disabled"
		}
	}

	return s
}

// collectDiagnostics queries ticket counts by state for the given project (or globally).
func (d *Dispatcher) collectDiagnostics(ctx context.Context, projectID string) *StatusDiagnostics {
	diag := &StatusDiagnostics{}

	states := []struct {
		state ticket.State
		dest  *int
	}{
		{ticket.StateDraft, &diag.DraftCount},
		{ticket.StatePlanning, &diag.PlanningCount},
		{ticket.StateExecuting, &diag.ExecutingCount},
		{ticket.StateAwaitingValidation, &diag.AwaitingValidationCount},
		{ticket.StateValidated, &diag.ValidatedCount},
		{ticket.StateAwaitingInput, &diag.AwaitingInputCount},
		{ticket.StateClosed, &diag.ClosedCount},
	}

	for _, s := range states {
		tickets, err := d.tickets.ListByState(ctx, projectID, s.state)
		if err == nil {
			*s.dest = len(tickets)
		}
	}

	return diag
}

// determineIdleReason walks a decision tree to produce a human-readable reason
// for why the dispatcher is idle.
func (d *Dispatcher) determineIdleReason(ctx context.Context, projectID string, s *Status, diag *StatusDiagnostics) string {
	// 1. Project-level dispatch disabled
	if projectID != "" && !d.isProjectDispatchEnabled(ctx, projectID) {
		return "dispatch_disabled_project"
	}

	// 2. No repo configured but drafts exist
	if projectID != "" && diag.DraftCount > 0 && !d.projectHasRepo(ctx, projectID) {
		return "no_repo_configured"
	}

	// 3. At capacity
	if s.ActiveWorkers >= s.MaxWorkers {
		return "at_capacity"
	}

	// 4. Tickets awaiting review or merge
	if diag.AwaitingValidationCount > 0 && diag.ValidatedCount > 0 {
		return "review_and_merge_pending"
	}
	if diag.AwaitingValidationCount > 0 {
		return "review_pending"
	}
	if diag.ValidatedCount > 0 {
		return "merge_pending"
	}

	// 5. All drafts blocked by dependencies
	if diag.DraftCount > 0 {
		allBlocked := d.allDraftsBlocked(ctx, projectID)
		if allBlocked {
			return "deps_not_met"
		}
	}

	// 6. Only awaiting human input
	if diag.AwaitingInputCount > 0 && diag.DraftCount == 0 && diag.PlanningCount == 0 && diag.ExecutingCount == 0 {
		return "awaiting_human_input"
	}

	// 7. No tickets in any active state
	activeStates := diag.DraftCount + diag.PlanningCount + diag.ExecutingCount +
		diag.AwaitingValidationCount + diag.ValidatedCount + diag.AwaitingInputCount
	if activeStates == 0 {
		return "all_work_complete"
	}

	// 8. No draft tickets but other active states exist
	if diag.DraftCount == 0 {
		return "no_draft_tickets"
	}

	return "idle"
}

// allDraftsBlocked checks whether every draft ticket has unmet dependencies.
func (d *Dispatcher) allDraftsBlocked(ctx context.Context, projectID string) bool {
	drafts, err := d.tickets.ListByState(ctx, projectID, ticket.StateDraft)
	if err != nil || len(drafts) == 0 {
		return false
	}

	for _, t := range drafts {
		if len(t.DependsOn) == 0 {
			return false
		}
		deps, err := d.tickets.GetTicketsByIDs(ctx, t.DependsOn)
		if err != nil {
			return false
		}
		allMet := true
		for _, dep := range deps {
			if dep.State != ticket.StateClosed {
				allMet = false
				break
			}
		}
		if allMet {
			return false // this draft has all deps met, so not all are blocked
		}
	}
	return true
}

// Summary reports global dispatch capacity without scanning ticket history.
func (d *Dispatcher) Summary() Status {
	d.mu.Lock()
	ids := make([]string, 0, len(d.active))
	seen := make(map[string]struct{}, len(d.active))
	for id := range d.active {
		ticketID := strings.TrimPrefix(id, "review:")
		ticketID = strings.TrimPrefix(ticketID, "resolve:")
		if ticketID == "" {
			continue
		}
		if _, ok := seen[ticketID]; ok {
			continue
		}
		seen[ticketID] = struct{}{}
		ids = append(ids, ticketID)
	}
	d.mu.Unlock()

	return Status{
		Enabled:         d.enabled.Load(),
		ActiveWorkers:   len(ids),
		MaxWorkers:      d.config().MaxWorkers,
		ActiveTicketIDs: ids,
		Timestamp:       time.Now().UTC(),
	}

}
