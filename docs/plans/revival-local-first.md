# Flywheel revival — local-first control plane for a Linear + Codex + Claude Code workflow

Status: phases 0–5 implemented 2026-09-04 (see git history); live Linear posting awaits `LINEAR_API_KEY`. Phase 6 items remain open.

## 1. Why

Flywheel was built as a multi-tenant "work queue plus shared context" where Flywheel owned the tickets and
agents were interchangeable CLI harnesses. The operator's real workflow has moved on:

- **Linear is the ticket store.** Tickets live in Linear projects the operator leads (RLETD / RLEP / RLE2 teams).
  Two Claude skills (`weekly-roundup`, `project-status`) and an identical `gh pr create` hook in both
  `~/.claude/settings.json` and `~/.codex/hooks.json` keep GitHub and Linear reconciled by hand.
- **Codex reviews, Claude Code implements.** Of ~187 Codex threads since 2026-07-27, ~85% are PR reviews
  (100 `gh pr review` calls, ~95 approvals, 3 request-changes, inline comments via `gh api pulls/N/comments`).
  Claude Code sessions (~130) are "address feedback", "fix merge conflicts", "get it to green", "make a PR",
  investigations, and planning, run in `~/git/<repo>-worktrees/<slug>`.
- **Reviews are PR-keyed, not ticket-keyed.** Almost no review has a Linear issue, and the operator does not want one.
- **Sessions are the unit of visibility.** Both harnesses keep complete local transcripts
  (`~/.claude/projects/**.jsonl`, `~/.codex/state_5.sqlite` + rollout JSONL) that nothing aggregates.

The revival turns Flywheel into a single-operator, local-first control plane over that workflow.

## 2. Goals and non-goals

Goals

1. Linear is the source of truth for tickets. Flywheel projects map 1:1 to Linear projects the operator leads,
   each spanning N git repos. Flywheel files tickets for implementation work it drives and posts project
   status updates — parity with what the `project-status` / `weekly-roundup` skills and the PR hook do today.
2. Code review is a separate, PR-keyed workflow with **no Linear tickets**. One recipe for now:
   in-depth review, inline conversational comments, `P1+ → request changes`, otherwise approve.
   Intake: pasted PR URLs, a `review-requested:@me` poller, and a watcher that re-queues reviewed PRs
   when new commits land or the operator's review is dismissed. Queue fan-out is just N review units
   created by the orchestrator; the dispatcher provides the parallelism.
3. Codex is a first-class harness beside Claude Code: a dispatch driver (`codex exec --json`) and a tracked
   interactive harness. Default: Codex for review phases, Claude Code for implementation phases —
   selectable per project and per workflow phase.
4. Every Codex and Claude Code session (interactive, dispatched, automation, subagent) is tracked and linked
   to PRs and Linear issues.
5. Local-first: one operator, `AUTH_DEV_BYPASS` identity, non-default ports (no collisions with joinera dev),
   `make dev` is the whole story.

Non-goals / removals

- GitHub OAuth login, JWT sessions, org invites, multi-org UI.
- Fly.io and any infrastructure/deployment provider surface; environments/pipelines pages.
- Steampipe state index, Weaviate findings store, Jira mirror, Slack/email/SMS notifications.
- Policy posture / pillars / risk / rollback / cost budgets / claims / catalog / code intel / observation
  streams / git notes / project templates / TUI. These stay in git history; they are not part of the
  operator's workflow.
- Docker worker isolation (the operator runs trusted local harnesses).

## 3. Domain model

### 3.1 Ticket = Linear issue

`tickets` gains an external reference and becomes a projection of Linear:

| field | source |
|---|---|
| `external_provider` = `linear`, `external_id`, `external_identifier` (e.g. `RLETD-465`), `external_url` | Linear |
| `linear_team_key`, `linear_project_id`, `linear_state_name`, `linear_state_type`, `linear_updated_at` | Linear |
| `state` (draft / planning / executing / awaiting_validation / validated / closed) | derived from `linear_state_type` + PR state via per-team mapping |
| workflow position, sessions, PR links, plans | Flywheel only |

