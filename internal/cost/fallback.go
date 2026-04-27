package cost

import (
	"log/slog"
	"sync"
)

// FallbackRouter determines which model tier to use for a given operation.
// Certain operations (classifier advisory, doc summarization, structural queries)
// can route to cheaper Haiku-class models without quality loss.
type FallbackRouter struct {
	mu       sync.RWMutex
	policies map[OperationType]*FallbackPolicy
	models   map[ModelTier]ModelSelection // configured model per tier
}

// ModelSelection maps a tier to a specific provider/model.
type ModelSelection struct {
	Provider string
	Model    string
}

// DefaultFallbackPolicies returns the default policies for each operation type.
// These route cheap operations to fast-tier models and expensive operations to flagship.
func DefaultFallbackPolicies() map[OperationType]*FallbackPolicy {
	return map[OperationType]*FallbackPolicy{
		OpClassifier: {
			OperationType: OpClassifier,
			PreferredTier: TierFast,
			AllowFallback: true,
			MaxTier:       TierMid,
		},
		OpSummarization: {
			OperationType: OpSummarization,
			PreferredTier: TierFast,
			AllowFallback: true,
			MaxTier:       TierMid,
		},
		OpStructuralQuery: {
			OperationType: OpStructuralQuery,
			PreferredTier: TierFast,
			AllowFallback: true,
			MaxTier:       TierMid,
		},
		OpCodeGeneration: {
			OperationType: OpCodeGeneration,
			PreferredTier: TierFlagship,
			AllowFallback: false,
			MaxTier:       TierFlagship,
		},
		OpReview: {
			OperationType: OpReview,
			PreferredTier: TierMid,
			AllowFallback: true,
			MaxTier:       TierFlagship,
		},
		OpPlanning: {
			OperationType: OpPlanning,
			PreferredTier: TierFlagship,
			AllowFallback: false,
			MaxTier:       TierFlagship,
		},
		OpGeneral: {
			OperationType: OpGeneral,
			PreferredTier: TierMid,
			AllowFallback: true,
			MaxTier:       TierFlagship,
		},
	}
}

// NewFallbackRouter creates a router with default policies.
func NewFallbackRouter() *FallbackRouter {
	return &FallbackRouter{
		policies: DefaultFallbackPolicies(),
		models:   make(map[ModelTier]ModelSelection),
	}
}

// SetPolicy sets or overrides the fallback policy for an operation type.
func (r *FallbackRouter) SetPolicy(policy *FallbackPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies[policy.OperationType] = policy
}

// SetModel configures the model to use for a given tier.
func (r *FallbackRouter) SetModel(tier ModelTier, provider, model string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[tier] = ModelSelection{Provider: provider, Model: model}
}

// GetPolicies returns a copy of all current policies.
func (r *FallbackRouter) GetPolicies() map[OperationType]*FallbackPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[OperationType]*FallbackPolicy, len(r.policies))
	for k, v := range r.policies {
		cp := *v
		result[k] = &cp
	}
	return result
}

// RouteResult contains the routing decision for an operation.
type RouteResult struct {
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Tier     ModelTier `json:"tier"`
	Fallback bool      `json:"fallback"` // true if using a cheaper model than preferred
	Reason   string    `json:"reason"`
}

// Route determines which model to use for the given operation.
// If the preferred tier model is rate-limited or unavailable, it falls back
// to the next cheapest tier (if policy allows).
func (r *FallbackRouter) Route(op OperationType, rateLimited func(provider, model string) bool) *RouteResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policy, ok := r.policies[op]
	if !ok {
		policy = r.policies[OpGeneral]
	}

	// Try preferred tier first.
	preferredModel, hasPreferred := r.models[policy.PreferredTier]
	if hasPreferred {
		if rateLimited == nil || !rateLimited(preferredModel.Provider, preferredModel.Model) {
			return &RouteResult{
				Provider: preferredModel.Provider,
				Model:    preferredModel.Model,
				Tier:     policy.PreferredTier,
				Fallback: false,
				Reason:   "preferred model available",
			}
		}
	}

	// If preferred is rate-limited or unavailable and fallback is allowed, try cheaper tiers.
	if policy.AllowFallback {
		tiers := []ModelTier{TierFast, TierMid, TierFlagship}
		for _, tier := range tiers {
			if tier == policy.PreferredTier {
				continue
			}
			model, hasModel := r.models[tier]
			if !hasModel {
				continue
			}
			if rateLimited != nil && rateLimited(model.Provider, model.Model) {
				continue
			}
			isCheaper := tierCost(tier) <= tierCost(policy.PreferredTier)
			reason := "fallback: preferred model unavailable"
			if isCheaper {
				reason = "fallback: using cheaper model"
			}
			slog.Info("routing to fallback model", "operation", string(op), "provider", model.Provider, "model", model.Model, "tier", string(tier), "reason", reason)
			return &RouteResult{
				Provider: model.Provider,
				Model:    model.Model,
				Tier:     tier,
				Fallback: true,
				Reason:   reason,
			}
		}
	}

	// No fallback available — use preferred even if rate-limited (caller will handle backoff).
	if hasPreferred {
		return &RouteResult{
			Provider: preferredModel.Provider,
			Model:    preferredModel.Model,
			Tier:     policy.PreferredTier,
			Fallback: false,
			Reason:   "no fallback available, using preferred (may be rate-limited)",
		}
	}

	// No models configured at all.
	return &RouteResult{
		Reason: "no models configured",
	}
}

// tierCost returns a relative cost ordering for tiers (lower = cheaper).
func tierCost(tier ModelTier) int {
	switch tier {
	case TierFast:
		return 1
	case TierMid:
		return 2
	case TierFlagship:
		return 3
	default:
		return 2
	}
}
