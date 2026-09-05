package gate

import (
	"context"
	"fmt"
	"time"
)

// WebhookChecker checks whether an external system has posted to the gate callback URL.
// It looks for a truthy value at outputs["_gate_webhook_<phaseID>"] on the ticket.
type WebhookChecker struct{}

// Check implements RequirementChecker.
func (c *WebhookChecker) Check(_ context.Context, req GateRequirement, rctx CheckContext) GateRequirementStatus {
	now := time.Now().UTC()

	if rctx.PhaseID == "" {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "no phase ID available for webhook check",
			CheckedAt:   now,
		}
	}

	key := fmt.Sprintf("_gate_webhook_%s", rctx.PhaseID)
	if v, ok := rctx.Outputs[key].(string); ok && rctx.PhaseEnteredAt != "" && v == rctx.PhaseEnteredAt {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   true,
			Reason:      "webhook callback received",
			CheckedAt:   now,
		}
	}

	return GateRequirementStatus{
		Requirement: req,
		Satisfied:   false,
		Reason:      "waiting for webhook callback",
		CheckedAt:   now,
	}
}
