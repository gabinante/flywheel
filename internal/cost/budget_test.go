package cost

import (
	"context"
	"testing"
	"time"
)

func TestBudgetCheckerNoBudget(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)
	checker := NewBudgetChecker(store, tracker, nil)

	ctx := context.Background()

	// No budget set — should always allow.
	status, shouldContinue := checker.CheckBudget(ctx, "proj-1", "ticket-1")
	if !shouldContinue {
		t.Error("expected shouldContinue=true when no budget set")
	}
	if status != nil {
		t.Error("expected nil status when no budget set")
	}
}

func TestBudgetCheckerUnderBudget(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)
	checker := NewBudgetChecker(store, tracker, nil)

	ctx := context.Background()

	// Set a $10 monthly budget.
	budget := &Budget{
		ProjectID:  "proj-1",
		Month:      CurrentMonth(),
		LimitMilli: FromDollars(10.0),
		WarnAt:     0.8,
		HardStop:   false,
	}
	if err := checker.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Record $1 of spend.
	call := &LLMCallRecord{
		ProjectID:     "proj-1",
		TicketID:      "ticket-1",
		CostMillicent: FromDollars(1.0),
		Timestamp:     time.Now().UTC(),
	}
	if err := store.RecordCall(ctx, call); err != nil {
		t.Fatalf("RecordCall: %v", err)
	}

	status, shouldContinue := checker.CheckBudget(ctx, "proj-1", "ticket-1")
	if !shouldContinue {
		t.Error("expected shouldContinue=true when under budget")
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.OverBudget {
		t.Error("expected not over budget")
	}
	if status.Warning {
		t.Error("expected no warning at 10% usage")
	}
}

func TestBudgetCheckerWarning(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	alerts := make([]*BudgetAlert, 0)
	notifier := &testNotifier{alerts: &alerts}
	checker := NewBudgetChecker(store, tracker, notifier)

	ctx := context.Background()

	// Set a $10 monthly budget with 80% warning.
	budget := &Budget{
		ProjectID:  "proj-1",
		Month:      CurrentMonth(),
		LimitMilli: FromDollars(10.0),
		WarnAt:     0.8,
		HardStop:   false,
	}
	if err := checker.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Record $9 of spend (90% of budget).
	call := &LLMCallRecord{
		ProjectID:     "proj-1",
		TicketID:      "ticket-1",
		CostMillicent: FromDollars(9.0),
		Timestamp:     time.Now().UTC(),
	}
	if err := store.RecordCall(ctx, call); err != nil {
		t.Fatalf("RecordCall: %v", err)
	}

	status, shouldContinue := checker.CheckBudget(ctx, "proj-1", "ticket-1")
	if !shouldContinue {
		t.Error("expected shouldContinue=true (warn, don't stop)")
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if !status.Warning {
		t.Error("expected warning at 90% usage")
	}
	if len(alerts) == 0 {
		t.Error("expected alert to be pushed")
	}
}

func TestBudgetCheckerExceeded(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	alerts := make([]*BudgetAlert, 0)
	notifier := &testNotifier{alerts: &alerts}
	checker := NewBudgetChecker(store, tracker, notifier)

	ctx := context.Background()

	// Set a $10 monthly budget, no hard stop.
	budget := &Budget{
		ProjectID:  "proj-1",
		Month:      CurrentMonth(),
		LimitMilli: FromDollars(10.0),
		WarnAt:     0.8,
		HardStop:   false, // warn, don't stop
	}
	if err := checker.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Record $12 of spend (over budget).
	call := &LLMCallRecord{
		ProjectID:     "proj-1",
		TicketID:      "ticket-1",
		CostMillicent: FromDollars(12.0),
		Timestamp:     time.Now().UTC(),
	}
	if err := store.RecordCall(ctx, call); err != nil {
		t.Fatalf("RecordCall: %v", err)
	}

	status, shouldContinue := checker.CheckBudget(ctx, "proj-1", "ticket-1")
	// Per constraint: must not hard-stop when budget low.
	if !shouldContinue {
		t.Error("expected shouldContinue=true when HardStop=false (warn, don't stop)")
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if !status.OverBudget {
		t.Error("expected over budget")
	}
}

func TestBudgetCheckerHardStop(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)
	checker := NewBudgetChecker(store, tracker, nil)

	ctx := context.Background()

	// Set a $5 budget with hard stop explicitly enabled.
	budget := &Budget{
		ProjectID:  "proj-1",
		Month:      CurrentMonth(),
		LimitMilli: FromDollars(5.0),
		WarnAt:     0.8,
		HardStop:   true, // operator explicitly requested hard stop
	}
	if err := checker.SetBudget(ctx, budget); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Record $6 of spend.
	call := &LLMCallRecord{
		ProjectID:     "proj-1",
		TicketID:      "ticket-1",
		CostMillicent: FromDollars(6.0),
		Timestamp:     time.Now().UTC(),
	}
	if err := store.RecordCall(ctx, call); err != nil {
		t.Fatalf("RecordCall: %v", err)
	}

	_, shouldContinue := checker.CheckBudget(ctx, "proj-1", "ticket-1")
	if shouldContinue {
		t.Error("expected shouldContinue=false when HardStop=true and over budget")
	}
}

// testNotifier captures alerts for testing.
type testNotifier struct {
	alerts *[]*BudgetAlert
}

func (n *testNotifier) NotifyBudgetAlert(_ context.Context, alert *BudgetAlert) error {
	*n.alerts = append(*n.alerts, alert)
	return nil
}
