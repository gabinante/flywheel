package cost

import "errors"

// Sentinel errors for cost management.
// These are used by the errors package for structured error mapping.
var (
	// ErrBudgetExceeded indicates spend exceeds the configured budget limit.
	// Per constraint: this is only returned when HardStop is explicitly enabled.
	ErrBudgetExceeded = errors.New("budget limit exceeded")

	// ErrRateLimited indicates the LLM provider is rate-limiting requests.
	// The system backs off rather than failing; this error surfaces the delay.
	ErrRateLimited = errors.New("rate limited — backing off")

	// ErrInvalidBudget indicates an invalid budget configuration (e.g. negative limit).
	ErrInvalidBudget = errors.New("invalid budget configuration")

	// ErrNoBudgetFound indicates no budget is configured for the given scope.
	ErrNoBudgetFound = errors.New("no budget found for scope")
)
