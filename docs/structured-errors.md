# Structured error responses

REST and MCP return errors in a consistent shape so clients and agents can branch on **code** and **retriable**.

## Shape

All error responses use this JSON structure:

```json
{
  "error": "Human-readable message",
  "code": "lease_expired",
  "retriable": false
}
```

- **REST:** Response body is this JSON; status code is derived from `code` (e.g. 401 for `unauthorized`, 404 for `not_found`).
- **MCP:** Tool errors return this same JSON as the error message string; parse it to read `code` and `retriable`.

## Error codes

| Code | Meaning | Typical cause | Retriable? |
|------|---------|----------------|------------|
| `lease_expired` | Lease invalid or expired | Token expired, wrong ticket, or lease was released | Yes – call **renew_lease** or **claim_ticket** again |
| `unauthorized` | Not authenticated or identity unknown | Missing/invalid API key or token; agent not linked to user | No – use a valid MCP key or restart to provision the local operator |
| `forbidden` | Authenticated but not allowed | No access to project/org; not the leaseholder | No – fix scope or use correct identity |
| `not_found` | Resource doesn't exist or no ticket available | Wrong ID; or **claim_ticket** when queue is empty | Often yes for "no ticket" (retry later); no for bad ID |
| `conflict` | State or precondition conflict | Ticket already claimed; dependency not done; wrong state for transition | No – refresh state, wait for deps, or release lease |
| `invalid_input` | Bad request (missing/illegal args) | Missing required param; invalid JSON; invalid trigger | No – fix arguments and retry |
| `project_closed` | Project is closed | Create ticket or claim on a closed project | No – use **update_project_status** to set status to `active`, or choose another project |
| `internal` | Server error | Bug or transient backend failure | Sometimes – retry once for transient; report if persistent |

## When to retry vs stop

- **Retriable = true:** Safe to retry the same or a follow-up action (e.g. `lease_expired` → renew or re-claim; `not_found` for "no ticket available" → try again later).
- **Retriable = false:** Do not retry the same call without a change (fix auth, fix input, refresh state, or escalate).

Use **code** to decide next step:

- `lease_expired` → **renew_lease** or **claim_ticket** (possibly another ticket).
- `unauthorized` → Sign in or supply a valid API key; do not retry until auth is fixed.
- `forbidden` → Check project/org access or leaseholder; fix then retry.
- `not_found` (no ticket) → Retry **claim_ticket** later; `not_found` (bad ID) → stop, fix ID.
- `conflict` → Refresh ticket state; wait for dependencies; or release lease and re-claim.
- `invalid_input` → Fix arguments and retry.
- `project_closed` → Reopen the project with **update_project_status** (status `active`) or pick another project.
- `internal` → Optional single retry; then stop or escalate.

Agents should parse the error string as JSON, read `code` and `retriable`, and branch accordingly. The in-app agent guide (Flywheel MCP) summarizes this; see also **docs/cursor-mcp.md** for setup.

## Idempotency (create and claim)

To reduce duplicate work on retries:

- **create_ticket**: Send an optional `idempotency_key`; duplicate requests with the same key return the existing ticket.
- **claim_ticket**: Send an optional `idempotency_key`; the same agent retrying with the same key gets the same ticket (renewed lease if still valid, or re-claim of that ticket if it is pending again).

Recommend using idempotency keys when the client may retry (timeouts, network errors) so one logical create or claim does not create duplicates or claim a different ticket.
