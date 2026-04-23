# Interacting with Flywheel

Flywheel is built for **agents** (Cursor, Codex, Claude Code, Claude Teams, CI) as first-class users. Humans use the REST API for setup and review. This doc describes both flows.

---

## 0. Command Center (primary human interface)

The web **Command Center** is the primary place for a human operator to interact with Flywheel. The intended loop is:

1. Chat with the **orchestrator** in the Command Center.
2. Let that strong planner inspect context, clarify scope, and create work streams plus ticket DAGs.
3. Let the event-driven dispatch system fan that work out to cheaper executor/reviewer agents in the background.

Use the `ORCHESTRATOR_AGENT_*` environment variables when you want the command-center planner to run on a stronger model than the background `DISPATCH_*` workers.

---

## 1. REST API (humans + scripts)

Use the REST server for:

- **Setup:** Create orgs, projects, context packs, agents; create tickets (or let agents claim from the queue).
- **Review:** List pending reviews and escalations; approve/reject tickets; resolve escalations.
- **Inspection:** List projects, tickets, traces; get a ticket with full context.

**Run the server**

```bash
cp .env.example .env
# Edit .env: set GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, JWT_SECRET
docker compose up -d
# Server at http://localhost:8080
```

For a full **operations runbook** (run locally, migrations, DB/Redis inspection, health and logs, OAuth/MCP troubleshooting), see **docs/troubleshooting.md**.

**Typical REST flow**

1. **Create org and project** (POST /orgs requires auth; the authenticated user is added as org owner)
   ```bash
   curl -s -X POST http://localhost:8080/orgs -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_JWT" \
     -d '{"name":"Acme","slug":"acme"}'  # → org id
   curl -s -X POST http://localhost:8080/orgs/{org_id}/projects \
     -H "Content-Type: application/json" \
     -d '{"name":"Hubble Backend","slug":"hubble"}'  # → project id
   ```

2. **Set project context** (conventions, key files, system prompt for agents)
   ```bash
   curl -s -X PUT http://localhost:8080/projects/{project_id}/context-pack \
     -H "Content-Type: application/json" \
     -d '{"conventions":"Use Go 1.23. Prefer table-driven tests.","system_prompt":"You are a backend engineer."}'
   ```

3. **Register an agent** (get an API key for that identity)
   ```bash
   curl -s -X POST http://localhost:8080/agents -H "Content-Type: application/json" \
     -d '{"name":"Cursor in my IDE","type":"custom"}'  # → agent id + api_key (save it)
   ```

4. **Create tickets** (or let the agent claim from the queue)
   ```bash
   curl -s -X POST http://localhost:8080/projects/{project_id}/tickets \
     -H "Content-Type: application/json" \
     -d '{
       "title":"Add health check",
       "type":"task",
       "created_by":"user-123",
       "objective":{"description":"Add GET /healthz that returns 200 and ok."}
     }'
   ```

5. **Review and escalations**
   ```bash
   curl -s http://localhost:8080/projects/{project_id}/reviews      # pending reviews
   curl -s -X POST http://localhost:8080/tickets/{ticket_id}/reviews \
     -d '{"decision":"approved","notes":"LGTM","reviewer_id":"human-1"}'
   curl -s http://localhost:8080/projects/{project_id}/escalations  # open escalations
   curl -s -X POST http://localhost:8080/tickets/{ticket_id}/escalations/{esc_id}/resolve \
     -d '{"answer":"Use the existing logger.","reviewer_id":"human-1"}'
   ```

---

## 2. MCP (agents in IDE / Codex / Claude)

Agents talk to Flywheel via **MCP** so they can list projects, get a full ticket (objective + context pack + dependency outputs), claim work, log steps, submit or escalate, and renew leases—all as tools.

**MCP over HTTP (recommended for Cursor)**
When the Flywheel REST server is running with GitHub OAuth configured, MCP is also exposed at **`/mcp`**. Point Cursor at `"url": "http://localhost:8080/mcp"` (or your deployed base URL + `/mcp`). On first connect, Cursor gets a 401, discovers our OAuth metadata, and opens a browser for GitHub sign-in; after that it stores the token and uses it automatically. No manual token copy; `agent_id` is inferred from the token for tools like `claim_ticket` and `start_ticket`. See **docs/cursor-mcp.md** and **docs/oauth-mcp-cursor.md**.

**Codex CLI**
Codex can use the same streamable HTTP MCP endpoint:

```bash
codex mcp add flywheel --url http://localhost:8080/mcp
```

Or add it directly to `.codex/config.toml`:

```toml
[mcp_servers.flywheel]
url = "http://localhost:8080/mcp"
```

**Run the MCP server (stdio)**

Same DB and Redis as REST. Run the MCP server as the process your IDE or Claude connects to (e.g. Cursor MCP config runs this over stdio):

```bash
docker compose up -d
make run-mcp     # MCP over stdio (connects to localhost:5433, localhost:6379; requires Go)
```

Or point your MCP client at:

```bash
go run ./cmd/mcp
```

with env: `DATABASE_URL`, `REDIS_URL` (same as REST).

**Typical agent flow (via MCP tools)**

