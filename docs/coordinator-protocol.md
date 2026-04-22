# Coordinator protocol

The coordinator is a Claude session that translates human intent into structured ticket DAGs. It is **Layer 1** of the Flywheel spec (v0.2). This document defines the protocol, workflow, and discipline.

## Role distinction

| Aspect | Coordinator | Worker |
|--------|------------|--------|
| Purpose | Translate human intent into tickets | Execute a single ticket |
| Mutates code? | Never | Yes (commits to worktree branch) |
| Mutates state? | Creates tickets + work streams only | Transitions own ticket state |
| Tools | get_project_context, list_tickets, create_ticket, create_work_stream, update_ticket, update_work_stream_plan | claim_ticket, start_ticket, log_step, submit_ticket, escalate_ticket |
| Discipline | Read-only: no commits, no file writes, no state mutation beyond ticket creation | Write-scoped: only touches files relevant to the ticket |
| Lifecycle | Persistent session with the human | Ephemeral: spawned per ticket, exits after submit |

## Coordinator discipline

The coordinator is **read-only by design**:

1. **No commits.** The coordinator never writes files or commits to git.
2. **No state mutation.** The coordinator does not claim, start, or submit tickets. It creates them.
3. **No direct execution.** The coordinator does not run builds, tests, or deployments.
4. **Conversational authority.** The coordinator talks to the human. Workers do not.
5. **Investigation dispatch.** For complex fact-finding, the coordinator dispatches subagent investigators (read-only sessions that report back).

## Workflow: chat -> investigate -> crystallize -> author

### Phase 1: Chat (understand the goal)

The coordinator starts by understanding what the human wants:

```
Human: "I want to add entity identity tracking across all layers"

Coordinator actions:
1. get_project_context(project_id) - load conventions, key files, tech stack
2. Ask clarifying questions:
   - "What entities need tracking? Just code objects, or infra too?"
   - "Should IDs be UUIDs or content-addressed hashes?"
   - "What layers exist today that need the ID plumbing?"
3. Confirm scope boundaries and non-goals
```

Key questions to ask:
- What is the desired end state?
- What are the constraints and non-goals?
- What existing code or systems does this touch?
- What is the priority and timeline?

### Phase 2: Investigate (gather facts)

Before designing tickets, the coordinator gathers facts:

```
Coordinator actions:
1. Read relevant source files (models, schemas, existing implementations)
2. Search for related patterns ("entity", "identity", "ID" in codebase)
3. Review dependency outputs from completed tickets
4. List existing tickets to avoid duplicate work
5. For complex areas, note findings for the human
```

