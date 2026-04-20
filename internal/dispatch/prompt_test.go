package dispatch

import (
	"strings"
	"testing"

	"github.com/matt0x6f/warrant/internal/project"
	"github.com/matt0x6f/warrant/internal/ticket"
)

func TestAssembleWorkerPromptMinimal(t *testing.T) {
	proj := &project.Project{
		ID:   "proj-1",
		Name: "test-project",
	}
	tk := &ticket.Ticket{
		ID:       "ticket-1",
		Title:    "Fix the bug",
		Type:     ticket.TypeBug,
		Priority: ticket.P1,
		Objective: ticket.Objective{
			Description: "Fix the login bug",
		},
	}

	result := AssembleWorkerPrompt(proj, tk, nil, "http://localhost:8080", "agent-1")

	// Should contain the role preamble.
	if !strings.Contains(result, "coding agent executing a warrant ticket") {
		t.Error("missing role preamble")
	}

	// Should contain ticket details.
	if !strings.Contains(result, "ticket-1") {
		t.Error("missing ticket ID")
	}
	if !strings.Contains(result, "Fix the bug") {
		t.Error("missing ticket title")
	}
	if !strings.Contains(result, "bug") {
		t.Error("missing ticket type")
	}
	if !strings.Contains(result, "P1") {
		t.Error("missing priority")
	}

	// Should contain the objective.
	if !strings.Contains(result, "Fix the login bug") {
		t.Error("missing objective description")
	}

	// Should contain workflow.
	if !strings.Contains(result, "claim_ticket") {
		t.Error("missing workflow instruction")
	}

	// Should contain server URL and agent ID.
	if !strings.Contains(result, "http://localhost:8080") {
		t.Error("missing server URL")
	}
	if !strings.Contains(result, "agent-1") {
		t.Error("missing agent ID")
	}

	// Should NOT contain optional sections when not set.
	if strings.Contains(result, "## Project system prompt") {
		t.Error("should not contain project system prompt section when empty")
	}
	if strings.Contains(result, "## Conventions") {
		t.Error("should not contain conventions section when empty")
	}
	if strings.Contains(result, "## Key files") {
		t.Error("should not contain key files section when empty")
	}
}

func TestAssembleWorkerPromptFull(t *testing.T) {
	proj := &project.Project{
		ID:   "proj-1",
		Name: "test-project",
		ContextPack: project.ContextPack{
			SystemPrompt: "You are a Go expert.",
			Conventions:  "Use gofmt. No global state.",
			KeyFiles: []project.FileRef{
				{Path: "main.go", Snippet: "entry point"},
				{Path: "config.go"},
			},
		},
	}
	tk := &ticket.Ticket{
		ID:       "ticket-42",
		Title:    "Add unit tests",
		Type:     ticket.TypeTask,
		Priority: ticket.P0,
		Objective: ticket.Objective{
			Description:     "Write tests for the dispatch package",
			SuccessCriteria: []string{"100% coverage", "No flaky tests"},
			AcceptanceTest:  "go test ./internal/dispatch/... -v",
		},
		Context: ticket.TicketContext{
			RelevantFiles: []string{"dispatch.go", "worker.go"},
			Constraints:   []string{"No external dependencies", "Must be fast"},
			PriorAttempts: []ticket.AttemptSummary{
				{AgentID: "agent-old", Outcome: "rejected", Summary: "Tests were incomplete"},
			},
			HumanAnswers: []string{"Focus on edge cases"},
		},
	}
	depOutputs := map[string]map[string]any{
		"dep-1": {"summary": "scaffolded the project", "files": 3},
	}

	result := AssembleWorkerPrompt(proj, tk, depOutputs, "http://localhost:9090", "agent-2")

	// Project context.
	if !strings.Contains(result, "You are a Go expert.") {
		t.Error("missing system prompt")
	}
	if !strings.Contains(result, "Use gofmt. No global state.") {
		t.Error("missing conventions")
	}
	if !strings.Contains(result, "`main.go`: entry point") {
		t.Error("missing key file with snippet")
	}
	if !strings.Contains(result, "`config.go`") {
		t.Error("missing key file without snippet")
	}

	// Ticket details.
	if !strings.Contains(result, "ticket-42") {
		t.Error("missing ticket ID")
	}
	if !strings.Contains(result, "P0") {
		t.Error("missing priority")
	}

	// Objective sections.
	if !strings.Contains(result, "Write tests for the dispatch package") {
		t.Error("missing objective")
	}
	if !strings.Contains(result, "100% coverage") {
		t.Error("missing success criteria")
	}
	if !strings.Contains(result, "go test ./internal/dispatch/... -v") {
		t.Error("missing acceptance test")
	}

	// Context sections.
	if !strings.Contains(result, "`dispatch.go`") {
		t.Error("missing relevant files")
	}
	if !strings.Contains(result, "No external dependencies") {
		t.Error("missing constraints")
	}

	// Prior attempts.
	if !strings.Contains(result, "Attempt 1") {
		t.Error("missing prior attempt number")
	}
	if !strings.Contains(result, "rejected") {
		t.Error("missing prior attempt outcome")
	}
	if !strings.Contains(result, "Tests were incomplete") {
		t.Error("missing prior attempt summary")
	}

	// Human answers.
	if !strings.Contains(result, "Focus on edge cases") {
		t.Error("missing human answer")
	}

	// Dependency outputs.
	if !strings.Contains(result, "dep-1") {
		t.Error("missing dependency ID")
	}
	if !strings.Contains(result, "scaffolded the project") {
		t.Error("missing dependency output value")
	}

	// Server info.
	if !strings.Contains(result, "http://localhost:9090") {
		t.Error("missing server URL")
	}
	if !strings.Contains(result, "agent-2") {
		t.Error("missing agent ID")
	}
}

