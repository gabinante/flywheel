# Flywheel Git Notes — Refs and schema

Quick reference for the Flywheel git-notes integration. Full design: [git-integration-design.md](git-integration-design.md).

## Refs (multi-ref)

Notes are stored in separate refs per type:

| Type      | Ref                              |
|-----------|----------------------------------|
| decision  | `refs/notes/flywheel/decision`   |
| trace     | `refs/notes/flywheel/trace`      |
| intent    | `refs/notes/flywheel/intent`     |

- **decision** — high-level decisions and rationale
- **trace** — execution summaries / trace attachments
- **intent** — what the agent set out to do

Sync: push/pull `refs/notes/flywheel/*` (or each ref).

## Note schema (JSON)

Versioned; current version `v: 1`.

| Field       | Type   | Required | Description |
|------------|--------|----------|-------------|
| `v`        | number | yes      | Schema version (1) |
| `type`     | string | yes      | `decision`, `trace`, or `intent` |
| `message`  | string | yes      | Note content |
| `created_at` | string | no    | RFC3339 timestamp |
| `agent_id` | string | no       | Flywheel agent ID |
| `ticket_id`| string | no       | Flywheel ticket ID |
| `project_id` | string | no     | Flywheel project ID |
| `payload`  | object | no       | Extra structured data |

Example:

```json
{
  "v": 1,
  "type": "decision",
  "message": "Refactored auth to use Pydantic",
  "agent_id": "agent-xyz",
  "ticket_id": "ticket-uuid",
  "project_id": "project-uuid",
  "created_at": "2026-03-12T10:00:00Z"
}
```

## How to use

- **CLI**: `flywheel-git note add|show|log|diff`, `flywheel-git sync` — see `flywheel-git help`.
- **MCP**: `flywheel_add_git_note`, `flywheel_show_git_notes`, `flywheel_log_git_notes`, `flywheel_diff_git_notes`, `flywheel_sync_git_notes`.
- **REST**: `GET /orgs/{orgID}/projects/{projectID}/git-notes/commits/{commitSha}?repo_path=...`, `GET .../git-notes/log?repo_path=...&limit=20&type=decision`.
