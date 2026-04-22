package cost

// PreviewEstimator generates cost previews before ticket authoring.
// Estimates are based on ticket scope (complexity, type, expected calls).
type PreviewEstimator struct {
	pricing *PricingRegistry
	router  *FallbackRouter
}

// NewPreviewEstimator creates a cost preview estimator.
func NewPreviewEstimator(pricing *PricingRegistry, router *FallbackRouter) *PreviewEstimator {
	return &PreviewEstimator{
		pricing: pricing,
		router:  router,
	}
}

// ScopeHint provides information about a ticket's expected scope.
type ScopeHint struct {
	Type       string // "task", "bug", "spike", "review"
	Complexity string // "low", "medium", "high"
	FilesCount int    // estimated files to touch
	HasTests   bool   // whether tests are expected
}

// EstimateCost generates a cost preview based on scope hints.
// The estimate breaks down coordinator, worker, and reviewer costs.
func (e *PreviewEstimator) EstimateCost(hint ScopeHint) *CostPreview {
	// Estimate number of LLM calls based on complexity.
	calls := e.estimateCalls(hint)

	// Estimate token usage per call by role.
	coordTokens := e.coordinatorTokens(hint)
	workerTokens := e.workerTokens(hint)
	reviewerTokens := e.reviewerTokens(hint)

	// Get model for each role's typical operation.
	coordRoute := e.router.Route(OpPlanning, nil)
	workerRoute := e.router.Route(OpCodeGeneration, nil)
	reviewerRoute := e.router.Route(OpReview, nil)

	// Calculate costs.
	coordCost := Unit(0)
	if coordRoute != nil && coordRoute.Provider != "" {
		coordCost = e.pricing.CalculateCost(
			coordRoute.Provider, coordRoute.Model,
			coordTokens.input, coordTokens.output,
		) * Unit(calls.coordinator)
	}

	workerCost := Unit(0)
	if workerRoute != nil && workerRoute.Provider != "" {
		workerCost = e.pricing.CalculateCost(
			workerRoute.Provider, workerRoute.Model,
			workerTokens.input, workerTokens.output,
		) * Unit(calls.worker)
	}

	reviewerCost := Unit(0)
	if reviewerRoute != nil && reviewerRoute.Provider != "" {
		reviewerCost = e.pricing.CalculateCost(
			reviewerRoute.Provider, reviewerRoute.Model,
			reviewerTokens.input, reviewerTokens.output,
		) * Unit(calls.reviewer)
	}

	total := coordCost + workerCost + reviewerCost

	return &CostPreview{
		EstimatedCalls:  calls.coordinator + calls.worker + calls.reviewer,
		CoordinatorCost: coordCost,
		WorkerCost:      workerCost,
		ReviewerCost:    reviewerCost,
		TotalEstimate:   total,
		TotalDollars:    total.ToDollars(),
		Confidence:      e.confidence(hint),
		Assumptions:     e.assumptions(hint),
	}
}

type callEstimate struct {
	coordinator int
	worker      int
	reviewer    int
}

type tokenEstimate struct {
	input  int64
	output int64
}

func (e *PreviewEstimator) estimateCalls(hint ScopeHint) callEstimate {
	// Base estimates by complexity.
	switch hint.Complexity {
	case "high":
		return callEstimate{coordinator: 3, worker: 15, reviewer: 3}
	case "medium":
		return callEstimate{coordinator: 2, worker: 8, reviewer: 2}
	default: // "low"
		return callEstimate{coordinator: 1, worker: 4, reviewer: 1}
	}
}

func (e *PreviewEstimator) coordinatorTokens(hint ScopeHint) tokenEstimate {
	switch hint.Complexity {
	case "high":
		return tokenEstimate{input: 8000, output: 2000}
	case "medium":
		return tokenEstimate{input: 5000, output: 1500}
	default:
		return tokenEstimate{input: 3000, output: 1000}
	}
}

func (e *PreviewEstimator) workerTokens(hint ScopeHint) tokenEstimate {
	base := tokenEstimate{input: 10000, output: 4000}
	switch hint.Complexity {
	case "high":
		base = tokenEstimate{input: 20000, output: 8000}
	case "medium":
		base = tokenEstimate{input: 12000, output: 5000}
	}
	if hint.HasTests {
		base.input += 3000
		base.output += 2000
	}
	return base
}

func (e *PreviewEstimator) reviewerTokens(hint ScopeHint) tokenEstimate {
	switch hint.Complexity {
	case "high":
		return tokenEstimate{input: 15000, output: 3000}
	case "medium":
		return tokenEstimate{input: 10000, output: 2000}
	default:
		return tokenEstimate{input: 6000, output: 1500}
	}
}

func (e *PreviewEstimator) confidence(hint ScopeHint) string {
	switch hint.Complexity {
	case "low":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func (e *PreviewEstimator) assumptions(hint ScopeHint) string {
	base := "Assumes standard token usage patterns. "
	switch hint.Type {
	case "bug":
		return base + "Bug fixes typically require investigation cycles; actual cost may vary."
	case "spike":
		return base + "Spikes are exploratory; cost is highly variable."
	case "review":
		return base + "Review-only tickets have lower worker cost."
	default:
		return base + "Task complexity determines estimated LLM interactions."
	}
}
