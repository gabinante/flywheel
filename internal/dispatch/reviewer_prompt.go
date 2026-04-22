package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// AssembleReviewerPrompt builds a system prompt for a reviewer agent.
// The reviewer reads the PR diff, checks code quality, and approves or rejects.
func AssembleReviewerPrompt(proj *project.Project, t *ticket.Ticket, serverURL, agentID string) string {
	var b strings.Builder

	b.WriteString("You are a code reviewer for a Flywheel ticket. ")
	b.WriteString("Your job is to review the pull request, check the code, and approve or reject.\n\n")

	// Project context
	if proj.ContextPack.SystemPrompt != "" {
		b.WriteString("## Project context\n\n")
		b.WriteString(proj.ContextPack.SystemPrompt)
		b.WriteString("\n\n")
	}
	if proj.ContextPack.Conventions != "" {
		b.WriteString("## Conventions\n\n")
		b.WriteString(proj.ContextPack.Conventions)
		b.WriteString("\n\n")
	}

	// Ticket details
	b.WriteString("## Ticket under review\n\n")
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

	// PR URL from outputs
	prURL := ""
	if u, ok := t.Outputs["pr_url"].(string); ok {
		prURL = u
	}

	// Review instructions
	b.WriteString("## Review workflow\n\n")
	if prURL != "" {
		b.WriteString(fmt.Sprintf("**PR to review:** %s\n\n", prURL))
	}
	b.WriteString("1. Read the PR diff: `gh pr diff` (or `gh pr diff <number>`).\n")
	b.WriteString("2. Check the code against the ticket's objective and success criteria.\n")
	b.WriteString("3. Run tests if an acceptance test is defined: execute the command and verify it passes.\n")
	b.WriteString("4. Review for:\n")
	b.WriteString("   - Correctness: does the code do what the ticket asks?\n")
	b.WriteString("   - Quality: clean code, no obvious bugs, reasonable structure.\n")
	b.WriteString("   - Tests: are there tests? Do they cover the key paths?\n")
	b.WriteString("   - No regressions: does `make test` still pass?\n")
	b.WriteString("5. If you find issues:\n")
	b.WriteString("   - Add comments to the PR: `gh pr comment <number> --body \"<feedback>\"`\n")
	b.WriteString(fmt.Sprintf("   - Call `reject_ticket` with `ticket_id: \"%s\"` and `notes` describing what needs to change.\n", t.ID))
	b.WriteString("6. If the code looks good:\n")
	b.WriteString(fmt.Sprintf("   - Call `approve_ticket` with `ticket_id: \"%s\"`.\n", t.ID))
	b.WriteString("\n")
	b.WriteString("**Be pragmatic.** Minor style nits are not worth rejecting over. Focus on correctness, missing tests for key behavior, and obvious bugs. If the code meets the objective and success criteria, approve it.\n\n")

	b.WriteString(fmt.Sprintf("**Project ID:** %s\n", proj.ID))
	b.WriteString(fmt.Sprintf("**Server URL:** %s\n", serverURL))
	b.WriteString(fmt.Sprintf("**Agent ID:** %s\n", agentID))

	return b.String()
}

// buildReviewerTaskPrompt returns the user-turn prompt for the reviewer agent.
func buildReviewerTaskPrompt(ticketID string) string {
	return fmt.Sprintf(
		"Review Flywheel ticket %s. "+
			"Read the PR diff, check code quality and correctness against the ticket objectives, "+
			"run tests if applicable, then either approve_ticket or reject_ticket with notes. "+
			"You MUST use the Flywheel MCP tools to approve or reject.",
		ticketID,
	)
}
