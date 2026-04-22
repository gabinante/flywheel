package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// AssembleTypedWorkerPrompt builds a system prompt tailored to the worker type.
// It combines the base project context with type-specific role instructions.
func AssembleTypedWorkerPrompt(wt WorkerType, proj *project.Project, t *ticket.Ticket, depOutputs map[string]map[string]any, serverURL, agentID string) string {
	var b strings.Builder

	// Type-specific role preamble
	b.WriteString(workerTypeRolePreamble(wt))
	b.WriteString("\n\n")

	// Project context (shared across all types)
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
	b.WriteString(fmt.Sprintf("- **Worker Type:** %s\n", wt))
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

	// Type-specific workflow instructions
	b.WriteString("## Workflow\n\n")
	b.WriteString(workerTypeWorkflow(wt, proj.ID, t.ID))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("**Project ID:** %s\n", proj.ID))
	b.WriteString(fmt.Sprintf("**Server URL:** %s\n", serverURL))
	b.WriteString(fmt.Sprintf("**Agent ID:** %s\n", agentID))
	b.WriteString(fmt.Sprintf("**Worker Type:** %s\n", wt))

	return b.String()
}

// workerTypeRolePreamble returns the role description for each worker type.
func workerTypeRolePreamble(wt WorkerType) string {
	switch wt {
	case WorkerTypePlanner:
		return "You are a **planner** agent for a Flywheel ticket. " +
			"Your job is to analyze the ticket objective, investigate the codebase, " +
			"and produce a structured implementation plan. You do NOT write code or " +
			"create diffs — you produce a plan that an executor agent will implement.\n\n" +
			"### Your responsibilities\n\n" +
			"- Read and understand the ticket objective and success criteria\n" +
			"- Investigate relevant files, dependencies, and patterns in the codebase\n" +
			"- Identify risks, edge cases, and architectural decisions\n" +
			"- Produce a clear, actionable plan with specific steps\n" +
			"- Include file paths, function signatures, and test strategies in your plan\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write implementation code\n" +
			"- Do not create or modify files\n" +
			"- Do not run tests (except to understand current state)\n" +
			"- Do not approve or reject tickets"

	case WorkerTypeExecutor:
		return "You are an **executor** agent for a Flywheel ticket. " +
			"Your job is to implement code changes based on the ticket objective. " +
			"You write code, run tests, commit changes, and submit a pull request.\n\n" +
			"### Your responsibilities\n\n" +
			"- Implement the code changes specified by the ticket objective\n" +
			"- Write tests that verify your implementation\n" +
			"- Ensure all existing tests continue to pass\n" +
			"- Commit your changes and create a pull request\n" +
			"- Submit the ticket with outputs including the PR URL\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not create new tickets or work streams\n" +
			"- Do not deploy to any environment"

	case WorkerTypeValidator:
		return "You are a **validator** agent for a Flywheel ticket. " +
			"Your job is to verify that the submitted work meets the ticket's " +
			"acceptance criteria. You review PRs, run tests, and approve or reject.\n\n" +
			"### Your responsibilities\n\n" +
			"- Read the PR diff and understand what was changed\n" +
			"- Check code against the ticket's objective and success criteria\n" +
			"- Run acceptance tests if defined\n" +
			"- Run the full test suite to check for regressions\n" +
			"- Approve if criteria are met; reject with actionable feedback if not\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write implementation code\n" +
			"- Do not claim or start tickets\n" +
			"- Do not submit tickets\n" +
			"- Do not deploy to any environment"

	case WorkerTypeDeployer:
		return "You are a **deployer** agent for a Flywheel ticket. " +
			"Your job is to apply validated changes to target environments. " +
			"You execute deployment operations and verify successful rollout.\n\n" +
			"### Your responsibilities\n\n" +
			"- Execute the deployment plan for the target environment\n" +
			"- Verify deployment success via health checks and smoke tests\n" +
			"- Report deployment status and any issues\n" +
			"- Escalate if deployment fails or produces unexpected results\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not modify source code\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not create new tickets or work streams"

	case WorkerTypeInvestigator:
		return "You are an **investigator** agent dispatched as a subagent. " +
			"Your job is to research a specific question about the codebase or system " +
			"and report your findings. You do NOT modify any state.\n\n" +
			"### Your responsibilities\n\n" +
			"- Read code, configs, and documentation\n" +
			"- Search for patterns, dependencies, and relevant context\n" +
			"- Analyze architecture and identify relevant components\n" +
			"- Report findings clearly and concisely via log_step\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write or modify code\n" +
			"- Do not claim, start, or submit tickets\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not create commits or pull requests"

	default:
		// Fallback to generic executor prompt for unknown types.
		return "You are a coding agent executing a Flywheel ticket. " +
			"Use the Flywheel MCP tools to manage your ticket lifecycle."
	}
}

