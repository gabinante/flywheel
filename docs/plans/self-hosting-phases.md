# Self-Hosting Phases and Bootstrap Discipline

> Defines the phased path from conventional development to full self-hosting,
> where Warrant manages its own development lifecycle through its own ticket system.

## Overview

Self-hosting means Warrant uses itself to orchestrate its own changes. This is
both the strongest possible dogfood signal and a dangerous failure mode: a bug
in the harness can prevent fixing the bug via the harness. The phases below
define a conservative ramp with clear entry criteria and safety valves.

---

## Phase Definitions

### V0: Bootstrap (Current)

**Description:** Conventional development. No self-hosting. Humans and agents
develop Warrant using external tooling (editors, CLI, manual git workflows).
The ticket system exists and is used for tracking, but changes are not
*enforced* through it.

**Characteristics:**
- Tickets are informational, not gatekeeping
- Any contributor can commit directly without a ticket
- No automated enforcement of ticket-per-change
- State machine, dispatcher, and core orchestration developed manually
- Focus: get the system reliable enough to trust with its own codebase

**Entry criteria:** N/A (starting state)

**Exit criteria → V1:**
- [ ] Core ticket lifecycle (draft → executing → validated → closed) has been
      exercised on 50+ non-self tickets without manual intervention failures
- [ ] Lease management has zero observed zombie tickets over 7 consecutive days
- [ ] Rollback rate on agent-executed tickets < 15% over trailing 30 tickets
- [ ] Agent worker can reliably: read files, write files, run tests, create
      commits, push branches, open PRs
- [ ] At least 3 distinct agent sessions have completed tickets end-to-end
      without human escalation for infrastructure reasons
- [ ] Meta-change parallel instance infrastructure is operational (see below)

---

### V1: Partial Self-Hosting

**Description:** Well-understood, low-risk change categories are executed
through the Warrant ticket system. The harness manages changes to itself for
categories where failure is cheap and rollback is trivial.

**Characteristics:**
- Specific change categories routed through self-hosted tickets
- All other changes remain manual (with optional ticket tracking)
- Human approval required for all self-hosted changes (no auto-advance)
- Parallel instance available for meta-changes
- Rollback is always `git revert` — no complex undo

**Change categories for V1:**

| Category | Example | Why safe |
|----------|---------|----------|
| Documentation | README updates, doc additions, comment improvements | Zero runtime impact |
| Log adjustments | Log level changes, message rewording, adding context fields | Observable, non-functional |
| Dependency bumps | go.mod minor/patch updates with passing tests | Test suite catches regressions |
| Test additions | New test cases, test coverage expansion | Cannot break production code |
| Linter/format fixes | gofmt, eslint autofix | Deterministic, style-only |
| Configuration comments | Adding doc comments to config structs | Non-functional |
| OpenAPI doc updates | Description improvements in openapi.yaml | Schema unchanged |

**Explicitly excluded from V1:**
- State machine transitions or guards
- Dispatcher logic
- Worker orchestration
- Database migrations
- Authentication/authorization
- MCP tool definitions
- Any change to `internal/ticket/`, `internal/queue/`, `internal/dispatch/`

**Entry criteria (from V0):** See V0 exit criteria above.

**Exit criteria → V2:**
- [ ] 100+ V1-category tickets completed via self-hosting without rollback
- [ ] Zero incidents where a self-hosted change broke the harness itself
- [ ] Mean time from ticket creation to PR merge < 10 minutes for V1 categories
- [ ] Human approval turnaround tracked; false-positive rate (approvals that
      should have been auto-approved) > 80% indicates readiness for auto-advance
- [ ] Rollback rate for V1 categories < 5% over trailing 50 tickets
- [ ] Agent demonstrates correct escalation behavior (asks for help rather than
      making risky changes) in at least 5 observed cases

---

### V2: Broad Self-Hosting

**Description:** Most changes flow through the ticket system. Manual override
remains available but is the exception. Auto-advance enabled for safe
categories. The ticket system is the *expected* path for all changes.

**Characteristics:**
- All change categories routed through tickets by default
- Auto-advance for V1 categories (no human approval needed)
- Human approval for new categories (business logic, API changes, infra)
- Manual override available via `--bypass-ticket` flag (audited)
- Every bypass is logged and reviewed in weekly retro
- Meta-changes still route through parallel instance

**Additional change categories (beyond V1):**