- **Inbound**: a poller (60 s) runs `issues(filter: {project: {id: {in: …}}, updatedAt: {gt: $since}})` per in-scope
  project and upserts. Scope = Linear projects where `lead == me` (discovered at startup, overridable in settings).
- **Outbound**: `issueCreate`, `issueUpdate` (state, assignee), `commentCreate`, `attachmentLinkGitHubPR`,
  `projectUpdateCreate`. Every description / work-summary comment starts with the Abstract block
  (Abstract, Components, Before, After) exactly as the global CLAUDE.md requires.
- Flywheel project ↔ Linear project (1:1). Work stream ↔ Linear milestone (optional). Single implicit org.
- The existing `internal/mirror/linear` GraphQL client is the seed; `mirror` ("Flywheel is source of truth,
  one-way") is replaced by `internal/linear` (two-way, Linear is source of truth).
- Interactive sessions keep using the Linear MCP connector to file tickets ("crack out a linear ticket");
  Flywheel ingests them on the next poll. Nothing about the operator's ad-hoc flow changes.

### 3.2 ReviewRequest (new, PR-keyed)

```
review_requests
  id, repo (owner/name), number, url, title, author, base_ref, head_sha,
  origin        paste | review_requested | re_review
  recipe        inline_conversational_p1_gate        (only one today; column exists for later)
  harness       codex | claude                        (default from project/system settings)
  state         queued | fetching | reviewing | publishing | approved | changes_requested |
                watching | superseded | closed | failed
  my_review_state, my_review_id        (GitHub: APPROVED / CHANGES_REQUESTED / DISMISSED)
  last_reviewed_head_sha, last_checked_at, watch bool
  ticket_id NULL                                      (optional; never auto-created)
review_findings
  id, review_request_id, severity P0..P3, path, line, side, body (conversational),
  github_comment_id, status posted | resolved | outdated | withheld
```

Workflow template **PR Review** (phases):

1. `fetch` (action) — `gh pr view --json …`, fetch head, create a detached worktree at `head_sha` under
   `~/git/<repo>-worktrees/review-<number>` (reviews stop running in the primary checkout on whatever branch
   happens to be there).
2. `review` (agent, harness=codex by default, worker type `reviewer`) — reads the diff and enough surrounding
   code, produces findings via MCP `report_finding` and a summary via `submit_review`. Read-only sandbox.
3. `publish` (action) — Flywheel posts **one** GitHub review: inline comments for each finding (conversational
   body text written by the agent) + event `REQUEST_CHANGES` if any P0/P1, else `APPROVE`, with a short
   summary. Idempotent; records `github_comment_id`s.
4. `watch` (external) — leaves the request in `watching`; the poller re-queues it as `re_review` when
   `head_sha` changes or the operator's review is dismissed, passing prior findings as context.

Intake

- **Paste**: a Review page input and a command-center intent accept one or many PR URLs → N `review_requests`.
- **Poller** (2 min): `gh search prs --review-requested=@me --state=open` → new requests.
- **Watcher** (2 min): for `watching` requests, compare `headRefOid` and my latest review state.
- **Feedback watcher** (2 min): for open PRs *authored by me* (`gh search prs --author=@me --state=open`), detect
  reviews that landed since the last check — new review comments, `CHANGES_REQUESTED`, bot reviews (Bugbot, CI
  annotations). Each new batch creates a `feedback_rounds` row on the PR's implementation ticket and triggers the
  **address_feedback** phase of the Implement workflow (§3.5): a Claude session in the PR's worktree that resolves
  the comments, replies on each thread, pushes, and re-requests review. This is the "address feedback on PR / check
  bugbot feedback" loop the operator runs by hand today.

GitHub access goes through the already-authenticated `gh` CLI (search, view, diff, review posting), the same
way Codex does it today. No PAT management.

### 3.3 AgentSession (new)

```
agent_sessions
  id, harness claude_code | codex, external_id (Claude sessionId / Codex thread id),
  origin interactive | dispatched | automation | subagent, parent_session_id,
  cwd, repo (from git origin), branch, model, reasoning_effort,
  title, first_prompt, transcript_path,
  tokens_in, tokens_out, started_at, last_activity_at, ended_at, status active | idle | ended
session_links (session_id, kind ticket | review | pr | linear_issue, ref)
session_prompts (session_id, seq, role, text, ts)  -- tsvector index for search
```

Ingestion (`internal/sessions/collector`, a goroutine in the server; no separate daemon):

- **Claude Code**: poll `~/.claude/projects/**/*.jsonl` mtimes (5 s), parse appended lines from a stored byte
  offset. Metadata from `cwd`, `gitBranch`, `version`, `ai-title`, `pr-link` lines, `usage` on assistant
  messages; user prompts for search and ref extraction. `<session>/subagents/*.jsonl` → child sessions.
- **Codex**: read `~/.codex/state_5.sqlite` `threads` read-only (`modernc.org/sqlite` is already a dependency;
  SQLite WAL permits concurrent readers). Keep `source='vscode'` threads with `thread_source in (user, subagent)`;
  drop guardian / `codex-auto-review` threads by default. `thread_spawn_edges` → parents. Rollout JSONL
  (`rollout_path`) is parsed lazily for prompts and tool calls.
- **Dispatched**: the dispatcher records the session at spawn — Claude via `--session-id`/stream-json `init`,
  Codex via the `thread.started` event in `--json` output — so linkage is exact.
- **Linking rules**: branch ↔ open PR head ref; PR URLs / `owner/repo#N` in prompts; `[A-Z]+-\d+` Linear
  identifiers in prompts and branch names (`rlep-3488-review-fixes`); worktree path → repo; plus an explicit
  `link_session` MCP tool.
- Later: Claude `SessionStart`/`Stop`/`UserPromptSubmit` hooks and Codex `notify` posting to `/hooks/*` for
  real-time status; Flywheel can write those hook configs during setup.

### 3.4 Codex as a driver

`internal/dispatch/driver_codex.go` implementing `AgentDriver`:

- `codex exec --json -C <workdir> -m <model> -c model_reasoning_effort=<effort> -c approval_policy=never
  -s <read-only|workspace-write> --output-schema <schema> -o <last-message>` with the task prompt on stdin.
- Flywheel MCP injected per run: `-c mcp_servers.flywheel.url=http://localhost:<PORT>/mcp`
  plus the API-key header override (config.toml already shows URL-based servers work).
- System prompt delivery: Codex has no `--system-prompt`; use the documented instructions override
  (`-c model_instructions_file=…` or equivalent for the installed version) and fall back to a prompt preamble.
  **Verify against the installed `codex` version during Phase 3.**
- `--json` JSONL events → `log_step` entries and the session record; `thread.started` gives the thread id.
- `codex exec review` exists as a native reviewer; evaluated in Phase 3 as an alternative to a prompt-driven
  review, but the publish step stays Flywheel-owned either way.

Harness selection: system default (`codex` for `reviewer`, `claude` for everything else), overridable per project
and per workflow phase (`phase.config.harness`, `.model`, `.effort`).

### 3.5 Implementation workflow on Linear tickets

Template **Implement** (phases): `plan` (agent, claude) → `implement` (agent, claude, worktree
`~/git/<repo>-worktrees/<identifier>-<slug>`) → `open_pr` (action: `gh pr create`, Abstract body,
Linear identifier in body) → `linear_sync` (action: attach PR, Abstract comment, move to In Review) →
`land` (agent loop: fix CI, resolve conflicts — the "get it to green" loop; re-entered by the feedback
watcher as `address_feedback` whenever a review lands) → `merged` (gate: PR merged → Linear Done). Worktrees are removed after merge.

### 3.6 Linear reporting parity

- **Ticket filing**: when a tracked or dispatched session opens a PR with no linked Linear issue, Flywheel creates
  the issue in the ticket's project, attaches the PR, and posts the Abstract comment (what the hook does now,
  but deduplicated and harness-agnostic).
