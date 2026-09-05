package workflow

import (
	"context"
	"testing"
)

func TestReviewFailedPhaseMustNotAdvance(t *testing.T) {
	def := &Definition{ID: "wf", Phases: []Phase{{ID: "deploy", Type: PhaseExternal}, {ID: "done", Type: PhaseGate}}}
	engine, updater := newTestEngine(def)
	updater.phases["t"] = "deploy"
	next, err := engine.AdvancePhase(context.Background(), "t", "wf", "deploy", "failed", nil, 0)
	if err == nil && next != nil && next.ID == "done" {
		t.Fatal("failed deployment advanced to done with no error")
	}
}

func TestReviewPhaseHarnessOverrideSupported(t *testing.T) {
	errs := ValidatePhaseConfig(PhaseAgent, map[string]any{"role": "validator", "harness": "codex", "model": "example", "effort": "high"})
	if len(errs) > 0 {
		t.Fatalf("documented per-phase harness settings rejected: %v", errs)
	}
}
