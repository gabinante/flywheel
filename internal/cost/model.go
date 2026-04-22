// Package cost implements provider-agnostic cost tracking, budget management,
// rate-limit handling, and cheaper-model fallback policies for LLM operations.
//
// All costs are normalized to a common unit ("millicents" — 1/1000 of a cent USD)
// to enable provider-agnostic budget tracking. The system warns but does not
// hard-block when budgets are exceeded (per project constraint).
package cost

import "time"

// Unit is the normalized cost unit: millicents (1/1000 of a cent USD).
// This allows sub-cent precision for cheap model calls while keeping integer math.
type Unit int64

const (
	// MillicentPerCent is 1000 millicents per cent.
	MillicentPerCent Unit = 1000
	// MillicentPerDollar is 100_000 millicents per dollar.
	MillicentPerDollar Unit = 100_000
)

// ToDollars converts millicents to dollars (float, for display only).
func (u Unit) ToDollars() float64 {
	return float64(u) / float64(MillicentPerDollar)
}

// FromDollars converts dollars to millicents.
func FromDollars(d float64) Unit {
	return Unit(d * float64(MillicentPerDollar))
}

// ModelTier classifies models by cost/capability tier.
type ModelTier string

const (
	TierFlagship ModelTier = "flagship" // e.g. Claude Opus, GPT-4o
	TierMid      ModelTier = "mid"      // e.g. Claude Sonnet
	TierFast     ModelTier = "fast"     // e.g. Claude Haiku, GPT-4o-mini
)

// OperationType classifies the purpose of an LLM call for fallback routing.
type OperationType string

const (
	OpGeneral         OperationType = "general"          // default: use configured model
	OpClassifier      OperationType = "classifier"       // classification/advisory
	OpSummarization   OperationType = "summarization"    // document summarization
	OpStructuralQuery OperationType = "structural_query" // code structure queries
	OpCodeGeneration  OperationType = "code_generation"  // actual code writing
	OpReview          OperationType = "review"           // code review
	OpPlanning        OperationType = "planning"         // architecture/planning
)

// LLMCallRecord is a single attributed LLM call for cost tracking.
type LLMCallRecord struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	TicketID      string        `json:"ticket_id"`
	WorkerRole    string        `json:"worker_role"`    // e.g. "executor", "reviewer", "coordinator"
	Provider      string        `json:"provider"`       // e.g. "anthropic", "openai"
	Model         string        `json:"model"`          // e.g. "claude-sonnet-4-20250514"
	ModelTier     ModelTier     `json:"model_tier"`
	OperationType OperationType `json:"operation_type"`
	InputTokens   int64         `json:"input_tokens"`
	OutputTokens  int64         `json:"output_tokens"`
	CostMillicent Unit          `json:"cost_millicent"` // normalized cost
	Timestamp     time.Time     `json:"timestamp"`
}

// Budget defines spending limits for a scope (project, ticket, or monthly).
type Budget struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	TicketID   string    `json:"ticket_id,omitempty"`   // empty = project-level budget
	Month      string    `json:"month,omitempty"`       // "2026-04" format; empty = no time scope
	LimitMilli Unit      `json:"limit_millicent"`       // budget limit in millicents
	WarnAt     float64   `json:"warn_at"`               // fraction (0.0-1.0) at which to warn (default 0.8)
	HardStop   bool      `json:"hard_stop"`             // if true, block at limit (default: false, warn only)
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BudgetStatus is the current spend vs limit for a budget scope.
type BudgetStatus struct {
	Budget       Budget  `json:"budget"`
	SpentMilli   Unit    `json:"spent_millicent"`
	Remaining    Unit    `json:"remaining_millicent"`
	UsedFraction float64 `json:"used_fraction"` // 0.0 to 1.0+
	Projected    Unit    `json:"projected_millicent"`   // projected month-end spend
	OverBudget   bool    `json:"over_budget"`
	Warning      bool    `json:"warning"`       // true if past warn_at threshold
}

// BudgetAlert is pushed to operators when projection exceeds budget.
type BudgetAlert struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	TicketID     string       `json:"ticket_id,omitempty"`
	AlertType    AlertType    `json:"alert_type"`
	Status       BudgetStatus `json:"status"`
	Message      string       `json:"message"`
	CreatedAt    time.Time    `json:"created_at"`
	Acknowledged bool         `json:"acknowledged"`
}

// AlertType classifies budget alerts.
type AlertType string

const (
	AlertWarning    AlertType = "warning"    // approaching limit
	AlertProjection AlertType = "projection" // projection exceeds budget
	AlertExceeded   AlertType = "exceeded"   // actual spend exceeds budget
)

// CostPreview estimates the cost of a ticket before execution.
type CostPreview struct {
	TicketID        string  `json:"ticket_id,omitempty"`
	EstimatedCalls  int     `json:"estimated_calls"`
	CoordinatorCost Unit    `json:"coordinator_cost_millicent"`
	WorkerCost      Unit    `json:"worker_cost_millicent"`
	ReviewerCost    Unit    `json:"reviewer_cost_millicent"`
	TotalEstimate   Unit    `json:"total_estimate_millicent"`
	TotalDollars    float64 `json:"total_dollars"`
	Confidence      string  `json:"confidence"` // "low", "medium", "high"
	Assumptions     string  `json:"assumptions,omitempty"`
}

// RateLimitEvent records a rate limit hit and backoff.
type RateLimitEvent struct {
	ID         string        `json:"id"`
	ProjectID  string        `json:"project_id"`
	TicketID   string        `json:"ticket_id"`
	Provider   string        `json:"provider"`
	Model      string        `json:"model"`
	RetryAfter time.Duration `json:"retry_after"`
	ResetAt    time.Time     `json:"reset_at"`
	Timestamp  time.Time     `json:"timestamp"`
	Resumed    bool          `json:"resumed"`
	ResumedAt  time.Time     `json:"resumed_at,omitempty"`
}

// FallbackPolicy defines when an operation can use a cheaper model.
type FallbackPolicy struct {
	OperationType OperationType `json:"operation_type"`
	PreferredTier ModelTier     `json:"preferred_tier"` // tier to use for this operation
	AllowFallback bool          `json:"allow_fallback"` // can fall back to cheaper tier
	MaxTier       ModelTier     `json:"max_tier"`       // most expensive tier allowed
}

// ModelPricing defines the cost per token for a model (provider-agnostic).
type ModelPricing struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Tier             ModelTier `json:"tier"`
	InputPerMillion  Unit      `json:"input_per_million_millicent"`  // cost per 1M input tokens
	OutputPerMillion Unit      `json:"output_per_million_millicent"` // cost per 1M output tokens
}

// CostSummary aggregates costs for reporting.
type CostSummary struct {
	ProjectID    string          `json:"project_id"`
	Month        string          `json:"month,omitempty"`
	TotalMilli   Unit            `json:"total_millicent"`
	TotalDollars float64         `json:"total_dollars"`
	ByTicket     map[string]Unit `json:"by_ticket"`
	ByRole       map[string]Unit `json:"by_role"`
	ByModel      map[string]Unit `json:"by_model"`
	ByProvider   map[string]Unit `json:"by_provider"`
	CallCount    int             `json:"call_count"`
}
