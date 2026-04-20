package cost

import "sync"

// PricingRegistry holds provider-agnostic pricing data for models.
// All prices are normalized to millicents per million tokens.
type PricingRegistry struct {
	mu     sync.RWMutex
	models map[string]*ModelPricing // key: "provider:model"
}

// NewPricingRegistry creates a registry with default pricing for common models.
func NewPricingRegistry() *PricingRegistry {
	r := &PricingRegistry{
		models: make(map[string]*ModelPricing),
	}
	r.loadDefaults()
	return r
}

// Register adds or updates pricing for a model.
func (r *PricingRegistry) Register(p *ModelPricing) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := p.Provider + ":" + p.Model
	r.models[key] = p
}

// CalculateCost returns the cost in millicents for a given call.
func (r *PricingRegistry) CalculateCost(provider, model string, inputTokens, outputTokens int64) Unit {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := provider + ":" + model
	p, ok := r.models[key]
	if !ok {
		// Unknown model — use mid-tier estimate.
		return estimateUnknownCost(inputTokens, outputTokens)
	}

	inputCost := Unit(inputTokens) * p.InputPerMillion / 1_000_000
	outputCost := Unit(outputTokens) * p.OutputPerMillion / 1_000_000
	return inputCost + outputCost
}

// GetTier returns the tier for a provider/model combination.
func (r *PricingRegistry) GetTier(provider, model string) ModelTier {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := provider + ":" + model
	p, ok := r.models[key]
	if !ok {
		return TierMid // default assumption
	}
	return p.Tier
}

// GetPricing returns the pricing for a specific model.
func (r *PricingRegistry) GetPricing(provider, model string) *ModelPricing {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := provider + ":" + model
	p := r.models[key]
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// ListModels returns all registered model pricing.
func (r *PricingRegistry) ListModels() []*ModelPricing {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelPricing, 0, len(r.models))
	for _, p := range r.models {
		cp := *p
		result = append(result, &cp)
	}
	return result
}

// loadDefaults populates commonly-used model pricing.
// Prices as of 2026-04. Input/output per million tokens in millicents.
func (r *PricingRegistry) loadDefaults() {
	defaults := []*ModelPricing{
		// Anthropic models
		{Provider: "anthropic", Model: "claude-opus-4-20250514", Tier: TierFlagship,
			InputPerMillion: 1_500_000, OutputPerMillion: 7_500_000}, // $15/$75
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Tier: TierMid,
			InputPerMillion: 300_000, OutputPerMillion: 1_500_000}, // $3/$15
		{Provider: "anthropic", Model: "claude-haiku-3-20250307", Tier: TierFast,
			InputPerMillion: 80_000, OutputPerMillion: 400_000}, // $0.80/$4
		// OpenAI models
		{Provider: "openai", Model: "gpt-4o", Tier: TierMid,
			InputPerMillion: 250_000, OutputPerMillion: 1_000_000}, // $2.50/$10
		{Provider: "openai", Model: "gpt-4o-mini", Tier: TierFast,
			InputPerMillion: 15_000, OutputPerMillion: 60_000}, // $0.15/$0.60
		{Provider: "openai", Model: "o1", Tier: TierFlagship,
			InputPerMillion: 1_500_000, OutputPerMillion: 6_000_000}, // $15/$60
		// Google models
		{Provider: "google", Model: "gemini-2.5-pro", Tier: TierMid,
			InputPerMillion: 125_000, OutputPerMillion: 1_000_000}, // $1.25/$10
		{Provider: "google", Model: "gemini-2.5-flash", Tier: TierFast,
			InputPerMillion: 15_000, OutputPerMillion: 60_000}, // $0.15/$0.60
	}

	for _, p := range defaults {
		key := p.Provider + ":" + p.Model
		r.models[key] = p
	}
}

// estimateUnknownCost provides a conservative estimate for unknown models.
// Uses mid-tier pricing as a baseline.
func estimateUnknownCost(inputTokens, outputTokens int64) Unit {
	// Assume ~$3/M input, $15/M output (mid-tier estimate).
	inputCost := Unit(inputTokens) * 300_000 / 1_000_000
	outputCost := Unit(outputTokens) * 1_500_000 / 1_000_000
	return inputCost + outputCost
}
