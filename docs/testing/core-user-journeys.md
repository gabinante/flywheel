# Flywheel core user journeys

Flywheel is a local metaharness. The operator enters a persistent local workspace; there is no login or signup journey. Linear remains the source of tickets and GitHub the source of pull requests. This catalog describes observable outcomes, not just pages that render.

## Coverage map

The test references below are exact title prefixes or distinctive test-title fragments. `journeys.spec.ts` contains the new J-prefixed journeys; `journey-gaps.spec.ts` contains layout checks and acceptance criteria for audit findings. Other references point to existing tests in `hardening.spec.ts`, `review-queue.spec.ts`, or `activity.spec.ts`.

| # | User journey and expected outcome | Browser coverage |
|---|---|---|
| 1 | Open Flywheel and arrive at the local project workspace without authentication. | hardening: local workspace opens directly |
| 2 | Reload or open a deep link and retain access without a browser token. | hardening: local workspace opens directly; J03 |
| 3 | Create a project with a name and slug, then open its settings. | J03 |
| 4 | Reopen a project through its readable URL. | J03 |
| 5 | Create a work stream and preview its Markdown plan before saving. | J04 |
| 6 | Edit a work stream's branch, reload it, and close the stream. | J04 |
| 7 | Browse the workflow library and start from a built-in template. | J05 |
| 8 | Save a custom workflow, reopen it, rename it, and persist the edit. | J05 |
| 9 | Select a saved workflow in a project and customize its independent copy. | J06 |
| 10 | Change the organization default without changing an existing project override. | J07 |
| 11 | Cancel or confirm deletion of a saved library workflow. | J08 |
| 12 | Add, configure, reorder with the keyboard, and remove workflow steps. | J18 |
| 13 | Understand whether Save changes a library workflow, the org default, or a project. | G01; J05–J07 |
| 14 | Recover from a missing workflow template rather than wait forever. | G02 |
| 15 | Receive an error when saving a workflow to the library fails. | G07 |
| 16 | Create a custom worker profile and retain it across reload. | J09 |
| 17 | Choose a worker for reviews and implementation without creating a role. | J09 |
| 18 | Route real dispatch phases through both supported harness protocols. | hardening: real dispatcher phases drive both harness protocols |
| 19 | Edit a built-in agent prompt and restore its default. | J10 |
| 20 | Save model settings without overwriting separately edited prompts. | hardening: saving operator settings preserves separately edited prompts |
| 21 | Discuss a proposed change with a project's orchestrator, see activity and a reply. | J11 |
| 22 | Reload a project and retain its planning conversation. | J11 |
| 23 | Keep a planning draft after a failed submission and retry. | J12 |
| 24 | Cancel an active planner run. | J13 |
| 25 | View a ticket in the project list and open its objective/detail. | J17; ticket creation is fixture setup, not a UI claim |
| 26 | Open a worker session from the global tray while it works on a ticket. | hardening: real dispatcher phases drive both harness protocols |
| 27 | Search for an earlier session and open its linked PR. | J16 |
| 28 | Continue the same harness session from its detail page. | J16 |
| 29 | See project-scoped sessions update without refreshing. | hardening: sessions are scoped and refresh |
| 30 | Paste arbitrary PR references from global My PRs, My Reviews, or project reviews. | hardening: global PR views and project reviews share arbitrary PR queueing |
| 31 | Submit a dry-run review and see a receipt without posting externally. | same test; review-queue: review buttons acknowledge the queue immediately |
| 32 | Preserve the PR entry draft when queue submission fails. | hardening: global PR views and project reviews share arbitrary PR queueing |
| 33 | Follow queued reviews into live reviewer sessions with direct PR links. | review-queue: review buttons acknowledge the queue immediately |
| 34 | Distinguish independent reviewer and dispatch worker capacity. | review-queue: review buttons acknowledge the queue immediately |
| 35 | Pause review intake while allowing active reviewers to finish. | review-queue: review buttons acknowledge the queue immediately |
| 36 | Stop watching a PR from its global card and see the stopped state. | review-queue: review buttons acknowledge the queue immediately |
| 37 | Manually re-review a stopped PR. | same test; rerun API triggers the update, browser verifies the result |
| 38 | See automatic retries for transient failures and changing PR heads. | review-queue: transient failures and new commits retry automatically |
| 39 | Navigate project review pages without leaking unrelated projects' reviews. | hardening: project review filtering precedes pagination |
| 40 | Distinguish a quiet live reviewer from a saved record with no worker. | hardening: review progress distinguishes an orphaned record |
| 41 | Find work needing approval, retry, or continuation from any page. | hardening: global tray keeps cross-project decisions |
| 42 | Approve the current workflow phase; reject stale decisions. | hardening: operator approval is tied to the current phase attempt |
| 43 | Retry a failed phase or manually continue a blocked workflow. | hardening: failed phases retry with a new attempt |
| 44 | See background work update through SSE without page reloads. | activity: committed background changes |
| 45 | Recover live state after browser or database-listener disconnection. | activity: browser reconnect; database listener reconnect |
| 46 | Keep the tray accurate when an activity event races a slow fetch. | activity: an activity notice during a slow fetch |
| 47 | Retain the last known tray data when refresh fails. | hardening: tray errors preserve the last known work |
| 48 | Open and close the work tray on a narrow screen. | same test |
| 49 | Inspect recurring actions and navigate to their settings. | J14 |
| 50 | See a persistent, actionable error when Run now fails. | G03 |
| 51 | Schedule a specific PR review for a chosen time. | G04 — missing; expected failure |
| 52 | Create a workflow from scratch from the library. | G05 — missing; expected failure; template-based creation works |
| 53 | Recover from an unknown URL with a link back to projects. | G06 |
| 54 | Navigate the main global surfaces on desktop and phone widths without a JavaScript crash. | J15 at 1440px and 390px; screenshots and control/layout inventory |
| 55 | Assign a custom worker directly to a workflow phase while preserving its task type. | J19; hardening: real dispatcher phases verifies the named worker’s model, effort and instructions |

## Running and interpreting the suite

Run `scripts/test-hardening.sh` from the repository root. It builds Go and the production frontend, migrates disposable Postgres, runs database regressions, and runs Playwright against port 8091 with fake GitHub and harness executables. It removes the test containers afterward. Do not run the mutation suite against port 8090.

Pass Playwright arguments for a focused run, for example `scripts/test-hardening.sh journeys.spec.ts`. The HTML report is `web/playwright-report/index.html`; screenshots, failure traces and attached layout inventories are in the report and `web/test-results`. These generated files are intentionally ignored by Git.

Expected failures are explicit product gaps, **not passing functionality**. If one unexpectedly passes, Playwright fails the suite so its annotation and audit status can be updated. Tests for fixed defects use normal assertions.

Most journeys use the real HTTP handlers and database. External GitHub and harness responses are deterministic local fixtures. Error paths intentionally intercept specific HTTP responses. The quiet-worker visual state is a response fixture; the separate dispatch/review tests launch actual fake harness processes and exercise tracking. These tests do not validate real provider credentials, actual model judgment, GitHub publication permissions, or Linear account behavior.

## Additional integration acceptance work

These are beyond the isolated browser audit and must not be presented as verified journeys: connect a real Linear account; import projects and tickets; round-trip a ticket edit to Linear; publish a real PR review; address real review feedback and push a commit; post a real report; merge a real PR; recover actual provider rate limits; and continue a real historical Codex/Claude session. Use disposable external repositories/projects and explicit publishing intent when exercising those flows.