func TestAssembleWorkerPromptSectionOrdering(t *testing.T) {
	proj := &project.Project{
		ID: "proj-1",
		ContextPack: project.ContextPack{
			SystemPrompt: "SYSTEM_PROMPT_MARKER",
			Conventions:  "CONVENTIONS_MARKER",
		},
	}
	tk := &ticket.Ticket{
		ID:    "t-1",
		Title: "Test ordering",
		Type:  ticket.TypeTask,
		Objective: ticket.Objective{
			Description: "OBJECTIVE_MARKER",
		},
	}

	result := AssembleWorkerPrompt(proj, tk, nil, "http://localhost", "agent")

	// Verify ordering: system prompt before conventions, conventions before ticket.
	sysIdx := strings.Index(result, "SYSTEM_PROMPT_MARKER")
	convIdx := strings.Index(result, "CONVENTIONS_MARKER")
	objIdx := strings.Index(result, "OBJECTIVE_MARKER")
	workflowIdx := strings.Index(result, "## Workflow")

	if sysIdx == -1 || convIdx == -1 || objIdx == -1 || workflowIdx == -1 {
		t.Fatal("missing expected markers in prompt")
	}

	if sysIdx >= convIdx {
		t.Error("system prompt should come before conventions")
	}
	if convIdx >= objIdx {
		t.Error("conventions should come before objective")
	}
	if objIdx >= workflowIdx {
		t.Error("objective should come before workflow")
	}
}

func TestAssembleWorkerPromptNilDepOutputs(t *testing.T) {
	proj := &project.Project{ID: "p"}
	tk := &ticket.Ticket{
		ID:    "t",
		Title: "t",
		Type:  ticket.TypeTask,
		Objective: ticket.Objective{
			Description: "do something",
		},
	}

	result := AssembleWorkerPrompt(proj, tk, nil, "http://localhost", "a")
	if strings.Contains(result, "Dependency outputs") {
		t.Error("should not contain dependency outputs section when nil")
	}
}

func TestAssembleWorkerPromptEmptyDepOutputs(t *testing.T) {
	proj := &project.Project{ID: "p"}
	tk := &ticket.Ticket{
		ID:    "t",
		Title: "t",
		Type:  ticket.TypeTask,
		Objective: ticket.Objective{
			Description: "do something",
		},
	}

	result := AssembleWorkerPrompt(proj, tk, map[string]map[string]any{}, "http://localhost", "a")
	if strings.Contains(result, "Dependency outputs") {
		t.Error("should not contain dependency outputs section when empty")
	}
}

func TestAssembleWorkerPromptMultiplePriorAttempts(t *testing.T) {
	proj := &project.Project{ID: "p"}
	tk := &ticket.Ticket{
		ID:    "t",
		Title: "t",
		Type:  ticket.TypeTask,
		Objective: ticket.Objective{
			Description: "d",
		},
		Context: ticket.TicketContext{
			PriorAttempts: []ticket.AttemptSummary{
				{Outcome: "rejected", Summary: "first try"},
				{Outcome: "failed", Summary: "second try"},
			},
		},
	}

	result := AssembleWorkerPrompt(proj, tk, nil, "http://localhost", "a")
	if !strings.Contains(result, "Attempt 1") {
		t.Error("missing attempt 1")
	}
	if !strings.Contains(result, "Attempt 2") {
		t.Error("missing attempt 2")
	}
	if !strings.Contains(result, "first try") {
		t.Error("missing first attempt summary")
	}
	if !strings.Contains(result, "second try") {
		t.Error("missing second attempt summary")
	}
}
