package gate

import (
	"context"
	"time"
)

// RequirementChecker checks whether a single gate requirement type is satisfied.
type RequirementChecker interface {
	Check(ctx context.Context, req GateRequirement, rctx CheckContext) GateRequirementStatus
}

// CheckContext provides the data needed by requirement checkers.
type CheckContext struct {
	TicketID  string
	ProjectID string
	PRURL     string         // from ticket outputs["pr_url"]
	PhaseID   string         // current workflow phase ID
	Outputs   map[string]any // ticket outputs map
}

// CheckerRegistry routes requirement checks to the appropriate checker.
type CheckerRegistry struct {
	checkers map[GateRequirementType]RequirementChecker
}

// NewCheckerRegistry creates a new empty registry.
func NewCheckerRegistry() *CheckerRegistry {
	return &CheckerRegistry{
		checkers: make(map[GateRequirementType]RequirementChecker),
	}
}

// Register adds a checker for the given requirement type.
func (r *CheckerRegistry) Register(t GateRequirementType, c RequirementChecker) {
	r.checkers[t] = c
}

// CheckAll evaluates all requirements and returns their statuses.
// Returns nil if there are no requirements.
func (r *CheckerRegistry) CheckAll(ctx context.Context, requirements []GateRequirement, rctx CheckContext) []GateRequirementStatus {
	if len(requirements) == 0 {
		return nil
	}
	statuses := make([]GateRequirementStatus, 0, len(requirements))
	for _, req := range requirements {
		checker, ok := r.checkers[req.Type]
		if !ok {
			statuses = append(statuses, GateRequirementStatus{
				Requirement: req,
				Satisfied:   false,
				Reason:      "no checker registered for requirement type",
				CheckedAt:   time.Now().UTC(),
			})
			continue
		}
		statuses = append(statuses, checker.Check(ctx, req, rctx))
	}
	return statuses
}

// Unsatisfied returns only the unsatisfied statuses from a list.
func Unsatisfied(statuses []GateRequirementStatus) []GateRequirementStatus {
	var result []GateRequirementStatus
	for _, s := range statuses {
		if !s.Satisfied {
			result = append(result, s)
		}
	}
	return result
}