| Category | Approval | Risk |
|----------|----------|------|
| New API endpoints | Human required | Medium — affects contract |
| Bug fixes (non-core) | Human required | Medium — logic changes |
| Feature additions (non-core) | Human required | Medium — scope expansion |
| Frontend changes | Auto-advance after 20 successes | Low — no backend impact |
| CI/CD pipeline changes | Human required | Medium — affects deploy path |
| Performance optimizations | Human required | Medium — behavior change |

**Still excluded (meta-changes — see below):**
- State machine modifications
- Dispatcher/scheduler logic
- Lease management
- Core ticket model changes
- Worker spawning/lifecycle

**Entry criteria (from V1):** See V1 exit criteria above.

**Exit criteria → V3:**
- [ ] 500+ tickets completed via self-hosting across all non-meta categories
- [ ] Bypass rate < 5% of total changes (most contributors use tickets naturally)
- [ ] Zero harness-breaking incidents from self-hosted changes in trailing 90 days
- [ ] Auto-advance categories have < 2% rollback rate
- [ ] Human approval categories have < 10% rollback rate
- [ ] Contributors report ticket workflow as "not slower than manual" in survey
- [ ] Parallel meta-instance has been used for 10+ meta-changes successfully

---

### V3: Full Self-Hosting

**Description:** Every change has a ticket. Contributors author tickets in the
harness as the primary development interface. The ticket system is the only
sanctioned path for changes to the codebase.

**Characteristics:**
- `every-change-has-a-ticket` enforced via pre-push hook and CI
- Contributors interact primarily through coordinator conversations
- Auto-advance for well-understood categories, human gates for novel ones
- Meta-changes use parallel instance (never changes to the running instance)
- Comprehensive audit trail: every line of code traceable to a ticket
- Rollback is a first-class operation in the ticket system
- Calibration loop: system learns from rejections and rollbacks

**Entry criteria (from V2):** See V2 exit criteria above.

**Ongoing operational criteria:**
- Monthly review of auto-advance categories (expand or restrict based on data)
- Quarterly review of meta-change procedures
- Incident review for any harness-affecting failure
- Bypass mechanism remains available for true emergencies (requires 2 humans)

---

## Meta-Change Handling

### The Problem

The harness cannot safely modify its own core orchestration while running on
itself. If a ticket modifies the state machine and introduces a bug, the state
machine may be unable to process the ticket's own completion — or worse, may
corrupt other in-flight tickets.

**Core orchestration** = components where a bug prevents the system from
functioning at all:
- `internal/ticket/statemachine.go` — state transitions
- `internal/ticket/service.go` — ticket lifecycle operations
- `internal/queue/` — scheduling, lease management, Redis coordination
- `internal/dispatch/dispatcher.go` — worker spawning and routing
- `internal/dispatch/worker.go` — execution sandbox
- `db/migrations/` — schema changes (irreversible without rollback plan)
- `config/config.go` — server bootstrap configuration

### The Solution: Parallel Instance

Meta-changes are processed by a **parallel instance** — a separate deployment
of Warrant that manages the ticket but does not run the modified code:

```
┌─────────────────────────────┐     ┌─────────────────────────────┐
│   Production Instance       │     │   Meta Instance              │
│   (runs current code)       │     │   (runs current code)        │
│                             │     │                             │
│   Manages all non-meta      │     │   Manages meta-change        │
│   tickets normally          │     │   tickets only               │
│                             │     │                             │
│   Does NOT execute changes  │     │   Executes changes to core   │
│   to its own core           │     │   orchestration code         │
└─────────────────────────────┘     └─────────────────────────────┘
                                              │
                                              ▼
                                    ┌─────────────────────┐
                                    │  Validation:        │
                                    │  1. Tests pass      │
                                    │  2. Deploy to       │
                                    │     staging         │
                                    │  3. Run smoke       │
                                    │     tickets on      │
                                    │     staging         │
                                    │  4. Human approves  │
                                    │  5. Rolling deploy  │
                                    │     to production   │
                                    └─────────────────────┘
```

### Meta-Change Workflow

1. **Detection:** Change is classified as meta if it touches files in the core
   orchestration set (detected by path prefix matching against the exclusion
   list above).

2. **Routing:** Ticket is created on the meta-instance, not the production
   instance. The meta-instance runs the *current stable* code.

3. **Execution:** Agent on meta-instance produces the change in a worktree of
   the same repo. Normal ticket lifecycle applies.

