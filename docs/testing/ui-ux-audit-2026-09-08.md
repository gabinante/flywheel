# Flywheel UI, UX and functionality audit — September 8, 2026

## Scope and method

Audited the production build in Chromium against disposable Postgres/Redis and deterministic local GitHub, planner, and reviewer fixtures. The browser clicks the actual controls; the normal backend persists projects, workflows, roles, conversations, reviews, and session state. Tests inspect both UI outcomes and persisted records. Desktop (1440px) and narrow-screen (390px) screenshots cover global navigation; the existing 800px test exercises opening and closing the global work tray.

The [journey catalog](core-user-journeys.md) maps 55 journeys to executable tests. GitHub/Linear publishing and real model behavior are not claimed as tested. The user's live projects and reviews were not used as mutation fixtures.

## Findings

| ID | Severity | Finding and reproduction | Resolution / coverage |
|---|---|---|---|
| AUDIT-00 | P1 | Create a library workflow, then reopen/edit it. The frontend requested `/workflows/:id`, but only `/api/v1/workflows/:id` was registered. GET returned the app HTML and saving failed. | Corrected the editor to use the registered GET/PUT endpoints. J05 verifies create, edit and reload; J06 verifies an independent project copy; J08 verifies deletion. |
| AUDIT-01 | P2 | A template or saved library workflow was labeled “Organization Default Pipeline,” and its primary button said “Save org default” even though it created/edited a library entry. | Library editors now identify themselves and say “Create workflow” / “Save workflow.” G01. |
| AUDIT-02 | P2 | Open `/workflows/new?template=nonexistent-audit-template`. The page never leaves “Loading template…”. | Missing/failed templates show an error and a link back to the library. G02. |
| AUDIT-03 | P2 | A failed “Run now” on Scheduled actions briefly set an error, then the unconditional reload cleared it. | Failure returns without clearing feedback; the error has alert semantics. G03 injects a failed action. |
| AUDIT-04 | Product gap | Scheduled actions lists recurring watchers and pending work, but has no way to schedule an arbitrary PR for a chosen time. | Still missing. G04 is an explicit expected-failure acceptance test, not a passing journey. |
| AUDIT-05 | Product gap | The workflow library only offers built-in template entry points. A blank workflow cannot be started directly from the library. | Still missing. G05 is an explicit expected-failure acceptance test. Customization of a template works. |
| AUDIT-06 | P2 | An unrecognized hash route rendered no recovery surface. | Added a not-found page with a link to projects. G06. |
| AUDIT-07 | P2 | “Save to library” ignored non-success responses, leaving the operator without feedback. | The failure is now displayed in the editor. G07. |
| AUDIT-08 | P2 | A missing saved workflow silently fell back to the suggested Standard SDLC pipeline, which looked like editable saved work. | Show an explicit error and disable saving until a definition loads. G08. |
| AUDIT-09 | P2 accessibility | Workflow drag/expand buttons and phase-name inputs lacked accessible names. Planner, agent-chat and prompt textareas also lacked explicit labels. | Added control labels and wrapped workflow toolbar buttons for narrow layouts. J18 uses named controls for keyboard editing. |

| AUDIT-10 | P1 | During a slow session continuation, live activity marked the session running and unmounted the conversation component. The completed reply disappeared. | Keep the conversation mounted and disable extra sends while running. J16 deliberately holds the harness until the UI sees the running state, then verifies the reply. |
| AUDIT-11 | P1 / product gap | Custom roles can be saved in Workers & roles, but workflow phases only offer built-in roles. A direct attempt to save a custom role also fails backend validation with “unknown agent role.” | Still missing. G09 expresses the complete acceptance path as an expected failure. J09 separately verifies that worker/role creation works. Fix requires the picker, validation, and execution to agree on custom roles. |

## UX observations and follow-up priorities

- The global tray provides a consistent place to find worker sessions and decisions. Separate reviewer/dispatcher capacities, direct PR links, pending states, and retained data after a failed refresh all have browser coverage.
- The local workspace opens directly. Login would be an obsolete acceptance criterion for this product.
- Global pages remain navigable at phone width, but the settings section chooser occupies most of the first viewport before the selected settings begin. A compact chooser would reduce scrolling. This is an observation, not a claim that all mobile interactions are tested.
- Ticket empty states say to create a ticket without providing a direct creation action or link to the project planner. The planner can author tickets, and Linear is the ticket store, but the entry point is poorly signposted. A “Plan work” action would make this path clearer.
- “Organization default” language still exposes an organizational layer in an otherwise single-operator product. Consider “Default workflow,” with project inheritance explained where it matters.
- Workflow authoring has many controls. Keyboard naming is improved, but this audit is not a comprehensive accessibility certification: contrast, screen-reader narration, every select/popover, and every drag target need a dedicated accessibility pass.

## Verification results

The expanded suite contains **46 Playwright tests: 43 working-path/regression tests and 3 expected failures** for AUDIT-04, AUDIT-05, and AUDIT-11. None of those three represents working functionality. The earlier suite had 20 tests.

The production build, frontend lint, 11 frontend unit tests, `go test ./...`, and isolated database regression suites also pass. The browser audit found and fixed nine concrete defects/accessibility issues; three missing capabilities remain explicit. No live review was stopped for deployment: the editor uses the server's existing versioned workflow endpoint, and the frontend is served from the rebuilt distribution.

## Selected browser evidence

These screenshots use synthetic test data and were inspected during the audit:

- [Workflow editor after saving a custom workflow](audit-images/workflow-editor.png)
- [Session continuation with the returned reply retained](audit-images/session-continuation.png)
- [Settings at phone width](audit-images/mobile-settings.png)

## Evidence and repeatability

Run `scripts/test-hardening.sh` to recreate the audit. `web/playwright-report/index.html` contains browser screenshots, traces for failed expectations, and attached control/layout inventories. `web/test-results/results.json` records outcomes in machine-readable form. The HTML report counts expected failures as satisfied expectations; distinguish those from working features when reporting results.

The planner fixture speaks the Claude event protocol and respects resume session IDs. It holds work until the browser releases it, allowing meaningful assertions about running/cancelled states. Existing dispatch fixtures exercise MCP claims and both harness protocols; reviewer fixtures exercise preparation, retry and session tracking.

## Worker interface follow-up

The worker consolidation replaces the separate settings navigation with Workers, including task assignments and shared harness connections. The AUDIT-11 intent is now covered by selecting a custom worker directly in a workflow step (J19), without requiring a custom role. Legacy roles remain an advanced compatibility feature. The suite now has 44 normal passing tests and two expected failures (G04/G05); the worker editor also has focused mobile and SSE picker checks. See [Workers](../workers.md).
