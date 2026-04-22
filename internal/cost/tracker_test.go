package cost

import (
	"context"
	"testing"
	"time"
)

func TestTrackerRecordCall(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	ctx := context.Background()

	call := &LLMCallRecord{
		ProjectID:    "proj-1",
		TicketID:     "ticket-1",
		WorkerRole:   "executor",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  5000,
		OutputTokens: 2000,
	}

	err := tracker.RecordCall(ctx, call)
	if err != nil {
		t.Fatalf("RecordCall: %v", err)
	}

	// Should have auto-generated ID and timestamp.
	if call.ID == "" {
		t.Error("expected ID to be set")
	}
	if call.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}

	// Should have calculated cost.
	if call.CostMillicent == 0 {
		t.Error("expected CostMillicent to be calculated")
	}

	// Should have resolved tier.
	if call.ModelTier != TierMid {
		t.Errorf("expected tier %s, got %s", TierMid, call.ModelTier)
	}
}

func TestTrackerGetTicketCost(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	ctx := context.Background()

	// Record multiple calls for the same ticket.
	for i := 0; i < 3; i++ {
		call := &LLMCallRecord{
			ProjectID:     "proj-1",
			TicketID:      "ticket-1",
			WorkerRole:    "executor",
			Provider:      "anthropic",
			Model:         "claude-sonnet-4-20250514",
			InputTokens:   1000,
			OutputTokens:  500,
			CostMillicent: 1000, // explicit cost
		}
		if err := tracker.RecordCall(ctx, call); err != nil {
			t.Fatalf("RecordCall: %v", err)
		}
	}

	total, err := tracker.GetTicketCost(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("GetTicketCost: %v", err)
	}
	if total != 3000 {
		t.Errorf("expected 3000, got %d", total)
	}
}

func TestTrackerGetProjectCost(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	ctx := context.Background()

	// Record calls across tickets.
	tickets := []string{"ticket-1", "ticket-2", "ticket-3"}
	for _, tid := range tickets {
		call := &LLMCallRecord{
			ProjectID:     "proj-1",
			TicketID:      tid,
			WorkerRole:    "executor",
			Provider:      "anthropic",
			Model:         "claude-sonnet-4-20250514",
			InputTokens:   1000,
			OutputTokens:  500,
			CostMillicent: 2000,
			Timestamp:     time.Now().UTC(),
		}
		if err := tracker.RecordCall(ctx, call); err != nil {
			t.Fatalf("RecordCall: %v", err)
		}
	}

	month := CurrentMonth()
	total, err := tracker.GetProjectCost(ctx, "proj-1", month)
	if err != nil {
		t.Fatalf("GetProjectCost: %v", err)
	}
	if total != 6000 {
		t.Errorf("expected 6000, got %d", total)
	}
}

func TestTrackerCostSummary(t *testing.T) {
	store := NewMemStore()
	pricing := NewPricingRegistry()
	tracker := NewTracker(store, pricing)

	ctx := context.Background()

	calls := []*LLMCallRecord{
		{ProjectID: "proj-1", TicketID: "t1", WorkerRole: "executor", Provider: "anthropic", Model: "claude-sonnet-4-20250514", CostMillicent: 1000, Timestamp: time.Now().UTC()},
		{ProjectID: "proj-1", TicketID: "t1", WorkerRole: "reviewer", Provider: "anthropic", Model: "claude-haiku-3-20250307", CostMillicent: 200, Timestamp: time.Now().UTC()},
		{ProjectID: "proj-1", TicketID: "t2", WorkerRole: "executor", Provider: "openai", Model: "gpt-4o", CostMillicent: 800, Timestamp: time.Now().UTC()},
	}
	for _, c := range calls {
		if err := tracker.RecordCall(ctx, c); err != nil {
			t.Fatalf("RecordCall: %v", err)
		}
	}

	summary, err := tracker.GetCostSummary(ctx, "proj-1", CurrentMonth())
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}

	if summary.TotalMilli != 2000 {
		t.Errorf("expected total 2000, got %d", summary.TotalMilli)
	}
	if summary.CallCount != 3 {
		t.Errorf("expected 3 calls, got %d", summary.CallCount)
	}
	if summary.ByTicket["t1"] != 1200 {
		t.Errorf("expected t1 cost 1200, got %d", summary.ByTicket["t1"])
	}
	if summary.ByRole["executor"] != 1800 {
		t.Errorf("expected executor cost 1800, got %d", summary.ByRole["executor"])
	}
	if summary.ByProvider["anthropic"] != 1200 {
		t.Errorf("expected anthropic cost 1200, got %d", summary.ByProvider["anthropic"])
	}
}
