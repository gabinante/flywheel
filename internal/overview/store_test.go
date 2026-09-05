package overview

import (
	"github.com/gabinante/flywheel/internal/workflow"
	"testing"
	"time"
)

func TestAttentionRequiresAnOperatorDecision(t *testing.T) {
	now := time.Now().UTC()
	human := &workflow.Phase{ID: "gate", Name: "Approval", Type: workflow.PhaseGate, Config: map[string]any{"conditions": []any{map[string]any{"type": "human_approval"}}}}
	auto := &workflow.Phase{ID: "gate", Name: "CI", Type: workflow.PhaseGate, Config: map[string]any{"conditions": []any{map[string]any{"type": "webhook"}}}}
	manual := &workflow.Phase{ID: "gate", Name: "Implementation", Type: workflow.PhaseAgent, Config: map[string]any{"auto_advance": false}}
	for _, tc := range []struct {
		name, state, status string
		phase               *workflow.Phase
		outputs             map[string]any
		working             bool
		want                string
	}{
		{name: "question", state: "awaiting_input", want: "Provide input"},
		{name: "failure", state: "executing", status: "failed", want: "Retry phase"},
		{name: "human gate", state: "awaiting_validation", status: "blocked", phase: human, want: "Approve phase"},
		{name: "approved current attempt", state: "awaiting_validation", status: "blocked", phase: human, outputs: map[string]any{"_human_approval_gate": now.Format(time.RFC3339Nano)}},
		{name: "old approval", state: "awaiting_validation", status: "blocked", phase: human, outputs: map[string]any{"_human_approval_gate": now.Add(-time.Hour).Format(time.RFC3339Nano)}, want: "Approve phase"},
		{name: "automated wait", state: "awaiting_validation", status: "blocked", phase: auto},
		{name: "manual continuation", state: "awaiting_validation", status: "blocked", phase: manual, want: "Continue workflow"},
		{name: "still working", state: "awaiting_validation", status: "blocked", phase: manual, working: true},
		{name: "closed failed ticket", state: "closed", status: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := attentionReason(candidate{state: tc.state, workflowID: "workflow", phaseID: "gate", phaseStatus: tc.status, enteredAt: &now, phase: tc.phase, outputs: tc.outputs}, tc.working)
			if got != tc.want {
				t.Fatalf("action=%q want %q", got, tc.want)
			}
		})
	}
}
