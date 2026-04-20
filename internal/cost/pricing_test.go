package cost

import (
	"testing"
)

func TestPricingRegistryDefaults(t *testing.T) {
	r := NewPricingRegistry()

	models := r.ListModels()
	if len(models) == 0 {
		t.Fatal("expected default models to be registered")
	}

	// Check that known models are present.
	knownModels := []struct {
		provider string
		model    string
		tier     ModelTier
	}{
		{"anthropic", "claude-opus-4-20250514", TierFlagship},
		{"anthropic", "claude-sonnet-4-20250514", TierMid},
		{"anthropic", "claude-haiku-3-20250307", TierFast},
		{"openai", "gpt-4o", TierMid},
		{"openai", "gpt-4o-mini", TierFast},
	}

	for _, km := range knownModels {
		p := r.GetPricing(km.provider, km.model)
		if p == nil {
			t.Errorf("expected pricing for %s/%s", km.provider, km.model)
			continue
		}
		if p.Tier != km.tier {
			t.Errorf("expected tier %s for %s/%s, got %s", km.tier, km.provider, km.model, p.Tier)
		}
	}
}

func TestPricingRegistryCalculateCost(t *testing.T) {
	r := NewPricingRegistry()

	// Sonnet: $3/M input, $15/M output in millicents: 300_000/M and 1_500_000/M.
	cost := r.CalculateCost("anthropic", "claude-sonnet-4-20250514", 1_000_000, 0)
	if cost != 300_000 {
		t.Errorf("expected 300_000 for 1M input tokens, got %d", cost)
	}

	cost = r.CalculateCost("anthropic", "claude-sonnet-4-20250514", 0, 1_000_000)
	if cost != 1_500_000 {
		t.Errorf("expected 1_500_000 for 1M output tokens, got %d", cost)
	}

	// Combined.
	cost = r.CalculateCost("anthropic", "claude-sonnet-4-20250514", 1000, 500)
	expectedInput := Unit(1000) * 300_000 / 1_000_000   // 0
	expectedOutput := Unit(500) * 1_500_000 / 1_000_000 // 0
	expected := expectedInput + expectedOutput
	if cost != expected {
		t.Errorf("expected %d, got %d", expected, cost)
	}
}

func TestPricingRegistryUnknownModel(t *testing.T) {
	r := NewPricingRegistry()

	// Unknown model should use mid-tier estimate.
	cost := r.CalculateCost("unknown", "unknown-model", 1_000_000, 0)
	if cost == 0 {
		t.Error("expected non-zero cost for unknown model")
	}

	tier := r.GetTier("unknown", "unknown-model")
	if tier != TierMid {
		t.Errorf("expected TierMid for unknown model, got %s", tier)
	}
}

func TestPricingRegistryCustomModel(t *testing.T) {
	r := NewPricingRegistry()

	// Register a custom model.
	r.Register(&ModelPricing{
		Provider:         "custom",
		Model:            "my-model-v1",
		Tier:             TierFast,
		InputPerMillion:  50_000,
		OutputPerMillion: 200_000,
	})

	p := r.GetPricing("custom", "my-model-v1")
	if p == nil {
		t.Fatal("expected custom model to be registered")
	}
	if p.Tier != TierFast {
		t.Errorf("expected TierFast, got %s", p.Tier)
	}

	cost := r.CalculateCost("custom", "my-model-v1", 1_000_000, 0)
	if cost != 50_000 {
		t.Errorf("expected 50_000, got %d", cost)
	}
}

func TestUnitConversion(t *testing.T) {
	// $1.00 = 100_000 millicents.
	u := FromDollars(1.0)
	if u != 100_000 {
		t.Errorf("expected 100_000, got %d", u)
	}

	dollars := u.ToDollars()
	if dollars != 1.0 {
		t.Errorf("expected 1.0, got %f", dollars)
	}

	// $0.001 = 100 millicents.
	u = FromDollars(0.001)
	if u != 100 {
		t.Errorf("expected 100, got %d", u)
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		milli    Unit
		expected string
	}{
		{FromDollars(1.50), "$1.50"},
		{FromDollars(0.01), "$0.01"},
		{FromDollars(100.0), "$100.00"},
	}

	for _, tt := range tests {
		result := FormatCost(tt.milli)
		if result != tt.expected {
			t.Errorf("FormatCost(%d) = %s, want %s", tt.milli, result, tt.expected)
		}
	}
}
