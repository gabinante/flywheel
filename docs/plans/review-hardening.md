# Flywheel review fixes

Implemented directly on `main` following the September 4 review of
`d7952ba3bea07da7e5e46bcf6daf82a740bb7fa9`. These changes retain the local-first
product and existing service boundaries while repairing the 23 concrete findings.

## Local metaharness entry

The marketing homepage and browser login flow have been removed. Opening `/` enters the
project workspace. Startup provisions the same persistent local operator and default org,
and browser requests use a stable API client without JWT storage or authentication guards.
Local Host/Origin checks and independent MCP credentials remain. Manually registered keys
are linked to the local workspace; run-scoped harness permissions are unchanged.
Migration `20260904230000_local_harness_agents` permits multiple MCP registrations
while preserving one operator identity. `make dev` applies it with the other migrations.
Validation used disposable databases; the operator database was not modified.

## Findings addressed

- [x] **1 — Workspace ownership.** Unique review/feedback directories and ownership records outside Git. Cleanup refuses dirty or unowned workspaces. Repository changes preserve old clones instead of deleting them.
- [x] **2 — Credentials.** Worker MCP configuration is temporary, mode 0600, and outside the checkout. Codex receives only a temporary run key.
- [x] **3 — Exact claims.** Dispatch reserves its selected ticket before starting a harness. MCP claims are forced to that ticket. Sequential phases clear the prior assignment and create a fresh lease.
- [x] **4 — Workflow authority.** Workflow tickets follow the pinned phase definition instead of the legacy state-driven dispatch/merge scans. Merge is an explicit `merge_pr` action. The next worker starts after its predecessor releases its slot.
- [x] **5 — Local trust boundary.** Loopback listener, Host/Origin checks, automatic local operator identity for REST controls, and enforced MCP run capabilities. Planners can edit project planning data but cannot claim, submit, or approve implementation work. Executors cannot approve themselves.
- [x] **6 — CI gates.** Structured GitHub checks distinguish passed, pending, failed, and unavailable. Unknown or absent checks do not permit merging. The observed head is checked again before a merge with `--match-head-commit`.
- [x] **7 — Durable events.** Pending deliveries are recovered per subscriber, claimed, retried, and ordered by entity. Ticket mutations and outgoing events use one database transaction; ignored publication errors still abort that transaction. Linear handlers finish before acknowledgement.
- [x] **8 — Failure behavior.** Failures stop or follow an explicit failure edge. Exhausted review/merge attempts require intervention. Retry starts a new phase attempt; a failed attempt cannot be completed by a late callback.
- [x] **9 — Configuration races.** Dispatcher configuration and worker routing are synchronized. The shared mutable reconciliation cache was removed. Race tests pass.
- [x] **10 — Feedback batches.** Database claims serialize write runs per PR. Completion updates only the claimed batch; later reviews remain pending. Runs use the server cancellation lifecycle and global execution capacity.
- [x] **11 — Restart recovery.** Interrupted reviews become visibly failed and can be rerun after inspecting GitHub. Interrupted feedback becomes pending. Interrupted running workflow phases require explicit retry. A database advisory lock prevents two servers from owning the same database.
- [x] **12 — Repository identity.** Primary checkouts must match the complete repository identity. Dispatch branches start from a fetched default branch; review and feedback checkouts use fetched PR heads, including fork repositories. Review publication rechecks the observed head.
- [x] **13 — Linear synchronization.** The off switch gates outbound operations as well as polling. Outbound handlers serialize and reconcile current ticket state. Durable retries and stable issue/comment identities cover uncertain create responses.
- [x] **14 — External intake.** Draft Linear imports resolve and pin the project workflow. Already active or completed imports retain a deliberate legacy lifecycle rather than retroactively starting automation.
- [x] **15 — Settings ownership.** Operational saves preserve prompt overrides and layout. Settings snapshots and listener inputs are copied; partial updates are serialized.
- [x] **16 — Current findings.** Review queries select findings for the request's current attempt, including a clean attempt with zero findings.
- [x] **17 — Phase contracts.** Sync, async, and polling HTTP phases execute; webhook gates expose callback URLs; human gates offer approval; manual agent continuation is honored. The editor offers the supported merge action. Missing external URLs and unsupported actions are rejected on save.
- [x] **18 — Execution limits.** One process budget covers dispatch, planning, reviews, feedback, and conversations, in addition to project dispatch limits. Waiting can be cancelled.
- [x] **19 — Harness contract.** Phase harness/model/effort overrides reach invocation. Claude and Codex output is normalized, incomplete/failed turns fail, native session IDs are preserved, and cancellation terminates process groups.
- [x] **20 — Planner context.** Planning resolves the selected project's checkout or scratch directory and applies the shared worker library live. Planning and validation use restricted harness modes and role-scoped MCP capabilities.
- [x] **21 — Sessions.** Session, prompt, link, and ingestion cursor writes commit together. Collection errors propagate. Dispatched runs record native identity directly; continuation checks its directory and reports harness failures.
- [x] **22 — Atomic attempts.** A ticket lock protects phase completion/history and terminal ticket closure. Callbacks capture definition version and phase entry time, consume their token only on commit, and reject stale/replayed attempts. Workflow edits snapshot versions transactionally.
- [x] **23 — Project views.** Review, feedback, and session queries scope by project before pagination. Reviews expose pagination; sessions refresh. The ticket API includes workflow fields so approval/retry/continue controls actually render.

