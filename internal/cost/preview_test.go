package cost

import (
	"testing"
)

func TestPreviewEstimator(t *testing.T) {
	pricing := NewPricingRegistry()
	router := NewFallbackRouter()

	// Configure models for preview calculation.
	router.SetModel(TierFlagship, "anthropic", "claude-opus-4-20250514")
	router.SetModel(TierMid, "anthropic", "claude-sonnet-4-20250514")
	router.SetModel(TierFast, "anthropic", "claude-haiku-3-20250307")

	estimator := NewPreviewEstimator(pricing, router)

	tests := []struct {
		name       string
		hint       ScopeHint
		minDollars float64
		maxDollars float64
		confidence string
	}{
		{
			name:       "low complexity task",
			hint:       ScopeHint{Type: "task", Complexity: "low", FilesCount: 2, HasTests: false},
			minDollars: 0.001,
			maxDollars: 5.0,
			confidence: "high",
		},
		{
			name:       "medium complexity task with tests",
			hint:       ScopeHint{Type: "task", Complexity: "medium", FilesCount: 5, HasTests: true},
			minDollars: 0.01,
			maxDollars: 50.0,
			confidence: "medium",
		},
		{
			name:       "high complexity task",
			hint:       ScopeHint{Type: "task", Complexity: "high", FilesCount: 15, HasTests: true},
			minDollars: 0.1,
			maxDollars: 200.0,
			confidence: "low",
		},
		{
			name:       "bug fix",
			hint:       ScopeHint{Type: "bug", Complexity: "medium", FilesCount: 3, HasTests: true},
			minDollars: 0.01,
			maxDollars: 50.0,
			confidence: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := estimator.EstimateCost(tt.hint)

			if preview.TotalDollars < tt.minDollars {
				t.Errorf("total $%.4f below minimum $%.4f", preview.TotalDollars, tt.minDollars)
			}
			if preview.TotalDollars > tt.maxDollars {
				t.Errorf("total $%.4f above maximum $%.4f", preview.TotalDollars, tt.maxDollars)
			}
			if preview.Confidence != tt.confidence {
				t.Errorf("expected confidence %s, got %s", tt.confidence, preview.Confidence)
			}
			if preview.EstimatedCalls == 0 {
				t.Error("expected non-zero estimated calls")
			}
			if preview.CoordinatorCost == 0 {
				t.Error("expected non-zero coordinator cost")
			}
			if preview.WorkerCost == 0 {
				t.Error("expected non-zero worker cost")
			}
			if preview.TotalEstimate == 0 {
				t.Error("expected non-zero total estimate")
			}
			if preview.Assumptions == "" {
				t.Error("expected assumptions to be set")
			}
		})
	}
}

func TestPreviewEstimatorNoModelsConfigured(t *testing.T) {
	pricing := NewPricingRegistry()
	router := NewFallbackRouter()
	// Deliberately don't configure any models.

	estimator := NewPreviewEstimator(pricing, router)

	hint := ScopeHint{Type: "task", Complexity: "medium"}
	preview := estimator.EstimateCost(hint)

	// Should not panic and should return zero costs (no models configured).
	if preview == nil {
		t.Fatal("expected non-nil preview")
	}
	if preview.EstimatedCalls == 0 {
		t.Error("expected non-zero estimated calls even without models")
	}
}
