package cost

import (
	"context"
	"testing"
	"time"
)

func TestServiceRecordAndCheck(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	ctx := context.Background()

	// Set a project budget.
	if err := svc.SetProjectBudget(ctx, "proj-1", 50.0); err != nil {
		t.Fatalf("SetProjectBudget: %v", err)
	}

	// Record a call.
	call := &LLMCallRecord{
		ProjectID:    "proj-1",
		TicketID:     "ticket-1",
		WorkerRole:   "executor",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  10000,
		OutputTokens: 5000,
		Timestamp:    time.Now().UTC(),
	}

	shouldContinue, status := svc.RecordAndCheck(ctx, call)
	if !shouldContinue {
		t.Error("expected shouldContinue=true for small spend")
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.OverBudget {
		t.Error("expected not over budget")
	}
}

func TestServiceRouteModel(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	// Test routing for different operation types.
	tests := []struct {
		op   OperationType
		tier ModelTier
	}{
		{OpClassifier, TierFast},
		{OpCodeGeneration, TierFlagship},
		{OpReview, TierMid},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			result := svc.RouteModel(tt.op)
			if result.Tier != tt.tier {
				t.Errorf("expected tier %s, got %s", tt.tier, result.Tier)
			}
		})
	}
}

func TestServiceHandleRateLimit(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	ctx := context.Background()

	// Handle a rate limit.
	wait, err := svc.HandleRateLimit(ctx, "proj-1", "t1", "anthropic", "claude-sonnet-4-20250514", 45*time.Second)
	if err != nil {
		t.Fatalf("HandleRateLimit: %v", err)
	}
	if wait != 45*time.Second {
		t.Errorf("expected 45s, got %v", wait)
	}

	// Route should detect rate limit and fall back.
	result := svc.RouteModel(OpGeneral) // General prefers mid tier (sonnet)
	// Since sonnet is rate-limited, should fall back.
	if result.Provider == "anthropic" && result.Model == "claude-sonnet-4-20250514" && !result.Fallback {
		// If it still returns sonnet, it should be marked as no-fallback-available.
		if result.Reason == "" {
			t.Error("expected reason to be set")
		}
	}
}

func TestServicePreviewTicketCost(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	preview := svc.PreviewTicketCost(ScopeHint{
		Type:       "task",
		Complexity: "medium",
		FilesCount: 5,
		HasTests:   true,
	})

	if preview == nil {
		t.Fatal("expected non-nil preview")
	}
	if preview.TotalEstimate == 0 {
		t.Error("expected non-zero total estimate")
	}
	if preview.Confidence != "medium" {
		t.Errorf("expected medium confidence, got %s", preview.Confidence)
	}
}

func TestServiceSetTicketBudget(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	ctx := context.Background()

	err := svc.SetTicketBudget(ctx, "proj-1", "ticket-1", 5.0)
	if err != nil {
		t.Fatalf("SetTicketBudget: %v", err)
	}

	// Verify it was stored.
	budget, err := store.GetBudget(ctx, "proj-1", "ticket-1", "")
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if budget == nil {
		t.Fatal("expected budget to be stored")
	}
	if budget.LimitMilli != FromDollars(5.0) {
		t.Errorf("expected limit %d, got %d", FromDollars(5.0), budget.LimitMilli)
	}
}

func TestServiceGetProjectStatus(t *testing.T) {
	store := NewMemStore()
	cfg := DefaultConfig()
	svc := NewService(store, cfg, nil, nil)

	ctx := context.Background()

	// No budget set.
	status, err := svc.GetProjectStatus(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetProjectStatus: %v", err)
	}
	if status != nil {
		t.Error("expected nil status when no budget set")
	}

	// Set budget and check.
	if err := svc.SetProjectBudget(ctx, "proj-1", 100.0); err != nil {
		t.Fatalf("SetProjectBudget: %v", err)
	}

	status, err = svc.GetProjectStatus(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetProjectStatus: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Budget.LimitMilli != FromDollars(100.0) {
		t.Errorf("expected limit $100, got %d millicents", status.Budget.LimitMilli)
	}
}