## Verification

Final results: the complete Go suite and targeted race suite pass; frontend lint and
all three frontend unit tests pass; all ten database regressions and all thirteen
Playwright scenarios pass against a fresh migrated database and production build.
The test script removes its disposable containers and server after completion.

The global right tray is consistent across routes. It shows in-flight ticket/PR work,
native session links, and decisions needing operator input. Managed harnesses expose
last-output time, activity counters, reported token usage, and a bounded progress timeline.
Quiet workers and orphaned review records are distinguished, and completed sessions retain
their progress summary. The reviewer prompt explicitly recommends approval without P0/P1
findings and keeps P2/P3 feedback nonblocking, matching the existing verdict logic.

The regression suite uses local Git repositories, a fake GitHub command, disposable
PostgreSQL/Redis databases, and deterministic Claude/Codex protocol fixtures. It does
not publish reviews, push commits, merge PRs, post Linear messages, or spend model tokens.

```sh
go test -timeout 120s ./...
go test -race -timeout 120s ./internal/dispatch ./internal/orchestrator ./internal/workflow ./events ./internal/settings ./internal/runlimit ./internal/queue ./internal/linear
cd web
npm run lint
npm test
cd ..
scripts/test-hardening.sh
```

The final command applies all migrations to a fresh PostgreSQL database, runs ten
cross-component database regressions, builds the Go server and production web bundle,
and runs thirteen Playwright scenarios in Chromium. It prints the retained log directory
and removes its own containers/server on exit. Install frontend dependencies with
`npm ci` and browser binaries with `npx playwright install chromium` from `web` first.
Port 8091 must be free. The operator's database and session directories are not used.

Browser coverage includes direct entry without login, manual MCP registration, local browser boundaries, 105 project reviews across two
pages, clean-attempt findings, session scoping and refresh, approval/stale decisions,
failed-phase retry, manual continuation, prompt preservation, navigation, and two
sequential harness phases ending at a human approval gate. The database tests include
restart backlog/retry ordering, outbox rollback, ingestion rollback, feedback batch
ownership, concurrent phase completion, callback rollback/replay, and pinned versions.
Tray coverage includes cross-project actions, error recovery, narrow-screen access, live
progress and session links for both harnesses, quiet-worker presentation, orphaned reviews,
and final progress persistence. Harness tests verify early streaming and preserve session
identity on incomplete output; usage tests cover repeated Claude message blocks.

## Upgrade and operating notes

Apply the new additive migrations `20260904220000_review_hardening` and
`20260904230000_local_harness_agents`, then restart the
server. The normal `make dev` flow runs migrations. No operator database was migrated
as part of this implementation. Worktree-root changes still require a restart.

Old workflow snapshots remain pinned. Legacy empty-URL merge stages are interpreted
as the explicit merge action. Other old external stages without an endpoint stop
instead of reporting success; configure an endpoint or adopt the updated template's
explicit human verification gate for new work. Remote callbacks must be able to reach
the local server; there is no unauthenticated non-loopback mode.

Recovery deliberately does not blindly replay an uncertain GitHub publication or
external phase. Inspect the external system before retrying such a failed attempt.
Event delivery is at least once, so external operations must remain idempotent or
reconciled. Dirty workspaces are preserved for inspection and manual cleanup.

Live GitHub/Linear writes and real provider-backed harness runs were not exercised.
The shared subprocess budget and result conventions now agree, but dispatch and the
single-turn runner remain separate adapters. This is a correctness repair, not a
replacement with a new generalized execution framework. The existing large frontend
bundle warning remains a performance follow-up.