1. **list_projects** (no args when using OAuth) → returns projects in all orgs you're a member of; optionally pass `org_id` to limit to one org. Requires OAuth (agent linked to a user).
2. **get_project_context** (`project_id`) → conventions, key files, system prompt.
3. **claim_ticket** (`project_id`, `agent_id`) → receive a **ticket** and a **lease** (with `lease_token` and `expires_at`).
4. **get_ticket** (`ticket_id`) → full payload: objective, success criteria, acceptance test, context pack, dependency outputs, prior attempts, human answers. The agent uses this as the single input to do the work.
5. **start_ticket** (`ticket_id`, `lease_token`, `agent_id`) → transition to **executing**.
6. **log_step** (`ticket_id`, `lease_token`, `step_type`, `payload`) → record tool_call / observation / thought / error as the agent works. Optionally **renew_lease** to extend TTL.
7. When done, either:
   - **submit_ticket** (`ticket_id`, `lease_token`, `outputs`) → transition to **awaiting_review**; a human approves or rejects via REST.
   - **escalate_ticket** (`ticket_id`, `lease_token`, `reason`, `question`) → transition to **needs_human**; a human resolves via REST (answer is injected into context and ticket goes back to **executing**).

**Tool summary**

| Tool | Purpose |
|------|--------|
| `list_projects` | List projects for your org(s); OAuth required. Optional `org_id` to filter. |
| `get_project_context` | Context pack for a project. |
| `list_tickets` | List tickets by project (optional `state`, `priority`). |
| `get_ticket` | Full ticket + context pack + dep outputs + prior attempts (main input for the agent). |
| `claim_ticket` | Claim next available ticket; returns ticket + lease. |
| `start_ticket` | Move claimed → executing. |
| `log_step` | Append a step to the execution trace. |
| `submit_ticket` | Submit outputs; move to awaiting_review. |
| `escalate_ticket` | Ask for human help; move to needs_human. |
| `renew_lease` | Extend lease TTL. |

---

## 3. Auth: GitHub OAuth2

**No CLI registration.** First sign-in with GitHub creates your user and agent; you get a JWT to use for MCP and REST. On first sign-up we also create a **default org** named after your email (or GitHub login if email is not available) and add you as owner, so `list_projects` returns that org’s projects without any extra setup. To work with others, you’d create a separate collaboration org (invite flow) later.

1. **Create a GitHub OAuth App** (Settings → Developer settings → OAuth Apps): Homepage URL = your app URL, Authorization callback URL = `https://your-domain/auth/github/callback` (or `http://localhost:8080/auth/github/callback` for local).

2. **Configure env:** `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `JWT_SECRET` (any long random string), and optionally `BASE_URL` (e.g. `http://localhost:8080`), `AUTH_SUCCESS_REDIRECT_URL` (if you want a custom landing URL after login; the callback appends `#token=<jwt>`).

3. **Sign in:** Open `GET /auth/github` in a browser (or redirect the user there). After authorizing on GitHub, you’re redirected back; the callback creates your user + agent and redirects to **`BASE_URL/`** with **`#token=<jwt>`** for the web UI (or your `AUTH_SUCCESS_REDIRECT_URL` with the same fragment). The **TUI** still uses `?token=...` on a localhost `redirect_uri`; **MCP** uses the OAuth `code` exchange—those flows are unchanged.

4. **Use the token:** For **MCP over URL**, Cursor uses the token automatically after you complete the in-browser sign-in. For **MCP over stdio**, set the Bearer token (e.g. `FLYWHEEL_TOKEN` env) to that JWT. Same token works for REST: `Authorization: Bearer <token>`.

**API keys** still work for headless/CI: create an agent via `POST /agents` (no OAuth), get an `api_key`, and use `X-API-Key` header. For humans and IDE agents, use GitHub OAuth and the JWT.

---

## 4. Ticket lifecycle (recap)

```
PENDING → CLAIMED → EXECUTING → AWAITING_REVIEW → DONE
                ↓                        ↓
            BLOCKED                   FAILED
                ↓
          NEEDS_HUMAN  ←→ (human resolves) → EXECUTING
```

- **Claim** picks the highest-priority unblocked pending ticket and creates a time-limited lease (Redis).
- **Lease expiry** (scheduler) returns the ticket to PENDING if not renewed or submitted.
- **Dependencies:** tickets with `depends_on` only become claimable when those dependencies are DONE; **get_ticket** includes **dependency_outputs** for the agent to use.

---

## 5. Error responses (REST and MCP)

Errors are returned in a **structured shape** so clients and agents can branch on a stable `code` and decide whether to retry.

**Shape (JSON)**

```json
{"error":"human-readable message","code":"<code>","retriable":false}
```

**Error codes**

| Code | Meaning | Retry? |
|------|--------|--------|
| `lease_expired` | Lease token invalid or expired (e.g. submit/renew after TTL). | **Yes** – re-claim or get a new lease. |
| `unauthorized` | Not authenticated (e.g. OAuth required, agent not found). | No – sign in or link agent. |
| `forbidden` | Authenticated but not allowed (e.g. not a member of the org, no access to project). | No. |
| `not_found` | Resource missing (ticket, project, or “no ticket available to claim”). | For “no ticket available”, retrying later is OK; for missing ID, fix the ID. |
| `conflict` | State conflict (e.g. ticket already claimed, not the leaseholder, dependency not done). | No – refresh state and act accordingly. |
| `invalid_input` | Bad request (missing/invalid params, invalid JSON, trigger not allowed). | No – fix the input. |
| `internal` | Server error. | Optional – retry with backoff. |

**REST**  
All error responses use this JSON body and the appropriate HTTP status (401, 403, 404, 409, 400, 500).

**MCP**  
Tool errors return this **same JSON as the error string**. Agents can parse the tool error content as JSON to read `code` and `retriable` and implement retry logic (e.g. on `lease_expired` + `retriable: true`, call `renew_lease` or re-claim).
