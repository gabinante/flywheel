package settings

import (
	"testing"

	"github.com/gabinante/flywheel/internal/project"
)

func TestExplicitWorkerAssignment(t *testing.T) {
	s := Settings{Workers: project.DispatchConfig{Workers: []project.DispatchWorkerProfile{
		{ID: "legacy", Enabled: true, Driver: "claude", Model: "wrong"},
		{ID: "reviewer", Enabled: true, Driver: "codex", SystemPrompt: "Be pragmatic"},
	}}, Review: ReviewSettings{WorkerID: "reviewer", RoleID: "validator", Harness: "claude", Model: "old-model"}, Feedback: FeedbackSettings{Harness: "claude"}, Harness: HarnessSettings{Codex: HarnessDefaults{Model: "codex-default"}}, Report: ReportSettings{DefaultHealth: "onTrack"}, Dispatch: DispatchSettings{Driver: "claude"}}
	if err := validate(s); err != nil {
		t.Fatalf("valid assignment rejected: %v", err)
	}
	r, _ := s.ReviewConfig()
	if r.Harness != "codex" || r.Model != "codex-default" || r.PromptPrefix != "Be pragmatic" {
		t.Fatalf("wrong worker config: %+v", r)
	}
	if _, ok := s.resolveAssignment("missing", "validator"); ok {
		t.Fatal("explicit missing worker fell back to legacy routing")
	}
	s.Review.WorkerID = "missing"
	if validate(s) == nil {
		t.Fatal("missing worker accepted")
	}
	s.Review.WorkerID = "reviewer"
	s.Workers.Workers[1].Enabled = false
	if validate(s) == nil {
		t.Fatal("disabled worker accepted")
	}
}

func TestDefaultRoleDoesNotImplicitlySelectNewWorker(t *testing.T) {
	s := Settings{Workers: project.DispatchConfig{Workers: []project.DispatchWorkerProfile{{ID: "new-worker", Enabled: true, Driver: "codex"}}, Policies: map[string]project.DispatchRolePolicy{"validator": {WorkerIDs: []string{"default"}}}}}
	if _, ok := s.ResolveRole("validator"); ok {
		t.Fatal("default task was reassigned to the new worker")
	}
}
