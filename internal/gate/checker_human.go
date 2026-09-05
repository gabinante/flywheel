package gate

import (
	"context"
	"time"
)

// ReviewStatusGetter checks whether a ticket has received an approved review.
// Implemented by review.Service or a bridge adapter.
type ReviewStatusGetter interface {
	HasApprovedReview(ctx context.Context, ticketID string) (bool, error)
}

// HumanApprovalChecker verifies that a ticket has received an approved review.
type HumanApprovalChecker struct {
	Reviews ReviewStatusGetter
}

// Check implements RequirementChecker.
func (c *HumanApprovalChecker) Check(ctx context.Context, req GateRequirement, rctx CheckContext) GateRequirementStatus {
	now := time.Now().UTC()

	if approved, ok := rctx.Outputs["_human_approval_"+rctx.PhaseID].(string); ok && approved != "" && approved == rctx.PhaseEnteredAt {
		return GateRequirementStatus{Requirement: req, Satisfied: true, Reason: "operator approved this phase attempt", CheckedAt: now}
	}
	if c.Reviews == nil {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "no review service configured",
			CheckedAt:   now,
		}
	}

	approved, err := c.Reviews.HasApprovedReview(ctx, rctx.TicketID)
	if err != nil {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "error checking review status: " + err.Error(),
			CheckedAt:   now,
		}
	}

	if approved {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   true,
			Reason:      "ticket has an approved review",
			CheckedAt:   now,
		}
	}

	return GateRequirementStatus{
		Requirement: req,
		Satisfied:   false,
		Reason:      "no approved review found",
		CheckedAt:   now,
	}
}
