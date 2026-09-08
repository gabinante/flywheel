# AGENTS.md

This file is the shared operating guide for coding agents working in this repo.
`CLAUDE.md` should point here.

## Product frame

- Flywheel is a **local-first control plane for one operator's agentic software delivery**. It sits between
  Linear (tickets), GitHub (PRs), and the two local coding harnesses — **Claude Code** and **Codex** — and
  keeps the ticket as the contract between the human and the agents.
- **Linear is the ticket store.** A Flywheel project maps 1:1 to a Linear project the operator leads and spans
  N git repos. Flywheel keeps a projection of each issue plus what Linear cannot hold: sessions, workflow
  position, review findings, agent runs. See `docs/plans/revival-local-first.md`.
- **Code review is a separate, PR-keyed workflow with no Linear tickets.** One recipe: in-depth review,
  short, conversational, pragmatic comments with no branding/signature footer; P0/P1 → request changes, otherwise approve. Intake: pasted PR URLs, a
  `review-requested:@me` poller, a watcher that re-queues reviewed PRs on new commits or dismissed reviews,
  and a feedback watcher on the operator's own PRs that triggers an address-review-comments workflow.
- **Codex reviews, Claude Code implements** by default; the harness is selectable per project and per
  workflow phase.
- **Every Codex and Claude Code session is tracked** (interactive, dispatched, automation, subagent) and
  linked to PRs and Linear issues.
- Humans stay in the loop, but the system should advance work until it hits a real approval or judgment
  boundary.

## Repo map

- `cmd/server`: the single server binary. Serves REST, MCP (`/mcp`), and the web UI.
- `internal/`: core services.
  - `dispatch/`: CLI worker runner, `claude`/`codex`/`generic` drivers, worktrees, prompt assembly, merge loop.
  - `harness/`: headless single-turn runs of Codex (`codex exec --json`) and Claude Code (`claude -p`) with structured output.
  - `codereview/`: PR-keyed review queue, `gh`-backed GitHub client, detached review worktrees, review-requested / watch / feedback pollers.
  - `orchestrator/`: command-center planning conversation.
  - `workflow/`: workflow definitions, phases (`agent`, `gate`, `external`, `action`), engine, templates.
  - `gate/`: automated gate checkers (GitHub checks, human approval, HTTP, webhook).
  - `ticket/`, `workstream/`, `project/`, `org/`, `review/`, `execution/`, `queue/`, `agent/`, `user/`, `auth/`.
  - `linear/`: Linear as the ticket store — discovers led projects, projects issues onto tickets, files Flywheel tickets as issues, pushes state changes and comments back.
  - `report/`: Linear reporting parity — delta project status updates and the weekly roundup, rendered from Flywheel data + `gh`.
  - `sessions/`: ingests Claude Code (`~/.claude/projects`) and Codex (`~/.codex`) sessions read-only, links them to PRs and Linear issues.
  - `runstatus/`: tracks live Flywheel-managed harness invocations and publishes native session IDs as soon as available.
    Progress separates process attachment, real output recency, semantic work, and reported usage; quiet is not a hang.
    Keep event history bounded, never copy raw thinking/tool payloads into global progress, and preserve final summaries on sessions.
  - `activity/`: ephemeral SSE invalidations (`GET /api/activity/events`), fed by committed database changes and runtime progress. See `docs/activity-stream.md`.
  - `overview/`: joins live work and operator decisions across projects for the global right tray (`GET /api/overview`).
  - `progress/`: bridges ticket lifecycle events into the command center thread.
- `api/openapi.yaml`: REST contract. `api/rest/`: HTTP handlers. `api/mcp/`: MCP tools and agent guide.
- `db/migrations/`: SQL migrations.
- `web/`: Vite/React frontend (dark, operational UI).
- `scripts/varlock`: env/secrets loader and validator.

## Operator settings

- Linear API key/sync, code review (harness, publish, watchers), PR-feedback auto-address, report posting, dispatch
  (on/off, concurrency, driver) and harness defaults (binary, model, effort per harness) are edited in the UI at
  `/settings` and stored in Postgres (`operator_settings`, one JSONB row). The dispatcher's worker MCP key is minted
  once and kept there too. The matching env vars
  are only defaults for a fresh install; after the first save the stored row wins, and changes apply live.
- `internal/settings` owns the model; services expose `Apply`/`Reconfigure` and are wired in `cmd/server/main.go`.
- Built-in agent prompts (code reviewer, feedback addresser, orchestrator, dispatch worker types, ticket reviewer,
  conflict resolver) live in `internal/prompts`: each service registers its default at init and reads
  `prompts.Text(id)` at run time; overrides are edited at Settings → Prompts (`GET/PUT /prompts`) and stored in
  operator settings. Do not hard-code a role preamble in a new service — register it.
- There is a single organization. It is not shown as a navigation level: `/orgs` forwards to its project list and
  projects are the main separation.
- The right tray is global and consistent on every page: in-flight work with ticket/session links, and items
  needing operator action. Project-specific context stays in the page. Active work comes from managed runs,
  dispatcher reservations, and external/review workflow state, not recently modified session files.
  Dispatch and reviewer capacity are independent. Show each active worker's session, link review PR titles/references
  directly to GitHub, and keep review details accessible separately. Queued reviews are waiting, not active workers.