4. **Validation (enhanced):**
   - All unit tests pass (`make test`)
   - Integration tests pass (`make test-integration`)
   - Build succeeds (`go build ./...`)
   - Deploy to staging environment
   - Run 10 "smoke tickets" on staging (pre-defined tickets that exercise
     the full lifecycle)
   - All smoke tickets complete without error
   - Human reviews diff with explicit attention to state machine invariants

5. **Deployment:** Rolling deploy with canary. New instance handles new tickets;
   existing tickets drain on old instance. If new instance fails health checks
   or ticket completion rate drops, automatic rollback.

6. **Observation:** Extended observation window (4h minimum) watching for:
   - Zombie tickets (leases expiring without progress)
   - State transition failures
   - Worker spawn failures
   - Unexpected escalation rate increase

### Manual Fallback

If the parallel instance is unavailable or the change is too risky for
automated processing:

1. Human authors the change manually (conventional development)
2. Change is still tracked in a ticket (created retroactively if needed)
3. Enhanced review: minimum 2 human reviewers for core orchestration
4. Same validation/deployment pipeline applies
5. Post-merge: ticket closed with `bypass_reason` field populated

---

## Dogfooding Threshold

### Definition

The "dogfooding threshold" is the point at which **every change to the Warrant
codebase must have a corresponding ticket**. This is enforced mechanically, not
just by convention.

### Enforcement Mechanism

```
Pre-push hook (installed at V3):
  1. Extract commit range being pushed
  2. For each commit, check git-notes for ticket ID
  3. If any commit lacks a ticket reference → reject push
  4. CI duplicate check: PR must reference a ticket in state ≥ executing

Bypass (emergency only):
  - Requires `WARRANT_EMERGENCY_BYPASS=<reason>` env var
  - Creates audit event with bypass reason
  - Two humans must approve the PR
  - Retroactive ticket created within 24h (enforced by weekly sweep)
```

### When to Enforce

The dogfooding threshold is reached when ALL of the following are true:

1. **V3 entry criteria met** (see above)
2. **All active contributors have completed 5+ tickets** through the system
   (familiarity requirement)
3. **Ticket creation latency < 30 seconds** (the system must not be slower
   than `git commit` for simple changes)
4. **Auto-advance covers 60%+ of historical change types** (most changes
   should not require human approval to avoid bottlenecks)
5. **Emergency bypass has been tested** and works reliably
6. **No single point of failure:** if the ticket system is down, development
   is not blocked (bypass mechanism, cached pre-push hook, etc.)

### Graduated Enforcement

Rather than a hard cutover, enforcement ramps:

1. **Advisory (V2 start):** Pre-push hook warns but does not block. Dashboard
   shows "ticketless commits" metric.
2. **Soft enforce (V2 + 30 days):** Pre-push hook blocks by default but
   `--no-verify` still works. Ticketless commits flagged in weekly retro.
3. **Hard enforce (V3):** Pre-push hook blocks unconditionally. Only the
   emergency bypass env var overrides. CI also rejects unticketed PRs.

---

## Risk Mitigation

### Known Failure Modes

| Failure Mode | Mitigation |
|---|---|
| Harness bug prevents ticket completion | Emergency bypass + manual fix |
| Meta-change breaks production | Canary deploy + automatic rollback |
| Circular dependency (fix requires ticket, ticket system is broken) | Manual fallback with retroactive ticketing |
| Agent produces subtly wrong meta-change | Enhanced validation + smoke tickets |
| Lease expiry during long meta-change validation | Extended lease TTL for meta-tickets (4h vs 30m) |
| Both instances down simultaneously | Manual development is always available as fallback |

### Metrics to Monitor

- **Ticket completion rate:** Should stay > 95% (non-escalated)
- **Rollback rate by category:** Tracked per V1/V2/V3 category
- **Time-to-merge:** Should not regress vs manual development
- **Escalation rate:** Should decrease over time as system learns
- **Bypass rate:** Should stay < 5% at V3
- **Meta-change success rate:** Should stay > 90%
- **Zombie ticket rate:** Should stay at 0 for > 7 days before any phase transition

---

## Timeline (Indicative)

| Phase | Estimated Duration | Key Milestone |
|-------|-------------------|---------------|
| V0 → V1 | 4-6 weeks | First self-hosted doc change merged |
| V1 → V2 | 8-12 weeks | 100 successful V1 tickets |
| V2 → V3 | 12-16 weeks | Bypass rate < 5% |
| V3 stable | Ongoing | Every change has a ticket |

Phases should not be rushed. Entry criteria are gates, not targets. If a gate
is not met, stay in the current phase. Regression (moving back a phase) is
acceptable and expected if reliability metrics degrade.
