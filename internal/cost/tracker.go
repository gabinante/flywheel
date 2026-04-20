package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tracker records and attributes LLM calls to tickets, projects, and workers.
// It is the primary entry point for cost attribution.
type Tracker struct {
	store   Store
	pricing *PricingRegistry
	mu      sync.RWMutex
	// In-flight tracking for rate of spend projection.
	recentCalls []LLMCallRecord
}

// NewTracker creates a new cost tracker with the given store and pricing registry.
func NewTracker(store Store, pricing *PricingRegistry) *Tracker {
	return &Tracker{
		store:   store,
		pricing: pricing,
	}
}

// RecordCall records a single LLM call with full attribution.
// The cost is automatically calculated from the pricing registry.
func (t *Tracker) RecordCall(ctx context.Context, call *LLMCallRecord) error {
	if call.ID == "" {
		call.ID = uuid.New().String()
	}
	if call.Timestamp.IsZero() {
		call.Timestamp = time.Now().UTC()
	}

	// Calculate cost from pricing if not already set.
	if call.CostMillicent == 0 {
		cost := t.pricing.CalculateCost(call.Provider, call.Model, call.InputTokens, call.OutputTokens)
		call.CostMillicent = cost
	}

	// Resolve model tier if not set.
	if call.ModelTier == "" {
		call.ModelTier = t.pricing.GetTier(call.Provider, call.Model)
	}

	// Track recent calls for projection.
	t.mu.Lock()
	t.recentCalls = append(t.recentCalls, *call)
	// Keep only last 1000 calls for projection window.
	if len(t.recentCalls) > 1000 {
		t.recentCalls = t.recentCalls[len(t.recentCalls)-1000:]
	}
	t.mu.Unlock()

	return t.store.RecordCall(ctx, call)
}

// GetTicketCost returns the total cost for a ticket.
func (t *Tracker) GetTicketCost(ctx context.Context, ticketID string) (Unit, error) {
	return t.store.SumByTicket(ctx, ticketID)
}

// GetProjectCost returns the total cost for a project in a given month.
func (t *Tracker) GetProjectCost(ctx context.Context, projectID, month string) (Unit, error) {
	return t.store.SumByProject(ctx, projectID, month)
}

// GetCostSummary returns a detailed cost breakdown for a project.
func (t *Tracker) GetCostSummary(ctx context.Context, projectID, month string) (*CostSummary, error) {
	return t.store.SumByProjectGrouped(ctx, projectID, month)
}

// ProjectMonthSpendRate returns the average daily spend rate for the current month.
// Used for budget projections.
func (t *Tracker) ProjectMonthSpendRate(ctx context.Context, projectID string) (Unit, error) {
	now := time.Now().UTC()
	month := now.Format("2006-01")
	spent, err := t.store.SumByProject(ctx, projectID, month)
	if err != nil {
		return 0, err
	}

	dayOfMonth := now.Day()
	if dayOfMonth == 0 {
		dayOfMonth = 1
	}

	dailyRate := spent / Unit(dayOfMonth)
	return dailyRate, nil
}

// ProjectMonthEnd projects the month-end spend based on current rate.
func (t *Tracker) ProjectMonthEnd(ctx context.Context, projectID string) (Unit, error) {
	now := time.Now().UTC()
	month := now.Format("2006-01")
	spent, err := t.store.SumByProject(ctx, projectID, month)
	if err != nil {
		return 0, err
	}

	dayOfMonth := now.Day()
	if dayOfMonth == 0 {
		dayOfMonth = 1
	}

	// Days remaining in month.
	firstOfNext := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := firstOfNext.Add(-24 * time.Hour).Day()
	daysRemaining := daysInMonth - dayOfMonth

	dailyRate := spent / Unit(dayOfMonth)
	projected := spent + (dailyRate * Unit(daysRemaining))

	return projected, nil
}

// CurrentMonth returns the current month string in "2006-01" format.
func CurrentMonth() string {
	return time.Now().UTC().Format("2006-01")
}

// FormatCost returns a human-readable cost string.
func FormatCost(milli Unit) string {
	dollars := milli.ToDollars()
	if dollars < 0.01 {
		return fmt.Sprintf("%.4f", dollars)
	}
	return fmt.Sprintf("$%.2f", dollars)
}
