package dispatch

import (
	"fmt"
	"strings"

	"github.com/matt0x6f/warrant/internal/project"
	"github.com/matt0x6f/warrant/internal/ticket"
)

// AssembleWorkerPrompt builds a system prompt for a Claude Code worker session.
// It combines the project context pack with ticket-specific details.
func AssembleWorkerPrompt(proj *project.Project, t *ticket.Ticket, depOutputs map[string]map[string]any, serverURL, agentID string) string {
	var b strings.Builder

	// Role
	b.WriteString("You are a coding agent executing a warrant ticket. ")
	b.WriteString("Use the warrant MCP tools to manage your ticket lifecycle.\n\n")

	// Project context
	if proj.ContextPack.SystemPrompt != "" {
		b.WriteString("## Project system prompt\n\n")
		b.WriteString(proj.ContextPack.SystemPrompt)
		b.WriteString("\n\n")
	}
	if proj.ContextPack.Conventions != "" {
		b.WriteString("## Conventions\n\n")
		b.WriteString(proj.ContextPack.Conventions)
		b.WriteString("\n\n")
	}
	if len(proj.ContextPack.KeyFiles) > 0 {
		b.WriteString("## Key files\n\n")
		for _, f := range proj.ContextPack.KeyFiles {
			b.WriteString(fmt.Sprintf("- `%s`", f.Path))
			if f.Snippet != "" {
				b.WriteString(fmt.Sprintf(": %s", f.Snippet))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Ticket details
	b.WriteString("## Your ticket\n\n")
	b.WriteString(fmt.Sprintf("- **ID:** %s\n", t.ID))
	b.WriteString(fmt.Sprintf("- **Title:** %s\n", t.Title))
	b.WriteString(fmt.Sprintf("- **Type:** %s\n", t.Type))
	b.WriteString(fmt.Sprintf("- **Priority:** P%d\n", t.Priority))
	b.WriteString("\n")

	b.WriteString("### Objective\n\n")
	b.WriteString(t.Objective.Description)
	b.WriteString("\n\n")

	if len(t.Objective.SuccessCriteria) > 0 {
		b.WriteString("### Success criteria\n\n")
		for _, c := range t.Objective.SuccessCriteria {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	if t.Objective.AcceptanceTest != "" {
		b.WriteString("### Acceptance test\n\n")
		b.WriteString(fmt.Sprintf("```\n%s\n```\n\n", t.Objective.AcceptanceTest))
	}

	// Context
	if len(t.Context.RelevantFiles) > 0 {
		b.WriteString("### Relevant files\n\n")
		for _, f := range t.Context.RelevantFiles {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		b.WriteString("\n")
	}
	if len(t.Context.Constraints) > 0 {
		b.WriteString("### Constraints\n\n")
		for _, c := range t.Context.Constraints {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	// Prior attempts (rejection feedback)
	if len(t.Context.PriorAttempts) > 0 {
		b.WriteString("### Prior attempts (IMPORTANT: address all feedback)\n\n")
		for i, a := range t.Context.PriorAttempts {
			b.WriteString(fmt.Sprintf("**Attempt %d** (outcome: %s):\n%s\n\n", i+1, a.Outcome, a.Summary))
		}
	}

	// Human answers from escalation
	if len(t.Context.HumanAnswers) > 0 {
		b.WriteString("### Human answers\n\n")
		for _, a := range t.Context.HumanAnswers {
			b.WriteString(fmt.Sprintf("- %s\n", a))
		}
		b.WriteString("\n")
	}

	// Dependency outputs
	if len(depOutputs) > 0 {
		b.WriteString("### Dependency outputs\n\n")
		for depID, outputs := range depOutputs {
			b.WriteString(fmt.Sprintf("**%s:**\n", depID))
			for k, v := range outputs {
				b.WriteString(fmt.Sprintf("- %s: %v\n", k, v))
			}
			b.WriteString("\n")
		}
	}

	// Workflow instructions
	b.WriteString("## Workflow\n\n")
	b.WriteString("1. `claim_ticket` with project_id and your ticket ID\n")
	b.WriteString("2. `start_ticket` to move to executing\n")
	b.WriteString("3. Do the work. Call `log_step` after each significant action.\n")
	b.WriteString("4. `submit_ticket` with your outputs when done.\n")
	b.WriteString("5. If blocked, use `escalate_ticket` to ask for human help.\n\n")

	b.WriteString(fmt.Sprintf("**Server URL:** %s\n", serverURL))
	b.WriteString(fmt.Sprintf("**Agent ID:** %s\n", agentID))

	return b.String()
}
