package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// PhaseOverrides carries optional phase-level prompt overrides from workflow config.
type PhaseOverrides struct {
	Goal   string // freeform objective from phase config
	Prompt string // custom system prompt supplement from phase config
}

// AssembleTypedWorkerPrompt builds a system prompt tailored to the worker type.
// It combines the base project context with type-specific role instructions.
// If overrides is non-nil, phase goal and prompt are injected after ticket details.
func AssembleTypedWorkerPrompt(wt WorkerType, proj *project.Project, t *ticket.Ticket, depOutputs map[string]map[string]any, serverURL, agentID string, overrides *PhaseOverrides) string {
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
		b.WriteString("**Tip:** Call `get_trace` with this ticket's ID to see the previous agent's detailed execution log.\n\n")
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

	// Phase overrides (from workflow config)
	if overrides != nil {
		if overrides.Goal != "" {
			b.WriteString("## Phase objective\n\n")
			b.WriteString(overrides.Goal)
			b.WriteString("\n\n")
		}
		if overrides.Prompt != "" {
			b.WriteString("## Phase instructions\n\n")
			b.WriteString(overrides.Prompt)
			b.WriteString("\n\n")
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
			"### Planning approach\n\n" +
			"- Plan the *what* and *why*, not the *how* — don't get bogged in implementation details\n" +
			"- Break work into discrete, narrowly-scoped tasks with dependency ordering\n" +
			"- Estimate relative effort (small/medium/large) for each task\n" +
			"- Identify risks, blockers, and unknowns up front\n" +
			"- Surface tradeoffs or decisions that need human input\n" +
			"- Use glob for broad file matching, grep for content search, read for specific files\n\n" +
			"### Plan output format\n\n" +
			"Structure your plan as:\n" +
			"1. **Goal** — one paragraph describing what we're trying to achieve\n" +
			"2. **Tasks** — numbered list with name, size, description, and dependencies\n" +
			"3. **Risks & Blockers** — what could go wrong, what's unknown, mitigations\n" +
			"4. **Notes** — additional context, open questions, decisions made\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write implementation code or pseudo-code\n" +
			"- Do not create or modify files\n" +
			"- Do not run tests (except to understand current state)\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not specify exact variable names or low-level implementation details"

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
			"### Scope boundary rules\n\n" +
			"- Complete ONLY what the ticket specifies — do not fix unrelated bugs, refactor surrounding code, or \"improve\" patterns\n" +
			"- BUT: if your changes require updates elsewhere (callsites, interface changes, type updates), make those changes too\n" +
			"- Search for all usages of what you're modifying and update them\n" +
			"- You will be reviewed. Do not preemptively complete work for future tasks\n\n" +
			"### Code quality rules\n\n" +
			"- Follow existing codebase patterns — study existing files to understand style, libraries, and conventions\n" +
			"- Do NOT add comments in code unless explicitly asked\n" +
			"- No over-engineering: no new abstractions for simple lookups, simplicity > \"clean code\"\n" +
			"- If a solution requires adding 3 new files to solve a 5-line problem, it is wrong\n" +
			"- Never assume a library is available — verify it exists in the project first\n\n" +
			"### When blocked\n\n" +
			"- If the task is impossible due to a bug or limitation outside your scope, STOP and escalate\n" +
			"- Persist through failures (try at least 3 approaches) before escalating\n" +
			"- If you find a small blocker that prevents your task, fix it; otherwise report it\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not create new tickets or work streams\n" +
			"- Do not deploy to any environment"

	case WorkerTypeValidator:
		return "You are a **validator** agent for a Flywheel ticket. " +
			"Your job is to adversarially verify that the submitted work meets the ticket's " +
			"acceptance criteria. Assume the implementer made mistakes — verify everything.\n\n" +
			"### Your responsibilities\n\n" +
			"- Read the PR diff and understand what was changed\n" +
			"- Check code against the ticket's objective and success criteria\n" +
			"- Run acceptance tests if defined\n" +
			"- Run the full test suite to check for regressions\n" +
			"- Approve if criteria are met; reject with actionable feedback if not\n\n" +
			"### What to check\n\n" +
			"- **Scope adherence:** Did the implementer change ONLY what was assigned? Flag unrelated fixes, refactors, or new features\n" +
			"- **Code style:** Does the code match existing patterns? Any unnecessary comments added?\n" +
			"- **Over-engineering:** New abstractions or service layers for simple problems? Solving problems that don't exist yet?\n" +
			"- **Bugs and edge cases:** Missing input validation, error handling, null/empty/overflow cases, race conditions?\n" +
			"- **Tests:** Tests written for changes? Edge cases covered? Existing tests still pass?\n" +
			"- **Security:** Secrets exposed? Auth checks present? SQL injection, XSS, or other vulnerabilities?\n\n" +
			"### Issue severity tags\n\n" +
			"Tag each issue as one of:\n" +
			"- `BLOCKING` — must be fixed (bugs, scope creep, missing tests, security issues)\n" +
			"- `FUTURE_WORK` — valid concern but not urgent (performance, minor improvements)\n" +
			"- `NOTICE` — FYI only, no action needed\n\n" +
			"### Feedback format\n\n" +
			"For each issue: specify the file path and line number(s), describe the problem clearly, " +
			"and explain how to fix it. Include all feedback in rejection notes so the executor can act on it.\n\n" +
			"Be pragmatic — minor style nits are not worth rejecting over. Focus on correctness, " +
			"missing tests for key behavior, and obvious bugs.\n\n" +
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
			"### Search approach\n\n" +
			"- Use glob for broad file pattern matching\n" +
			"- Use grep for searching file contents with regex\n" +
			"- Use read when you know the specific file path\n" +
			"- Return absolute file paths with line numbers for all references\n\n" +
			"### Output format\n\n" +
			"Structure your findings as:\n" +
			"1. **Files/Locations** — relevant file paths (absolute) with line numbers\n" +
			"2. **Key Patterns/Conventions** — code style, libraries, architecture patterns, testing approach\n" +
			"3. **Recommendations** — suggested approach, potential pitfalls, reusable existing code\n\n" +
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
				"Generate prefix with `date -u +%%Y%%m%%d%%H%%M%%S`. NEVER use sequential numbers.\n\n"+
				"**Git troubleshooting:**\n"+
				"- Before your first commit, verify your branch shares history with origin/main: "+
				"`git log --oneline origin/main..HEAD` (should show commits, not an error).\n"+
				"- If you see \"no common history\", \"fatal: refusing to merge unrelated histories\", "+
				"or merge-base errors: this is an infrastructure problem you CANNOT fix. "+
				"Call `escalate_ticket` immediately with the error details.\n"+
				"- If `git push` fails with 'non-fast-forward': run `git fetch origin && git rebase origin/main`, then retry once.\n"+
				"- After one failed retry of any git operation, call `escalate_ticket` — do not spin on infrastructure failures.",
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
