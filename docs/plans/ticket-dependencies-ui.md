# Ticket dependencies in UI

Work stream: `ticket-dependencies-ui` (Flywheel project **Flywheel**). Branch: `feature/ticket-dependencies-ui`.

## Goal

Surface **dependency relationships** between tickets (`depends_on`) in the Flywheel web app so humans can see what blocks what without reading raw JSON or leaving the UI.

## Background

- Tickets already persist **`depends_on`** (array of ticket IDs); the API and generated TS types expose it.
- Server logic resolves dependencies for scheduling and unblocking; the gap is **visibility** in the UI.

## View matrix (what to show where)

| View | Route | Degree | Notes |
|------|--------|--------|--------|
| **Ticket detail** | `/orgs/.../tickets/:ticketId` | **Full** | **Depends on:** every linked ticket with title + id, link to detail. **Blocks** (reverse): tickets that list this id in `depends_on`, same treatment. Empty: omit sections or one-line “No dependencies.” |
| **Project tickets list** | `/orgs/.../tickets` | **Light** | If `depends_on.length > 0`: compact **badge** with count (e.g. `2 deps`); **tooltip** with dependency titles (from the already-loaded project ticket list). No full lists or graphs on each row. |
| **Review queue** | `/orgs/.../reviews` | **None (v1)** | Detail is one click away; skip dependency UI here unless we later see approval mistakes. |
| **Work stream / project hub** | work-streams, project | **None** | No ticket rows in those surfaces today. |

## Out of scope (v1)

- Editing `depends_on` in the UI.
- Full graph visualization (optional later).
- Dedicated API for reverse lookups — derive **Blocks** client-side from `GET /projects/{id}/tickets` on ticket detail.

## Implementation tickets (Flywheel)

Tracked as separate tickets in the **ticket-dependencies-ui** work stream:

1. **Deps UI: ticket detail (full)** — Depends on + Blocks sections with links; load project tickets once to resolve titles and reverse edges.
2. **Deps UI: tickets list (light)** — Dependency count badge + tooltip on list cards.

## Verification

- Manual: tickets with `depends_on` set show correct links on detail; list shows badge/tooltip when deps exist.
- No regressions on ticket list or detail layout.

## Success criteria

- Users see **which tickets a ticket depends on** and **who is blocked on this ticket** from **ticket detail**, with navigation to related tickets.
- **Tickets list** gives at-a-glance awareness without cluttering rows.
- No regression to existing ticket views.
