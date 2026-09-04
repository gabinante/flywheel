package dispatch

import (
	"fmt"
	"github.com/gabinante/flywheel/internal/prompts"
	"sort"
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

	// Ticket progress (only present on re-entry)
	b.WriteString(buildProgressBlock(t))

	// Prior attempts (before objective so executor sees feedback first)
	if len(t.Context.PriorAttempts) > 0 {
		b.WriteString("### Prior attempts (IMPORTANT: address all feedback)\n\n")
		for i, a := range t.Context.PriorAttempts {
			b.WriteString(fmt.Sprintf("**Attempt %d** (outcome: %s):\n%s\n\n", i+1, a.Outcome, a.Summary))
		}
		b.WriteString("**Tip:** Call `get_trace` with this ticket's ID to see the previous agent's detailed execution log.\n\n")
	}

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
func defaultWorkerTypeRolePreamble(wt WorkerType) string {
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
			"### Assessing current state\n\n" +
			"- After starting the ticket, FIRST investigate whether the work is already done or partially done\n" +
			"- Check relevant files and run the acceptance test (if defined) before writing any code\n" +
			"- If the feature is **already fully implemented** and working: log the finding, then call `submit_ticket` with outputs explaining it was already complete. Do NOT escalate — this is a valid resolution\n" +
			"- If the feature is **partially implemented**: continue from the current state. Build on what exists rather than starting from scratch\n" +
			"- Only start writing new code after you understand what already exists\n\n" +
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

	case WorkerTypeDecomposer:
		return "You are a **decomposer** agent for a Flywheel ticket. " +
			"Your job is to analyze ticket scope and break large tickets into well-scoped, " +
			"independently implementable subtickets.\n\n" +
			"### Your responsibilities\n\n" +
			"- Read and understand the ticket objective, success criteria, and codebase context\n" +
			"- Identify natural task boundaries and decomposition points\n" +
			"- Create subtickets via `create_ticket` with proper `depends_on` ordering\n" +
			"- Each subticket should have a single clear objective and be independently testable\n" +
			"- Include parent ticket ID in subticket description for traceability (e.g. \"Decomposed from: <parent_ticket_id>\")\n" +
			"- Set `workflow_id` on subtickets to assign a simpler workflow (no decompose phase)\n\n" +
			"### When NOT to decompose\n\n" +
			"- If the ticket is already well-scoped (single objective, clear acceptance criteria, reasonable size), " +
			"submit immediately noting no decomposition needed\n" +
			"- Do not decompose for the sake of decomposing — only when it genuinely reduces complexity\n\n" +
			"### Subticket quality\n\n" +
			"- Each subticket must have: a clear title, description, success criteria, and acceptance test\n" +
			"- Use `depends_on` to order subtickets that depend on each other\n" +
			"- Keep subtickets at a level where a single executor agent can complete them in one session\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write implementation code\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not create work streams\n" +
			"- Do not deploy to any environment"

	case WorkerTypeOperator:
		return "You are an **operator** agent for a Flywheel ticket. " +
			"Your job is to analyze, triage, or respond to operational events. " +
			"You are a non-coding agent — you analyze and report, not implement.\n\n" +
			"### Your responsibilities\n\n" +
			"- Analyze alerts, incidents, or operational tickets\n" +
			"- Triage issues and classify severity\n" +
			"- Execute runbook steps and document findings\n" +
			"- Query system state and report observations\n" +
			"- Recommend actions or escalate to humans when needed\n\n" +
			"### What you do NOT do\n\n" +
			"- Do not write or modify source code\n" +
			"- Do not use version control or open review requests\n" +
			"- Do not approve or reject tickets\n" +
			"- Do not deploy to any environment"

	default:
		// Fallback to generic prompt for unknown types.
		return "You are an agent executing a Flywheel ticket. " +
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
				"3. Assess the current state: check relevant files and run the acceptance test (if any) to see if the work is already done.\n"+
				"   - If fully implemented: log the finding and skip to step 6 (submit_ticket with summary that work was already complete).\n"+
				"   - If partially done: implement only the remaining work.\n"+
				"   - If not started: implement the full objective.\n"+
				"   Call `log_step` with `ticket_id`, `lease_token`, and `step_type` after each significant action.\n"+
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
				"   - Post a formal GitHub review requesting changes: `gh pr review <number> --request-changes --body \"<feedback>\"`\n"+
				"   - Call `reject_ticket` with `ticket_id: \"%s\"` and notes describing what needs to change.\n"+
				"6. If the code looks good:\n"+
				"   - Post a formal GitHub approval review: `gh pr review <number> --approve --body \"Looks good.\"`\n"+
				"   - Call `approve_ticket` with `ticket_id: \"%s\"`.\n\n"+
				"**IMPORTANT:** Always use `gh pr review` (not `gh pr comment`) so the review status is visible on GitHub.\n\n"+
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

	case WorkerTypeDecomposer:
		return fmt.Sprintf(
			"Follow these steps using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"` to get your lease.\n"+
				"2. Call `start_ticket` with the ticket_id and lease_token.\n"+
				"3. Read the ticket objective and investigate the codebase to understand scope.\n"+
				"4. Decide: decompose or pass through.\n"+
				"   - **Well-scoped:** Call `log_step` noting the ticket is already well-scoped, "+
				"then `submit_ticket` with outputs: `{\"decomposition\": \"none\", \"reason\": \"already well-scoped\"}`.\n"+
				"   - **Needs decomposition:** Continue to step 5.\n"+
				"5. Create subtickets via `create_ticket`:\n"+
				"   - Each gets: title, description (include \"Decomposed from: %s\"), success_criteria, acceptance_test\n"+
				"   - Set `depends_on` for ordering between siblings where needed\n"+
				"   - Set `workflow_id` to a simpler workflow (e.g. \"Subticket SDLC\" or \"Fast Track\")\n"+
				"   - Set `inputs` with `{\"decomposed_from\": \"%s\"}` for provenance tracking\n"+
				"6. Call `log_step` after each subticket creation.\n"+
				"7. Call `submit_ticket` with outputs listing the created subticket IDs.\n"+
				"8. If blocked, use `escalate_ticket` to ask for human help.",
			projectID, ticketID, ticketID,
		)

	case WorkerTypeOperator:
		return fmt.Sprintf(
			"Follow these steps using the Flywheel MCP tools:\n\n"+
				"1. Call `claim_ticket` with `project_id: \"%s\"` to get your lease.\n"+
				"2. Call `start_ticket` with the ticket_id and lease_token.\n"+
				"3. Analyze the situation:\n"+
				"   - Query system state and observations\n"+
				"   - Review alerts, logs, or relevant context\n"+
				"   - Classify severity and identify root cause\n"+
				"4. Call `log_step` after each significant finding.\n"+
				"5. Call `submit_ticket` with your analysis and recommendations in outputs.\n"+
				"6. If blocked or a decision requires human judgment, use `escalate_ticket`.\n\n"+
				"**IMPORTANT:** Do not write code or modify files. You are analysis-only.",
			projectID,
		)

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

// buildProgressBlock creates a ticket progress context block for re-spawned workers.
// Returns empty string for fresh tickets (no prior attempts).
func buildProgressBlock(t *ticket.Ticket) string {
	if len(t.Context.PriorAttempts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Ticket progress\n\n")

	// Attempt count with breakdown
	counts := map[string]int{}
	for _, a := range t.Context.PriorAttempts {
		counts[a.Outcome]++
	}
	attemptNum := len(t.Context.PriorAttempts) + 1
	var parts []string
	for outcome, n := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", n, outcome))
	}
	// Sort for deterministic output
	sort.Strings(parts)
	b.WriteString(fmt.Sprintf("- **Attempt:** %d (%d prior: %s)\n",
		attemptNum, len(t.Context.PriorAttempts), strings.Join(parts, ", ")))

	// Existing branch
	if branch, ok := t.Outputs["_branch"].(string); ok && branch != "" {
		b.WriteString(fmt.Sprintf("- **Branch:** `%s` (exists — do NOT create a new branch)\n", branch))
	}

	// Existing PR
	if prURL, ok := t.Outputs["pr_url"].(string); ok && prURL != "" {
		b.WriteString(fmt.Sprintf("- **PR:** %s (exists — push fixes to this PR, do not create a new one)\n", prURL))
	}

	// Last outcome + action required
	last := t.Context.PriorAttempts[len(t.Context.PriorAttempts)-1]
	b.WriteString(fmt.Sprintf("- **Last outcome:** %s\n", last.Outcome))

	switch last.Outcome {
	case "rejected":
		b.WriteString("- **Action required:** Fix all BLOCKING issues from the most recent review before resubmitting.\n")
	case "worker_exit":
		b.WriteString("- **Action required:** Previous worker exited without completing. Review the trace (`get_trace`), then pick up where it left off.\n")
	default:
		b.WriteString("- **Action required:** Retry the work. Check the trace (`get_trace`) for context on what happened.\n")
	}

	b.WriteString("\n")
	return b.String()
}

// workerTypeRolePreamble returns the operator-editable role preamble for a worker type.
func workerTypeRolePreamble(wt WorkerType) string {
	if t := prompts.Text("dispatch_" + string(wt)); t != "" {
		return t
	}
	return defaultWorkerTypeRolePreamble(wt)
}

var workerTypePromptMeta = []struct {
	wt    WorkerType
	name  string
	order int
}{
	{WorkerTypePlanner, "Planner", 51}, {WorkerTypeDecomposer, "Decomposer", 52}, {WorkerTypeExecutor, "Executor", 53},
	{WorkerTypeValidator, "Validator", 54}, {WorkerTypeDeployer, "Deployer", 55}, {WorkerTypeInvestigator, "Investigator", 56},
	{WorkerTypeOperator, "Operator", 57},
}

func init() {
	for _, m := range workerTypePromptMeta {
		prompts.Register(prompts.Prompt{ID: "dispatch_" + string(m.wt), Name: "Dispatch worker: " + m.name, Order: m.order,
			Description: "Role preamble for the " + strings.ToLower(m.name) + " worker type. Project context, ticket details, dependency outputs and the MCP protocol are appended automatically.",
			UsedBy:      "Dispatch (" + string(m.wt) + " phases)", Default: defaultWorkerTypeRolePreamble(m.wt)})
	}
}
