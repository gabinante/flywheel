package mcp

// AgentGuideURI is the URI for the MCP resource that describes the typical agent flow.
const AgentGuideURI = "flywheel://docs/agent-guide"

// AgentGuideContent is the markdown content for the agent guide resource.
// Uses (param) instead of `param` so the string can be a single raw literal.
const AgentGuideContent = `# Flywheel MCP – Agent guide

Use this flow when working on tickets via Flywheel. Your identity is tied to your OAuth login; you only see projects in organizations you belong to.

**Work streams + Git:** If the project has **repo_url** and you use **work streams**, you must call **update_work_stream** with **branch** after you create or check out the Git branch. **update_work_stream_plan** only changes Markdown—it does **not** set the branch. Omitting **branch** is a common mistake; **claim_ticket** / **get_ticket** will keep returning **create_or_set_branch** until you fix it.

## Typical flow

1. **list_projects** (no args when using OAuth) – Returns projects in all organizations you are a member of. Optionally pass (org_id) to limit to one org.
2. **get_project_context** (project_id) – Load conventions, key files, system prompt, and extra hints for the project.
3. **list_tickets** (project_id, optional state, priority) – See available tickets. Filter by state (e.g. pending) or priority (0–3).
4. **Work stream branch (when repo_url + work_stream)** – If you **create_work_stream** or will claim tickets tied to a work stream: in the same session, create or checkout the branch (see **git_instruction** from **create_work_stream** / **get_work_stream**), then **update_work_stream** with **branch** = the real branch name (run: git branch --show-current). Do this **before** **claim_ticket** when practical.
5. **claim_ticket** (project_id) – Claim the next available ticket. Returns the ticket and a **lease** (lease_token, expires_at). agent_id is inferred from OAuth.
6. **get_ticket** (ticket_id) – Load the full payload: objective, success criteria, acceptance test, context pack, dependency outputs, prior attempts, human answers. This is your main input for doing the work.
7. **start_ticket** (ticket_id, lease_token) – Move the ticket to **executing**.
8. **While working, interleave log_step** so reviewers see what you did:
   - After each significant tool or action: **log_step** with step_type **tool_call** and payload (e.g. name, input).
   - For key findings or decisions: **log_step** with **observation** or **thought** and a short payload (e.g. summary).
   - On failure: **log_step** with step_type **error** and payload describing the error.
   Use **renew_lease** if the job takes longer than the lease TTL.
9. When done:
   - **submit_ticket** (ticket_id, lease_token, outputs) – Submit deliverables (JSON) and move to **awaiting_review**. A human approves or rejects via REST.
   - **escalate_ticket** (ticket_id, lease_token, reason, question) – Ask for human help; ticket moves to **needs_human**. The human's answer is stored and the ticket returns to **executing**.

## Tool JSON workflow hints

Some tools return a (workflow) object alongside the main payload:

- (next_steps): ordered tool names to continue the happy path (claim → start → log → submit).
- (note): one short reminder (e.g. do not skip claim_ticket).

Shapes:

- create_ticket → JSON with keys ticket and workflow (the ticket is nested; it is not the only root field).
- claim_ticket → includes workflow next to ticket, lease (and optional work_stream / git_instruction).
- start_ticket → JSON with ok: true and workflow.
- list_tickets → JSON with tickets array; when you list all states or filter to pending only, workflow is also included.

## Tool summary

| Tool | Purpose |
|------|--------|
| list_orgs | List organizations you belong to (id, name, slug). OAuth required. |
| create_project | Create a project in your default org (name, optional slug). For initiatives/epics. |
| list_projects | List projects for your org(s). Default: active only; pass include_closed: true to include closed. OAuth required; org_id optional. |
| update_project_status | Set project status to active or closed (project_id, status). Use to close when done or reopen for follow-up. |
| create_ticket | Create a pending ticket in a project (project_id, title, description, optional type/priority). Fails if project is closed. |
| get_project_context | Context pack for a project (conventions, key files, system prompt). |
| list_tickets | List tickets by project; optional state/priority filter. |
| get_ticket | Full ticket + context + dependency outputs + prior attempts. |
| claim_ticket | Claim next ticket; returns ticket + lease. If none available, elicitation may ask you to force-release a stuck ticket. |
| force_release_lease | Force-release a ticket's lease (no token). Use when the user says "release X and claim it"; then call claim_ticket. |
| start_ticket | Move claimed to executing. |
| log_step | Append a step to the execution trace. Call as you work (after tool use, observations, errors) so reviewers see what was done. |
| submit_ticket | Submit outputs; move to awaiting_review. If the ticket has **objective.acceptance_test** and the server has it enabled, the server runs that command (e.g. shell script) before transitioning; on failure the submit is rejected with the test output so you can fix and retry. |
| escalate_ticket | Ask for human help; move to needs_human. |
| renew_lease | Extend lease TTL. |
| list_pending_reviews | List tickets in awaiting_review for a project. Use when the user asks "what needs my review?" or "show pending reviews". |
| get_trace | Get the execution trace for a ticket (all log_step entries). Use when summarizing a ticket for review. |
| create_work_stream / list_work_streams / get_work_stream / update_work_stream / **update_work_stream_plan** | Group tickets under a goal. **plan** = Markdown (GFM, code fences, **mermaid**). **update_work_stream_plan** changes plan text only—it does **not** set **branch**. When project has **repo_url**, always call **update_work_stream** with **branch** after creating/checking out the Git branch (see **Work streams and Git branches**). |
| approve_ticket | Approve a ticket in awaiting_review (moves to done). Use **only** when the user explicitly says to approve, ship it, looks good, etc. Do not approve to "sync status" without their say-so. |
| reject_ticket | Reject a ticket with required notes; returns to executing so the agent can fix and resubmit. |
| reopen_ticket | Move a ticket from **done** back to **awaiting_review** (e.g. mistaken approval). Optional **notes**. Use **only** when the user explicitly asks to reopen or return a completed ticket for review. |
| dispatch_investigation | **(Coordinator only)** Dispatch a scoped research investigation to a subagent. Returns structured findings (claims with citations, negative space, open questions). Use during the investigate phase before authoring tickets. One level deep — subagents cannot dispatch further investigations. |
| coordinator_get_history | **(Coordinator only)** Query prior ticket history and feedback findings for a project. Call at the start of every coordinator session for cross-session continuity. |
| coordinator_calibration | **(Coordinator only)** Surface calibration metrics: success rate, common failure patterns, authoring quality trends. Use to self-improve. |
| coordinator_record_feedback | **(Coordinator only)** Record a coordinator-feedback finding with category and analysis. Use to capture lessons from ticket outcomes. |

## Ticket states

pending → claimed → executing → awaiting_review → done. A human can move **done → awaiting_review** via REST (decision **reopened**) or **reopen_ticket** to undo a mistaken approval. Side states: blocked, needs_human, failed.

## Finishing a project

A project is **finished** when: (1) all tickets are in state **done** (no pending, claimed, executing, or awaiting_review), and (2) you have no more work to add. Set the project status to **closed** via **update_project_status** (project_id, status: "closed") so it no longer appears in the default **list_projects** and so new tickets cannot be created or claimed for it. To reopen for follow-up work, call **update_project_status** with status **active**. Optionally create a final ticket (e.g. "Project complete – [initiative name]") and approve it before closing. For initiatives tracked in docs (e.g. docs/initiatives/), update the doc or add a "Done" note when the project is finished.

## Review visibility

When you **submit_ticket**, a human reviews it. They see your **outputs** (the JSON you passed) and the **execution trace** (every **log_step** you sent). If you never call log_step, the trace is empty and the reviewer has no record of what you did—only the final outputs. Interleaving log_step as you work gives reviewers a clear picture of your actions and decisions.

## Writing traces (log_step)

Call **log_step** frequently so the execution trace is useful:

- **After significant actions**: use step_type **tool_call** and pass **payload** as an object, e.g. (name, input) for a write or run_terminal_cmd. The server accepts payload as either a JSON object or a JSON string.
- **For decisions or findings**: use step_type **observation** or **thought** with a short payload (e.g. summary text).
- **On errors**: use step_type **error** with payload describing what failed.

Without these steps, **get_trace** returns an empty list and reviewers cannot see what was done.

## Git notes after submit

When you complete work and call **submit_ticket**, if the user's repo is the project repo (or you have a repo_path), add a **git note** so the commit records what was done. Use **flywheel_add_git_note** with type **decision**, message = one-line summary of the work, and optional ticket_id/project_id. If the server cannot access the repo, the tool returns commands to run **flywheel-git note add** locally—surface those to the user or run them in the workspace. That way refs/notes/flywheel/decision (and optionally trace/intent) stay in sync with completed work.

## Work streams and Git branches

Work streams group tickets toward a goal. **Flywheel does not create a Git branch for you.** When the project has **repo_url** (Git is opted in):

**Checklist (agents often skip steps 2–3):**

1. **create_work_stream** (or pick an existing stream). Optional: set **plan** with **update_work_stream_plan**—that does **not** affect Git.
2. **Mandatory:** In the repo, create or check out the branch (see **git_instruction** in tool responses, usually feature/slug from the work stream). Run git branch --show-current if you are already on the correct branch.
3. **Mandatory:** Call **update_work_stream** with **project_id**, **work_stream_id**, and **branch** = that exact branch name. Until this succeeds, **claim_ticket** / **get_ticket** keep returning **create_or_set_branch**.
4. After **branch** is stored, responses use **checkout_branch** instead.
5. Do steps 2–3 **before** **start_ticket** for tickets with **work_stream_id** so commits land on the right branch.
6. **get_work_stream** includes **git_instruction** when **repo_url** is set—use it to verify branch is linked.

If the project has no **repo_url**, work streams are logical only—no **git_instruction** is returned.

## Reviews in conversation

The user can review work entirely in chat. When they ask **"What needs my review?"** or **"Show me pending reviews"**:

1. Call **list_pending_reviews** (project_id) for the relevant project (or for each project they care about). You get back tickets in awaiting_review.
2. For each ticket, summarize: title, objective, and **outputs**. Optionally call **get_trace** (ticket_id) and summarize key steps so they see what was done.
3. When the user says to **approve** (e.g. "Approve it", "Ship it", "Looks good"): call **approve_ticket** (ticket_id, optional notes).
4. When they say to **reject** (e.g. "Reject", "Needs changes"): call **reject_ticket** (ticket_id, notes) — notes are required so the agent knows what to fix.
</think>

## Human-in-the-loop (elicitation)

When **claim_ticket** returns "no ticket available", the client may show an **elicitation** prompt: the server lists any stuck tickets (claimed or executing) and asks the user to confirm. The user can **direct the agent to claim** by choosing a ticket ID to force-release (e.g. agent-reliability-3); the server then force-releases that ticket and retries **claim_ticket** so the agent gets the lease in this session. Alternatively, the user can say "release (ticket_id) and claim it"—the agent should call **force_release_lease** (ticket_id) then **claim_ticket** (project_id). See **docs/mcp-human-in-the-loop.md** and **docs/troubleshooting.md** (releasing a stuck lease).

## Roles: coordinator vs worker

Flywheel distinguishes two agent roles. Each session operates as **one** role—never both at once.

| Aspect | Coordinator | Worker |
|--------|------------|--------|
| **Purpose** | Translate human intent into tickets | Execute a single ticket |
| **Mutates code?** | Never | Yes (commits to worktree branch) |
| **Mutates state?** | Creates tickets + work streams only | Transitions own ticket state |
| **Tools used** | get_project_context, list_tickets, create_ticket, create_work_stream, update_ticket, update_work_stream_plan | claim_ticket, start_ticket, log_step, submit_ticket, escalate_ticket |
| **Discipline** | Read-only: no commits, no file writes, no state mutation beyond ticket creation | Write-scoped: only touches files relevant to the ticket |
| **Lifecycle** | Persistent session with the human | Ephemeral: spawned per ticket, exits after submit |

## Coordinator mode (goal decomposition)

When operating as a **coordinator**, your job is to decompose a high-level goal into a ticket DAG. You are the bridge between human intent and structured work. You **never** execute tickets—you create them and let the dispatcher assign workers.

### Coordinator discipline

- **Read-only.** You do not write code, commit to git, or mutate project state beyond creating tickets and work streams.
- **Conversational.** You chat with the human to clarify intent before creating tickets.
- **Investigative.** You read code, search the codebase, and dispatch investigation subagents to gather facts before crystallizing tickets.
- **Structured output.** Every goal becomes a ticket DAG with explicit dependencies, success criteria, and acceptance tests.

### Coordinator workflow

The coordinator follows a four-phase loop:

**Phase 1: Chat — understand the goal**

1. Read the project context pack (**get_project_context**) to learn conventions, key files, tech stack.
2. Ask clarifying questions. Confirm scope boundaries, constraints, and non-goals.
3. Identify what you already know vs what needs investigation.

**Phase 2: Investigate — gather facts (including prior history)**

4. **coordinator_get_history** (project_id) — **Always call this first.** Loads prior ticket outcomes, coordinator-feedback findings, and history stats. This is your cross-session memory: you see what was tried before, what failed, and why. Never start a coordinator session without it.
5. Read relevant files directly (source code, configs, schemas, tests).
6. Search the codebase for related patterns, existing implementations, or prior art.
7. If the investigation is complex, dispatch a subagent (investigation worker) to explore a specific area and report findings. Keep the coordinator session focused on orchestration.
8. Review dependency outputs from previously completed tickets if this goal builds on prior work.
9. **coordinator_calibration** (project_id, optional agent_id) — Check your authoring track record. If many tickets were rejected or failed, tighten acceptance criteria or decompose more granularly before authoring new work.

**Phase 3: Crystallize — design the ticket DAG**

8. Decompose the goal into discrete, independently-executable tickets. Each ticket should:
   - Have a single clear objective a worker can complete in one session.
   - List 2–5 success criteria (verifiable, not vague).
   - Include an acceptance_test (shell command) when possible.
   - Specify relevant_files so the worker knows where to look.
   - Specify constraints (e.g. "do not change public API", "backward-compatible").
9. Define dependency edges: ticket B depends_on ticket A means A must be **closed** before B is claimable.
10. Identify which tickets can be parallelized (no dependency between them) vs which must be sequential.
11. Estimate priority: P0 (critical path), P1 (high), P2 (normal), P3 (nice to have).

**Phase 4: Author — create tickets and work stream**

12. **create_work_stream** (project_id, name, optional plan) — Group all tickets under one stream.
13. **create_ticket** for each unit of work:
    - title: imperative verb phrase ("Add migration for entity table", "Implement claims registry API")
    - type: task | bug | investigation | refactor
    - priority: 0–3
    - depends_on: array of ticket IDs this ticket blocks on
    - description: full objective with success_criteria and relevant_files
14. **update_ticket** to set work_stream_id on each ticket (if not set at creation).
15. **update_work_stream_plan** with a Markdown plan showing the DAG structure, rationale, and execution order.
16. Summarize the plan to the human and ask for confirmation before they trigger the dispatcher.

### Ticket decomposition principles

- **One concern per ticket.** A ticket that says "add migration AND implement API AND write tests" is too big. Split into three.
- **Dependencies flow forward.** Schema before code, code before tests, tests before integration.
- **Workers are stateless.** Each worker sees only its ticket context + dependency outputs. Don't assume shared knowledge between workers.
- **Acceptance tests are the contract.** If you can't write a test command, the success criteria are too vague.
- **Err toward more tickets.** Small tickets complete faster, fail more cheaply, and are easier to review.

### Ticket templates

Use these patterns for common ticket types:

**Implement function/feature:**
- type: task
- success_criteria: ["function X exists in path/to/file.go", "handles edge cases Y, Z", "unit test covers happy path and error case"]
- acceptance_test: "go test ./pkg/... -run TestX"
- relevant_files: ["path/to/file.go", "path/to/file_test.go"]

**Add database migration:**
- type: task
- success_criteria: ["migration file created in db/migrations/ with timestamp prefix (YYYYMMDDHHmmss)", "up creates table/columns correctly", "down reverses cleanly"]
- acceptance_test: "go run ./cmd/migrate up && go run ./cmd/migrate down"
- relevant_files: ["db/migrations/"]
- naming: Use timestamp prefix — generate with ` + "`date -u +%Y%m%d%H%M%S`" + ` or ` + "`make migrate-create NAME=description`" + `. NEVER use sequential numbers.

**Add MCP tool:**
- type: task
- success_criteria: ["tool registered in tools.go with InputSchema", "handler returns expected JSON shape", "test covers success and error paths"]
- acceptance_test: "go test ./api/mcp/... -run TestToolName"
- relevant_files: ["api/mcp/tools.go"]

**Add/update tests:**
- type: task
- success_criteria: ["test covers cases X, Y, Z", "no test pollution (cleanup in t.Cleanup)"]
- acceptance_test: "go test ./path/... -run TestName"

**Refactor:**
- type: refactor
- success_criteria: ["old structure removed", "new structure in place", "all existing tests pass"]
- acceptance_test: "go test ./..."
- constraints: ["no behavior change", "no new dependencies"]

**Investigation (fact-finding):**
- type: investigation
- success_criteria: ["report documents findings", "lists options with trade-offs", "recommends one approach"]
- Note: investigation tickets produce knowledge, not code. Output goes into dependency_outputs for downstream tickets.

### DAG design patterns

**Linear chain:** A → B → C (migration → API → tests)

**Fan-out:** A → {B, C, D} (shared schema, then parallel feature work)

**Fan-in:** {B, C, D} → E (parallel work, then integration ticket)

**Diamond:** A → {B, C} → D (B and C depend on A; D depends on both B and C)

When designing a DAG, sketch it mentally, then verify: can each ticket be completed by a worker who only sees its own context + dependency outputs?

### Coordinator learning loop

The coordinator sharpens over time by learning from ticket outcomes. This happens automatically and via explicit tools:

**Automatic feedback capture:** When a ticket is **rejected**, **fails**, is **replanned**, or **invalidated**, the system automatically creates a coordinator-feedback finding in the findings layer. These findings include the ticket context and failure reason, tagged with categories like rejection, failure, replan, or invalidation.

**Cross-session continuity:** Every coordinator session should start with **coordinator_get_history** to load prior work context. You are not starting fresh — you inherit the project's full ticket history and feedback findings.

**Calibration self-check:** Use **coordinator_calibration** periodically to see your authoring quality metrics: success rate, rejection count, common failure categories, average attempts per close. When the calibration shows patterns (e.g. "3 rejections due to acceptance_ambiguity"), adjust your decomposition approach.

**Manual feedback recording:** Use **coordinator_record_feedback** to capture nuanced lessons that automated feedback misses. For example: "Ticket X failed because the acceptance test didn't account for the database migration ordering."

**Learning loop tools:**

| Tool | When to use |
|------|------------|
| coordinator_get_history | Start of every coordinator session — loads prior tickets and feedback |
| coordinator_calibration | Before authoring new tickets — check your track record |
| coordinator_record_feedback | After reviewing outcomes — capture specific lessons learned |

**Feedback categories:**
- **acceptance_ambiguity** — Success criteria or acceptance tests were vague or misleading.
- **scope_too_large** — Ticket tried to do too much in one unit.
- **missing_dependency** — Ticket needed work done by another ticket first but dependency was not declared.
- **wrong_decomposition** — The way work was split didn't match reality.
- **missing_context** — Worker lacked crucial information that should have been in the ticket.

## Worker mode (ticket execution)

When operating as a **worker**, you have been spawned to execute a single ticket:

1. Your system prompt contains the project context, ticket objective, success criteria, dependency outputs, and any prior attempt feedback.
2. **claim_ticket** → **start_ticket** → do the work → **log_step** frequently → **submit_ticket**.
3. If blocked, use **escalate_ticket** to ask for human help.
4. Your work happens in a **git worktree** isolated from other workers. Commit to the ticket branch.
5. If this is a retry (prior_attempts exist), read the rejection notes carefully and address every point.
6. Run the acceptance_test (if present) before submitting. If it fails, fix the issue and retry.
7. Include files_changed, build_status, and a summary in your submit outputs so the reviewer and downstream tickets have context.

The worker flow is the same as the typical flow above, but automated—no human selects tickets.

## Idempotency and retries

- **create_ticket**: Pass an optional **idempotency_key** (e.g. a UUID or deterministic key per "logical" create). Retries with the same key return the existing ticket instead of creating a duplicate. Use when the client may retry after timeouts or network errors.
- **claim_ticket**: Pass an optional **idempotency_key** (e.g. a stable key per "logical" claim, e.g. session or task ID). Retries with the same key: if the same agent still has a valid lease on the previously claimed ticket, the server renews and returns that ticket and lease; if the ticket is pending again (lease expired), the server re-claims that same ticket. Use so the same agent retrying after a timeout gets the same ticket instead of claiming another one.
- **Agent retry strategy**: Use idempotency keys for create and claim when operating in a retry-prone environment. For other tools, use **code** and **retriable** from error responses (see Error handling below).

## Error handling

Tool errors are returned as **JSON** in the error message. Parse the error string as JSON to get:

- **error** – Human-readable message.
- **code** – Stable code: lease_expired, unauthorized, forbidden, not_found, conflict, invalid_input, internal.
- **retriable** – If true, you may retry (e.g. lease_expired: try renew_lease or re-claim; not_found for "no ticket available": the client may prompt the user via elicitation, or try again later).

Use **code** to decide: lease_expired → renew or re-claim; unauthorized → ensure OAuth/sign-in; conflict → refresh ticket state; invalid_input → fix arguments. For the full list of codes and when to retry vs stop, see **docs/structured-errors.md**.

## Stuck tickets and runbook

If a ticket is **claimed** but not started (agent crashed), or you need to inspect ticket state and trace or release a stuck lease: see **docs/troubleshooting.md** → section **Tickets: agent stuck or wrong state**. Operators and agents can use that runbook to see how to inspect state (get_ticket, get_trace), when the lease expires and the ticket returns to pending, and how to force a ticket back to pending via REST (with or without the lease token).
`
