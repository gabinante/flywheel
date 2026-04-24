package orchestrator

import (
	"fmt"
	"strings"
)

type Playbook struct {
	Name           string       `json:"name"`
	Summary        string       `json:"summary"`
	Principles     []string     `json:"principles"`
	TicketSOP      []string     `json:"ticket_sop"`
	WorkerLanes    []WorkerLane `json:"worker_lanes"`
	StarterPrompts []string     `json:"starter_prompts"`
}

type WorkerLane struct {
	Name         string `json:"name"`
	Purpose      string `json:"purpose"`
	DefaultStyle string `json:"default_style"`
}

func DefaultPlaybook() Playbook {
	return Playbook{
		Name:    "Orchestrator",
		Summary: "Clarify scope, inspect project context, and create or update work streams and tickets. Leave implementation to workers.",
		Principles: []string{
			"Chat first, ticket second. Clarify the goal before creating work.",
			"Author small tickets with explicit success criteria, relevant files, and acceptance tests.",
			"Use work streams to capture the initiative plan and the execution order.",
			"Keep the orchestrator on strategic decomposition; keep code execution on worker agents.",
			"Default worker lanes should be cost-conscious. Escalate model strength only when the ticket complexity justifies it.",
		},
		TicketSOP: []string{
			"Restate the user goal, scope boundaries, and non-goals.",
			"Call coordinator_get_history and inspect the relevant code before authoring work.",
			"Create or update a work stream that explains the initiative in Markdown.",
			"Decompose the goal into tickets that each represent one concern and one acceptance boundary.",
			"Set success criteria and an acceptance test for every ticket whenever possible.",
			"Use depends_on edges to express the execution DAG explicitly.",
			"Reserve planner/reviewer/deployer tickets for cases where those roles are materially useful.",
			"Prefer executor tickets that a small or medium model can finish from local context.",
		},
		WorkerLanes: []WorkerLane{
			{
				Name:         "Orchestrator",
				Purpose:      "Translate conversation into work streams and ticket DAGs.",
				DefaultStyle: "Use the strongest available model and high reasoning effort.",
			},
			{
				Name:         "Executor",
				Purpose:      "Implement narrowly-scoped coding tasks with tests and clear acceptance criteria.",
				DefaultStyle: "Prefer small or medium models by default; only escalate when the ticket is unusually cross-cutting or ambiguous.",
			},
			{
				Name:         "Validator",
				Purpose:      "Review diffs, run checks, and approve or reject with concrete feedback.",
				DefaultStyle: "Default to a cheaper verification model unless the review surface is broad or high risk.",
			},
		},
		StarterPrompts: []string{
			"Create or update a work stream and draft the smallest useful ticket DAG.",
			"What missing scope or constraints should we resolve before creating tickets?",
			"Review the current queue and identify what should be replanned, merged, or delegated next.",
		},
	}
}

func (p Playbook) PromptAppendix() string {
	var b strings.Builder
	b.WriteString("## Command Center Orchestrator SOP\n\n")
	b.WriteString(p.Summary)
	b.WriteString("\n\n### Operating principles\n\n")
	for _, item := range p.Principles {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("\n### Ticket authoring SOP\n\n")
	for i, item := range p.TicketSOP {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	b.WriteString("\n### Model lane strategy\n\n")
	for _, lane := range p.WorkerLanes {
		b.WriteString(fmt.Sprintf("- **%s**: %s %s\n", lane.Name, lane.Purpose, lane.DefaultStyle))
	}
	return strings.TrimSpace(b.String())
}