- **Project status updates**: scheduled job (2-day cadence, per led project) posting a `projectUpdate` delta —
  merged/closed since last update, merge-queue state, blockers.
- **Weekly roundup**: Linear document refresh, same content as the skill. Later phase.

## 4. Runtime and configuration

- Ports (chosen to avoid joinera local dev: 5432, 6378–6383, 8080, 3000/3003, 30030, 7233/8233, 50051,
  16378–16382): **server 8090, Postgres 5439, Redis 6389**. `make dev` / `dev-stop` parameterized on `PORT`.
- `docker-compose.yml`: project `flywheel`, `postgres` + `redis` only (server runs natively).
- `.env.schema` (varlock): `PORT`, `DATABASE_URL`, `REDIS_URL`, `AUTH_DEV_BYPASS`, `LINEAR_API_KEY`,
  `LINEAR_PROJECT_IDS` (optional; default = projects I lead), `GITHUB_LOGIN` (default from `gh api user`),
  `DISPATCH_*`, `SESSIONS_CLAUDE_DIR`, `SESSIONS_CODEX_DIR`, `WORKTREE_ROOT=~/git`. OAuth / Fly / Steampipe /
  Weaviate / notification / Jira keys removed.
- Redis stays for queue leases and the bus for now; moving leases and the bus fully onto Postgres is a later
  simplification.