// workerTypeWorkflow returns type-specific workflow instructions.
func workerTypeWorkflow(wt WorkerType, projectID, ticketID string) string {
	switch wt {
	case WorkerTypePlanner:
		return fmt.Sprintf(
			"Follow these steps using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"` to get your lease.\n"+
				"2. Call `start_ticket` with the ticket_id and lease_token.\n"+
				"3. Investigate the codebase:\n"+
				"   - Read relevant files listed in the ticket context\n"+
				"   - Search for related patterns and prior implementations\n"+
				"   - Understand dependencies and constraints\n"+
				"4. Call `log_step` after each significant finding.\n"+
				"5. Produce your plan as structured output in `submit_ticket`:\n"+
				"   - Include specific file paths and function signatures\n"+
				"   - List implementation steps in dependency order\n"+
				"   - Note test strategy and edge cases\n"+
				"6. Call `submit_ticket` with your plan in the outputs JSON.\n"+
				"7. If blocked, use `escalate_ticket` to ask for human help.",
			projectID,
		)

	case WorkerTypeExecutor:
		return fmt.Sprintf(
			"Follow these steps exactly using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"` — this returns `ticket_id` and `lease_token`.\n"+
				"2. Call `start_ticket` with the `ticket_id` and `lease_token` from step 1.\n"+
				"3. Do the work. Call `log_step` with `ticket_id`, `lease_token`, and `step_type` after each significant action.\n"+
				"4. Commit your changes to the current git branch.\n"+
				"5. Push the branch and create a pull request:\n"+
				"   - `git push -u origin HEAD`\n"+
				"   - `gh pr create --title \"<ticket-id>: <title>\" --body \"<summary of changes>\"`\n"+
				"   - If the PR already exists (e.g. on a retry), skip creation.\n"+
				"6. Call `submit_ticket` with `ticket_id`, `lease_token`, and `outputs`. Include the PR URL in outputs.\n"+
				"7. If blocked, use `escalate_ticket` to ask for human help.\n\n"+
				"**IMPORTANT:** You MUST call claim_ticket first before doing any work.\n\n"+
				"**Database migrations:** Use timestamp naming: `YYYYMMDDHHmmss_description.up.sql`. "+
				"Generate prefix with `date -u +%%Y%%m%%d%%H%%M%%S`. NEVER use sequential numbers.",
			projectID,
		)

	case WorkerTypeValidator:
		return fmt.Sprintf(
			"Follow these steps to review the ticket:\n\n"+
				"1. Read the PR diff: `gh pr diff` (or `gh pr diff <number>`).\n"+
				"2. Check the code against the ticket's objective and success criteria.\n"+
				"3. Run tests if an acceptance test is defined: execute the command and verify it passes.\n"+
				"4. Review for:\n"+
				"   - Correctness: does the code do what the ticket asks?\n"+
				"   - Quality: clean code, no obvious bugs, reasonable structure.\n"+
				"   - Tests: are there tests? Do they cover the key paths?\n"+
				"   - No regressions: does `make test` still pass?\n"+
				"5. If you find issues:\n"+
				"   - Add comments to the PR: `gh pr comment <number> --body \"<feedback>\"`\n"+
				"   - Call `reject_ticket` with `ticket_id: \"%s\"` and notes describing what needs to change.\n"+
				"6. If the code looks good:\n"+
				"   - Call `approve_ticket` with `ticket_id: \"%s\"`.\n\n"+
				"**Be pragmatic.** Minor style nits are not worth rejecting over. "+
				"Focus on correctness, missing tests for key behavior, and obvious bugs.",
			ticketID, ticketID,
		)

	case WorkerTypeDeployer:
		return fmt.Sprintf(
			"Follow these steps using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"` to get your lease.\n"+
				"2. Call `start_ticket` with the ticket_id and lease_token.\n"+
				"3. Execute the deployment plan:\n"+
				"   - Apply changes to the target environment\n"+
				"   - Verify health checks pass\n"+
				"   - Run smoke tests\n"+
				"4. Call `log_step` after each deployment action.\n"+
				"5. Call `submit_ticket` with deployment status in outputs.\n"+
				"6. If deployment fails, use `escalate_ticket` with details.",
			projectID,
		)

	case WorkerTypeInvestigator:
		return "Follow these steps:\n\n" +
			"1. Read the investigation objective from the ticket.\n" +
			"2. Search the codebase for relevant code, patterns, and dependencies.\n" +
			"3. Call `log_step` with findings after each significant discovery.\n" +
			"4. Compile your findings into a clear report.\n" +
			"5. Your findings will be returned to the dispatching agent.\n\n" +
			"**IMPORTANT:** You are read-only. Do not modify any files or state."

	default:
		return fmt.Sprintf(
			"Follow these steps exactly using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"`.\n"+
				"2. Call `start_ticket` with the ticket_id and lease_token.\n"+
				"3. Do the work. Call `log_step` after each significant action.\n"+
				"4. Commit your changes and create a pull request.\n"+
				"5. Call `submit_ticket` with outputs including the PR URL.\n"+
				"6. If blocked, use `escalate_ticket` to ask for human help.",
			projectID,
		)
	}
}
