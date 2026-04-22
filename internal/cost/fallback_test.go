package cost

import (
	"testing"
)

func TestFallbackRouterDefaultPolicies(t *testing.T) {
	router := NewFallbackRouter()

	// Configure models.
	router.SetModel(TierFlagship, "anthropic", "claude-opus-4-20250514")
	router.SetModel(TierMid, "anthropic", "claude-sonnet-4-20250514")
	router.SetModel(TierFast, "anthropic", "claude-haiku-3-20250307")

	tests := []struct {
		op           OperationType
		expectedTier ModelTier
	}{
		{OpClassifier, TierFast},
		{OpSummarization, TierFast},
		{OpStructuralQuery, TierFast},
		{OpCodeGeneration, TierFlagship},
		{OpReview, TierMid},
		{OpPlanning, TierFlagship},
		{OpGeneral, TierMid},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			result := router.Route(tt.op, nil)
			if result.Tier != tt.expectedTier {
				t.Errorf("expected tier %s for op %s, got %s", tt.expectedTier, tt.op, result.Tier)
			}
			if result.Fallback {
				t.Errorf("expected no fallback for op %s", tt.op)
			}
		})
	}
}

func TestFallbackRouterRateLimited(t *testing.T) {
	router := NewFallbackRouter()

	router.SetModel(TierFlagship, "anthropic", "claude-opus-4-20250514")
	router.SetModel(TierMid, "anthropic", "claude-sonnet-4-20250514")
	router.SetModel(TierFast, "anthropic", "claude-haiku-3-20250307")

	// Simulate fast tier being rate-limited for a classifier operation.
	rateLimited := func(provider, model string) bool {
		return provider == "anthropic" && model == "claude-haiku-3-20250307"
	}

	result := router.Route(OpClassifier, rateLimited)
	if result.Tier == TierFast {
		t.Error("expected fallback away from rate-limited fast tier")
	}
	if !result.Fallback {
		t.Error("expected fallback=true when preferred is rate-limited")
	}
}

func TestFallbackRouterNoFallbackForCodeGen(t *testing.T) {
	router := NewFallbackRouter()

	router.SetModel(TierFlagship, "anthropic", "claude-opus-4-20250514")
	router.SetModel(TierMid, "anthropic", "claude-sonnet-4-20250514")
	router.SetModel(TierFast, "anthropic", "claude-haiku-3-20250307")

	// Simulate flagship being rate-limited.
	rateLimited := func(provider, model string) bool {
		return provider == "anthropic" && model == "claude-opus-4-20250514"
	}

	// Code generation does not allow fallback — should still return flagship.
	result := router.Route(OpCodeGeneration, rateLimited)
	if result.Provider != "anthropic" || result.Model != "claude-opus-4-20250514" {
		t.Errorf("expected flagship model for code gen even when rate-limited, got %s/%s", result.Provider, result.Model)
	}
	if result.Fallback {
		t.Error("expected fallback=false for code generation")
	}
}

func TestFallbackRouterCustomPolicy(t *testing.T) {
	router := NewFallbackRouter()

	router.SetModel(TierFlagship, "anthropic", "claude-opus-4-20250514")
	router.SetModel(TierMid, "anthropic", "claude-sonnet-4-20250514")
	router.SetModel(TierFast, "anthropic", "claude-haiku-3-20250307")

	// Override review to use fast tier.
	router.SetPolicy(&FallbackPolicy{
		OperationType: OpReview,
		PreferredTier: TierFast,
		AllowFallback: true,
		MaxTier:       TierMid,
	})

	result := router.Route(OpReview, nil)
	if result.Tier != TierFast {
		t.Errorf("expected fast tier for review with custom policy, got %s", result.Tier)
	}
}

func TestFallbackRouterGetPolicies(t *testing.T) {
	router := NewFallbackRouter()
	policies := router.GetPolicies()

	// Should have default policies.
	if len(policies) == 0 {
		t.Error("expected default policies")
	}
	if _, ok := policies[OpClassifier]; !ok {
		t.Error("expected classifier policy in defaults")
	}
}