Tools used:
- **get_project_context** - conventions, key files
- **list_tickets** - see what exists and what's done
- Direct file reading and codebase search (via the agent's own tools)

### Phase 3: Crystallize (design the ticket DAG)

The coordinator designs the work breakdown:

```
Coordinator thinking:
- "Entity identity needs: (1) a model + migration, (2) a registry service,
  (3) integration with ticket model, (4) integration with change events,
  (5) MCP tool for entity lookup, (6) tests for each layer"
- "Dependencies: 1 before 2, 2 before {3,4}, {3,4} before 5, 5 before 6"
- "Tickets 3 and 4 can parallelize since they touch different files"
```

Decomposition principles:
- **One concern per ticket.** "Add migration AND implement API" is two tickets.
- **Dependencies flow forward.** Schema -> code -> tests -> integration.
- **Workers are stateless.** Each worker sees only its ticket + dependency outputs.
- **Acceptance tests are the contract.** If you can't write a shell test command, criteria are too vague.
- **Err toward more tickets.** Small tickets complete faster and fail cheaply.

### Phase 4: Author (create tickets and work stream)

The coordinator creates the structured work:

```
Coordinator actions:
1. create_work_stream(project_id, name="Entity Identity System")
2. create_ticket(project_id, title="Add entity model and migration",
     type="task", priority=1,
     description="Create Entity model with ID, type, source fields...",
     success_criteria=["migration creates entities table", "model in internal/entity/"],
     acceptance_test="go test ./internal/entity/... -run TestEntityModel")
3. create_ticket(..., title="Implement entity registry service",
     depends_on=["warrant-XX"],  // depends on the migration ticket
     ...)
4. ... (more tickets)
5. update_work_stream_plan(work_stream_id, plan="## Entity Identity\n\n...")
6. Summarize plan to human, ask for confirmation
```

## DAG design patterns

### Linear chain
```
A -> B -> C
```
Use when each step depends on the previous. Example: migration -> API handler -> tests.

### Fan-out
```
A -> {B, C, D}
```
Use when one foundation enables parallel work. Example: schema migration enables parallel feature implementations.

### Fan-in
```
{B, C, D} -> E
```
Use when parallel work must converge. Example: parallel features -> integration test ticket.

### Diamond
```
A -> {B, C} -> D
```
Combines fan-out and fan-in. B and C both need A; D needs both B and C.

## Ticket quality checklist

Before creating each ticket, verify:

- [ ] **Title** is an imperative verb phrase ("Add X", "Implement Y", "Fix Z")
- [ ] **Type** is one of: task, bug, investigation, refactor
- [ ] **Priority** reflects actual execution order needs (0=critical path, 1=high, 2=normal, 3=nice-to-have)
- [ ] **Success criteria** are 2-5 verifiable statements (not vague)
- [ ] **Acceptance test** is a shell command when possible
- [ ] **Relevant files** point workers to the right code
- [ ] **Constraints** list what NOT to do
- [ ] **depends_on** correctly reflects the DAG edges
- [ ] A worker with only this ticket's context can complete the work

## Example coordinator session

Below is an annotated example of a coordinator session decomposing a natural language goal into tickets.

### Human goal

> "I want to add a claims registry so agents can declare what entities they're working on, preventing conflicts."

### Coordinator response

**Phase 1 — Chat:**

"Let me understand the scope. A claims registry would let agents declare intent to modify specific entities (files, config keys, etc.) so other agents can check for conflicts before starting work. A few questions:

1. Should claims be advisory (warn on conflict) or mandatory (block on conflict)?
2. What's the claim granularity — file-level, entity-level, or arbitrary string keys?
3. Should claims auto-expire with the ticket lease, or have independent TTLs?
4. Does this need a UI, or is it MCP-tool-only for now?"

**Phase 2 — Investigate:**

After the human answers (mandatory, entity-level, expire with lease, MCP-only for now), the coordinator reads:
- `internal/queue/service.go` — lease mechanics to understand TTL integration
- `internal/ticket/model.go` — ticket structure for assigned_to field
- `api/mcp/tools.go` — existing tool patterns for the new MCP tools

**Phase 3 — Crystallize:**

"Here's my proposed breakdown:

```
warrant-21: Add claims table migration
    |
    v
warrant-22: Implement claims service (acquire, release, check)
    |
    +---> warrant-23: Add MCP tools (acquire_claim, release_claim, check_claims)
    |
    +---> warrant-24: Integrate claim check into ticket start_ticket guard
    |
    v
warrant-25: Add claims integration tests
```

Tickets 23 and 24 can run in parallel since they touch different files (api/mcp/ vs internal/queue/). Ticket 25 fans in from both."

**Phase 4 — Author:**

The coordinator then calls:
1. `create_work_stream(project_id, name="Claims Registry")`
2. `create_ticket(title="Add claims table migration", priority=1, success_criteria=["migration creates claims table with entity_key, agent_id, ticket_id, expires_at", "down migration drops table cleanly"], acceptance_test="make migrate-up && make migrate-down")`
3. `create_ticket(title="Implement claims service", depends_on=["warrant-21"], ...)`
4. `create_ticket(title="Add MCP tools for claims", depends_on=["warrant-22"], ...)`
5. `create_ticket(title="Integrate claim check into start_ticket guard", depends_on=["warrant-22"], ...)`
6. `create_ticket(title="Add claims integration tests", depends_on=["warrant-23", "warrant-24"], ...)`
7. `update_work_stream_plan(...)` with the DAG diagram and rationale.

## System prompt location

The coordinator system prompt is stored in `context_pack.system_prompt` on the project. Load it with `get_project_context(project_id)`. Update it with `update_project_context(project_id, system_prompt=...)`.

When the dispatcher spawns a coordinator session, it injects `context_pack.system_prompt` into the Claude session's system prompt along with the project conventions and key files.
