package cost

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// BudgetChecker evaluates budget status and emits alerts.
// It is called after each LLM call to check if budgets are at risk.
type BudgetChecker struct {
	store   Store
	tracker *Tracker
	notify  AlertNotifier
}

// AlertNotifier is the interface for pushing budget alerts to operators.
// Implementations may use webhooks, email, Slack, or the event bus.
type AlertNotifier interface {
	NotifyBudgetAlert(ctx context.Context, alert *BudgetAlert) error
}

// NoopNotifier discards alerts (used when no notification channel is configured).
type NoopNotifier struct{}

// NotifyBudgetAlert is a no-op.
func (n *NoopNotifier) NotifyBudgetAlert(_ context.Context, _ *BudgetAlert) error { return nil }

// NewBudgetChecker creates a budget checker.
func NewBudgetChecker(store Store, tracker *Tracker, notify AlertNotifier) *BudgetChecker {
	if notify == nil {
		notify = &NoopNotifier{}
	}
	return &BudgetChecker{
		store:   store,
		tracker: tracker,
		notify:  notify,
	}
}

// SetBudget creates or updates a budget for a project/ticket/month scope.
func (bc *BudgetChecker) SetBudget(ctx context.Context, budget *Budget) error {
	if budget.ID == "" {
		budget.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if budget.CreatedAt.IsZero() {
		budget.CreatedAt = now
	}
	budget.UpdatedAt = now
	if budget.WarnAt == 0 {
		budget.WarnAt = 0.8 // default: warn at 80%
	}
	return bc.store.SetBudget(ctx, budget)
}

// CheckBudget evaluates the current budget status for a project.
// Returns the status and whether work should continue.
// Per constraint: warn, don't hard-stop (unless HardStop is explicitly set).
func (bc *BudgetChecker) CheckBudget(ctx context.Context, projectID, ticketID string) (*BudgetStatus, bool) {
	month := CurrentMonth()

	// Check project-level monthly budget first.
	budget, err := bc.store.GetBudget(ctx, projectID, "", month)
	if err != nil || budget == nil {
		// No budget set — always allow.
		return nil, true
	}

	spent, err := bc.store.SumByProject(ctx, projectID, month)
	if err != nil {
		slog.Error("cost: budget check failed", "project", projectID, "error", err)
		return nil, true // fail open
	}

	status := bc.computeStatus(ctx, *budget, spent, projectID)

	// Check ticket-level budget if applicable.
	if ticketID != "" {
		ticketBudget, err := bc.store.GetBudget(ctx, projectID, ticketID, "")
		if err == nil && ticketBudget != nil {
			ticketSpent, err := bc.store.SumByTicket(ctx, ticketID)
			if err == nil {
				ticketStatus := bc.computeStatus(ctx, *ticketBudget, ticketSpent, projectID)
				// Use the more restrictive status.
				if ticketStatus.UsedFraction > status.UsedFraction {
					status = ticketStatus
				}
			}
		}
	}

	// Emit alerts if needed.
	bc.maybeAlert(ctx, projectID, ticketID, status)

	// Decision: warn but don't block (unless HardStop is explicitly set).
	shouldContinue := true
	if status.Budget.HardStop && status.OverBudget {
		shouldContinue = false
	}

	return status, shouldContinue
}

// GetBudgetStatus returns the current budget status for a project without side effects.
func (bc *BudgetChecker) GetBudgetStatus(ctx context.Context, projectID string) (*BudgetStatus, error) {
	month := CurrentMonth()
	budget, err := bc.store.GetBudget(ctx, projectID, "", month)
	if err != nil {
		return nil, err
	}
	if budget == nil {
		return nil, nil
	}

	spent, err := bc.store.SumByProject(ctx, projectID, month)
	if err != nil {
		return nil, err
	}

	status := bc.computeStatus(ctx, *budget, spent, projectID)
	return status, nil
}

func (bc *BudgetChecker) computeStatus(ctx context.Context, budget Budget, spent Unit, projectID string) *BudgetStatus {
	remaining := budget.LimitMilli - spent
	if remaining < 0 {
		remaining = 0
	}

	usedFraction := float64(0)
	if budget.LimitMilli > 0 {
		usedFraction = float64(spent) / float64(budget.LimitMilli)
	}

	projected, _ := bc.tracker.ProjectMonthEnd(ctx, projectID)

	return &BudgetStatus{
		Budget:       budget,
		SpentMilli:   spent,
		Remaining:    remaining,
		UsedFraction: usedFraction,
		Projected:    projected,
		OverBudget:   spent >= budget.LimitMilli,
		Warning:      usedFraction >= budget.WarnAt,
	}
}

func (bc *BudgetChecker) maybeAlert(ctx context.Context, projectID, ticketID string, status *BudgetStatus) {
	if status == nil {
		return
	}

	var alertType AlertType
	var message string

	switch {
	case status.OverBudget:
		alertType = AlertExceeded
		message = fmt.Sprintf("Budget exceeded for project %s: spent %s of %s limit",
			projectID, FormatCost(status.SpentMilli), FormatCost(status.Budget.LimitMilli))
	case status.Projected > status.Budget.LimitMilli:
		alertType = AlertProjection
		message = fmt.Sprintf("Projected month-end spend %s exceeds budget %s for project %s",
			FormatCost(status.Projected), FormatCost(status.Budget.LimitMilli), projectID)
	case status.Warning:
		alertType = AlertWarning
		message = fmt.Sprintf("Budget warning for project %s: %.0f%% used (%s of %s)",
			projectID, status.UsedFraction*100, FormatCost(status.SpentMilli), FormatCost(status.Budget.LimitMilli))
	default:
		return // no alert needed
	}

	alert := &BudgetAlert{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		TicketID:  ticketID,
		AlertType: alertType,
		Status:    *status,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	}

	if err := bc.store.CreateAlert(ctx, alert); err != nil {
		slog.Error("cost: failed to create alert", "error", err)
		return
	}

	if err := bc.notify.NotifyBudgetAlert(ctx, alert); err != nil {
		slog.Error("cost: failed to notify alert", "error", err)
	}
}
