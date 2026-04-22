package cost

import (
	"context"
	"log"
	"time"
)

// Service is the top-level cost management service that coordinates tracking,
// budgets, rate limits, and model routing. It is the main integration point
// for the rest of the system.
type Service struct {
	Tracker   *Tracker
	Budget    *BudgetChecker
	RateLimit *RateLimitHandler
	Router    *FallbackRouter
	Pricing   *PricingRegistry
	Preview   *PreviewEstimator
}

// Config holds cost management configuration.
type Config struct {
	// Budget defaults
	DefaultMonthlyBudgetDollars float64 // 0 = no default budget
	DefaultTicketBudgetDollars  float64 // 0 = no default budget
	WarnAtFraction              float64 // default 0.8

	// Model configuration
	FlagshipProvider string
	FlagshipModel    string
	MidProvider      string
	MidModel         string
	FastProvider     string
	FastModel        string

	// Fallback policies (nil = use defaults)
	Policies map[OperationType]*FallbackPolicy
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		WarnAtFraction:   0.8,
		FlagshipProvider: "anthropic",
		FlagshipModel:    "claude-opus-4-20250514",
		MidProvider:      "anthropic",
		MidModel:         "claude-sonnet-4-20250514",
		FastProvider:     "anthropic",
		FastModel:        "claude-haiku-3-20250307",
	}
}

// NewService creates a fully-wired cost management service.
func NewService(store Store, cfg Config, budgetNotify AlertNotifier, rlNotify RateLimitNotifier) *Service {
	pricing := NewPricingRegistry()
	router := NewFallbackRouter()

	// Configure model tiers.
	if cfg.FlagshipProvider != "" && cfg.FlagshipModel != "" {
		router.SetModel(TierFlagship, cfg.FlagshipProvider, cfg.FlagshipModel)
	}
	if cfg.MidProvider != "" && cfg.MidModel != "" {
		router.SetModel(TierMid, cfg.MidProvider, cfg.MidModel)
	}
	if cfg.FastProvider != "" && cfg.FastModel != "" {
		router.SetModel(TierFast, cfg.FastProvider, cfg.FastModel)
	}

	// Apply custom policies if provided.
	if cfg.Policies != nil {
		for _, p := range cfg.Policies {
			router.SetPolicy(p)
		}
	}

	tracker := NewTracker(store, pricing)
	budget := NewBudgetChecker(store, tracker, budgetNotify)
	rateLimit := NewRateLimitHandler(store, rlNotify)
	preview := NewPreviewEstimator(pricing, router)

	return &Service{
		Tracker:   tracker,
		Budget:    budget,
		RateLimit: rateLimit,
		Router:    router,
		Pricing:   pricing,
		Preview:   preview,
	}
}

// RecordAndCheck records an LLM call and checks budget status.
// Returns (shouldContinue, budgetStatus). Per constraint: warns but does not
// hard-stop unless HardStop is explicitly set on the budget.
func (s *Service) RecordAndCheck(ctx context.Context, call *LLMCallRecord) (bool, *BudgetStatus) {
	if err := s.Tracker.RecordCall(ctx, call); err != nil {
		log.Printf("cost: failed to record call: %v", err)
		return true, nil // fail open
	}

	status, shouldContinue := s.Budget.CheckBudget(ctx, call.ProjectID, call.TicketID)
	return shouldContinue, status
}

// RouteModel determines which model to use for an operation, considering
// rate limits and fallback policies.
func (s *Service) RouteModel(op OperationType) *RouteResult {
	return s.Router.Route(op, func(provider, model string) bool {
		limited, _ := s.RateLimit.IsRateLimited(provider, model)
		return limited
	})
}

// HandleRateLimit processes a rate limit error from a provider.
// Returns the backoff duration. The caller should wait this duration before retrying.
func (s *Service) HandleRateLimit(ctx context.Context, projectID, ticketID, provider, model string, retryAfter time.Duration) (time.Duration, error) {
	return s.RateLimit.HandleRateLimit(ctx, projectID, ticketID, provider, model, retryAfter)
}

// PreviewTicketCost estimates the cost of executing a ticket.
func (s *Service) PreviewTicketCost(hint ScopeHint) *CostPreview {
	return s.Preview.EstimateCost(hint)
}

// GetProjectStatus returns the current budget/cost status for a project.
func (s *Service) GetProjectStatus(ctx context.Context, projectID string) (*BudgetStatus, error) {
	return s.Budget.GetBudgetStatus(ctx, projectID)
}

// SetProjectBudget sets the monthly budget for a project.
func (s *Service) SetProjectBudget(ctx context.Context, projectID string, limitDollars float64) error {
	budget := &Budget{
		ProjectID:  projectID,
		Month:      CurrentMonth(),
		LimitMilli: FromDollars(limitDollars),
		WarnAt:     0.8,
		HardStop:   false, // per constraint: warn, don't hard-stop
	}
	return s.Budget.SetBudget(ctx, budget)
}

// SetTicketBudget sets a per-ticket budget.
func (s *Service) SetTicketBudget(ctx context.Context, projectID, ticketID string, limitDollars float64) error {
	budget := &Budget{
		ProjectID:  projectID,
		TicketID:   ticketID,
		LimitMilli: FromDollars(limitDollars),
		WarnAt:     0.8,
		HardStop:   false,
	}
	return s.Budget.SetBudget(ctx, budget)
}