## 5. UI (dark, operational)

- **Command Center**: live sessions, review queue with states, my tickets by Linear project, needs-attention.
- **Reviews**: queue + detail (findings, posted comments, session transcript link, re-review / open PR).
- **Tickets**: by Linear project; detail shows Linear fields, linked PRs, sessions, workflow position.
- **Sessions**: list/timeline filtered by harness, repo, branch, origin; detail with prompts and tool-call
  summary; full-text search over prompts.
- **Settings**: Linear projects in scope, repos per project, harness defaults, collector status.
- Removed: orgs, invites, infrastructure, policy health, entity detail, usage (folded into sessions).

## 6. Phases (each phase lands on `main` before the next starts)

0. **Foundation** — real git clone + worktree; ports; boot on `AUTH_DEV_BYPASS` only; remove OAuth / Fly /
   Steampipe / Weaviate / Jira / notifications / policy / pillars / etc. from wiring, config schema, and UI
   nav; single implicit org; `make dev`, web build, and `make test` green. Dead-package deletion as a
   follow-up PR in the same phase.
1. **Sessions** — schema, collector (Claude + Codex + subagents), ref extraction, Sessions UI, search.
2. **Linear as ticket store** — `internal/linear`, project scope discovery, incremental poller, outbound
   writes, ticket projection, Tickets UI on Linear identifiers, session ↔ ticket linking.
3. **Review mode** — `review_requests` + `review_findings`, PR Review workflow, `reviewer` worker type +
   MCP tools (`get_review_target`, `report_finding`, `submit_review`), Codex driver, `gh` publisher, paste
   intake, review-requested poller, watch / re-review loop, Reviews UI.
4. **Implementation workflow** — Implement template, worktree convention, PR → Linear filing, land loop,
   feedback watcher → `address_feedback` dispatch, merge → Done.
5. **Reporting parity** — project status job, weekly roundup, usage roll-ups.
6. Later — real-time hooks, "resume session" actions (`claude --resume`, `codex resume`), Redis removal,
   `codex exec review` evaluation.

## 7. Open items to verify while building

- Codex: instructions-override flag for the installed version; MCP header override key; `thread.started`
  event shape in `--json`.
- Claude Code: `--session-id` acceptance in `--print` mode vs reading `session_id` from the `init` message.
- `gh search prs --review-requested=@me` result fields vs a GraphQL `search(type: ISSUE)` query.
- Linear: `attachmentLinkGitHubPR` availability on the workspace; per-team state name mapping.