- UI activity uses one shared `ActivityProvider`: scoped query invalidation or `useActivityVersion` for existing read effects. Preserve local drafts; do not add per-page SSE connections or polling loops. New writers must emit a committed topic or runtime notification.
- Explicit GitHub review re-requests are deduplicated by timeline event, independently of commit changes.
  Pending requests survive active attempts and restart; manual stops consume outstanding demand and block future automatic reviews until manually resumed.
  My Reviews caches GitHub facts but overlays current queue state on every read. Show recommendations
  separately from confirmed GitHub publication, and clear old session links when starting a new attempt.
  Review head changes schedule a fresh attempt; transient pre-publication failures retry after 30 seconds,
  2 minutes, and 10 minutes. Retry deadlines/budgets persist across restart; stop cancels pending retries.
  Never automatically replay a GitHub submission with an ambiguous outcome.

## Local dev

- Ports are **non-default on purpose** so Flywheel never collides with other local stacks:
  server **8090**, Postgres **5439**, Redis **6389**.
- `make dev` — the primary dev command. Ensures Docker Postgres+Redis are running (compose project
  `flywheel`), runs migrations, then starts the Go server on `:8090`. Re-invoke to restart.
- `make dev-infra` — start only Postgres and Redis. `make dev-stop` — kill the server only.
- `scripts/test-hardening.sh` — build the server and web app, then run database regressions and Playwright with disposable Docker databases and fake local harnesses on port 8091. Requires Docker, web dependencies, and Playwright Chromium.
- `make test` — Go tests. `make web-build` — production web build. `make generate` — regenerate from
  OpenAPI (requires `oapi-codegen`; `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`).
- `cd web && npm run dev` — Vite dev server (HMR); `npm run gen:api` after OpenAPI changes.
- `make varlock-validate` — validate `.env` against `.env.schema`.
- Opening `/` enters the local workspace directly. Server startup provisions the persistent local operator
  and default organization. There is no homepage, login, browser token, or external identity provider.

## MCP setup

Flywheel exposes MCP at `http://localhost:8090/mcp` (Streamable HTTP). Claude Code and Codex use the same
endpoint with an API key (`DISPATCH_API_KEY`, or an agent key from `POST /agents`).

Claude Code (`~/.claude/.mcp.json` or the repo's `.claude/settings.json`):
```json
{ "mcpServers": { "flywheel": { "url": "http://localhost:8090/mcp", "headers": { "X-API-Key": "<key>" } } } }
```

Dispatched workers receive a short-lived, ticket/phase/role-scoped credential automatically. Credential files live outside checkouts with mode 0600. Operator REST controls use the startup-provisioned local identity without browser credentials; the server binds to loopback and rejects untrusted Host/Origin headers. MCP keys authenticate separately and do not inherit the REST operator context. After restarting the server, MCP clients reconnect;
if tools error after a restart, start a new session.

## Platform rules

- Tickets are the unit of work and the contract between agents; the Linear issue is the ticket.
- Work streams group tickets into an initiative; they are not a substitute for the ticket dependency DAG.
- PRs and reviews use GitHub as the source of truth (via the authenticated `gh` CLI), never Flywheel's DB.
- Command-center orchestration is planning-only; implementation belongs to dispatch workers.
- When touching orchestrator UI or API, preserve both conversation `messages` and planner `runs/events`.
- Keep harness selection modular: a workflow phase names a harness (`claude`, `codex`), never a vendor API.

## Delivery workflow

- Flywheel is in active revival. Optimize for getting working code merged to `main`.
- A feature is not done until it is merged to `main`; a pushed branch or open PR is not completion.
- Unless a task explicitly requires a separate branch or PR-based handoff, take the shortest path to landing
  validated changes on `main`.

## Dispatch config

- Runner is `cli` only. Drivers: `claude` (default), `codex`, `generic`.
- Worktrees follow the operator's layout: `<DISPATCH_WORKTREE_DIR>/<repo>-worktrees/<slug>` (default root `~/git`), cut from
  the operator's own checkout at `<root>/<repo>` when it exists. Linear-backed tickets branch as `<identifier>-<title-slug>`
  (e.g. `rlep-3488-review-fixes`). Review worktrees are `review-<n>-<unique>`, feedback worktrees `feedback-<n>-<unique>`. Flywheel records ownership outside each checkout and removes only owned, clean worktrees. Dirty or unknown directories are preserved.
- `DISPATCH_AGENT_MODEL` / `DISPATCH_AGENT_REASONING_EFFORT` are passed to harnesses that accept them.
- Orchestrator and dispatch worker configs can differ (`ORCHESTRATOR_AGENT_*`).

## Coding rules

- Prefer integration over rebuilding commodity systems (Linear, GitHub, `gh`, the harnesses' own stores).
- Keep schemas additive. Avoid breaking the REST or MCP contracts.
- Do not read `.env` directly in application code. Use `config` and a varlock-loaded environment.
- Dark mode first in the web UI. The UI language is glassy and operational, not marketing-style.
- Avoid broad refactors unless they directly serve the task.

## Migrations

- Existing legacy migrations are numbered. New migrations: `make migrate-create NAME=...` (timestamped).
- Do not renumber or rewrite old migrations.

## When to flag a human

- Public schema or MCP contract changes.
- New privileged surface or secret-handling changes.
- Anything that posts to GitHub or Linear on the operator's behalf in a new way.
- Anything that drifts toward Flywheel replacing Git hosting, CI, observability, or identity.
