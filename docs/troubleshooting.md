# Troubleshooting

## Operations runbook

**Run locally:** See README or **docs/deployment.md**: copy `.env.example` to `.env`, set secrets, then `docker compose up -d`. Server at http://localhost:8080.

**Run migrations:** Migrations run automatically in the server container. For hosted/non-Docker deploys, run `make migrate` from the host. Migrations are in **db/migrations/**; never run them from multiple app instances at once.

**Inspect DB:** Connect with `psql` using `DATABASE_URL` (e.g. `psql postgres://flywheel:flywheel@localhost:5433/flywheel`). Key tables: `orgs`, `org_members`, `projects`, `tickets`, `execution_steps`, `reviews`, `escalations`. Use `GET /tickets/{id}` or MCP **get_ticket** to inspect a ticket; use **get_trace** for execution steps.

**Inspect Redis:** Connect with `redis-cli` using `REDIS_URL` (e.g. `redis-cli -u redis://localhost:6379/0`). Lease keys: `flywheel:lease:{ticketID}`. Idempotency keys: `flywheel:idempotency_claim:*`. Expiry set: `flywheel:lease:expires`. Use `KEYS flywheel:*` to list; `TTL flywheel:lease:X` to see lease TTL.

**Health and logs:** **GET /healthz** returns 200 when the HTTP server is up. For Docker, use `docker compose logs -f server` to tail logs. Each request is logged with method, path, status, duration, and **request_id** (in `X-Request-Id` response header). Use request_id to correlate errors with log lines. API errors are structured JSON only; no stack traces in responses.

**OAuth / MCP troubleshooting:** If Cursor or another MCP client cannot sign in or gets 401: ensure **BASE_URL** in `.env` matches the URL the client uses (e.g. `http://localhost:8080`). Ensure `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `JWT_SECRET` are set. For "no ticket available" and releasing stuck tickets, see **Tickets: agent stuck or wrong state** below and **docs/mcp-human-in-the-loop.md**. For full MCP setup, see **docs/cursor-mcp.md** and **docs/interacting.md**.

---

## Server exits when running in Docker

For Docker Compose setup, config, health checks, and migration workflow, see **docs/deployment.md**.

**See the actual error:**
```bash
docker compose run --rm server
```
(Run the server in the foreground without `-d`; the log message will show before the process exits.)

**Or view logs of the last run:**
```bash
docker compose logs server
```

**Common causes:**

1. **Database connection failed** – Postgres not ready or wrong URL.
   - In Docker Compose the server uses `postgres:5432` and `redis:6379` (set in `docker-compose.yml`). Don’t override `DATABASE_URL` or `REDIS_URL` in `.env` when running the server in Compose, or use the same hostnames.

2. **Migrations not run** – Migrations run automatically in the server container. If the server exits on startup, check `docker compose logs server` for migration errors.

3. **Missing .env** – If `docker compose up` fails with "no such file: .env" or similar, run `cp .env.example .env` and edit it with your secrets.

4. **Invalid `.env`** – If auth is enabled, the server expects `GITHUB_CLIENT_ID` and `JWT_SECRET`. Empty values are fine for startup; the server only enables auth when both are non-empty. If the process still exits, check for syntax errors or stray characters in `.env`.

5. **Port 8080 already in use** – Another process is bound to 8080. Change `PORT` in the server’s environment or stop the other process.

---

## Tickets: agent stuck or wrong state

When an agent crashes after claiming a ticket, or a ticket is stuck in **claimed** or **executing**, use this runbook.

### Ticket claimed but not started (agent crashed)

- The ticket is in state **claimed** and the agent never called **start_ticket** (or crashed before doing work).
- A **lease** is held in Redis with a TTL. When the TTL expires, the **queue scheduler** (running in the server) detects it and transitions the ticket to **pending** with trigger `lease_expired`. The default poll interval is 30 seconds, so a stuck lease will clear shortly after it expires.
- **Option 1 – Wait:** Let the lease expire. The scheduler will move the ticket back to **pending** so another agent (or retry) can claim it.
- **Option 2 – Release lease (if you have the token):** If you have the `lease_token` (e.g. from logs or the agent's last response), call:
  - **REST:** `DELETE /tickets/{ticketID}/lease` with body `{"lease_token": "..."}` (or `?lease_token=...` in query).
  - That removes the lease and transitions the ticket to **pending**.
- **Option 3 – Force transition (no token):** If you don't have the lease token, call the transitions API as a human/operator:
  - **REST:** `POST /tickets/{ticketID}/transitions` with body `{"trigger": "lease_expired", "actor": "system", "actor_id": "operator"}`.
  - This moves **claimed → pending** (or **executing → pending**) without validating the lease. Only `actor: "system"` is allowed for `lease_expired` from executing. The lease key in Redis may still exist until TTL; the ticket is still back in the queue.

### Lease expired and ticket returned to pending

- When a lease expires (or is released), the ticket transitions **claimed → pending**. No action needed; the ticket is again available for **claim_ticket**.

### How to inspect ticket state and trace

- **Ticket state and metadata:**
  - **REST:** `GET /tickets/{ticketID}`
  - **MCP:** **get_ticket** (ticket_id) – returns full ticket, context, dependency outputs, prior attempts.
- **Execution trace (log_step entries):**
  - **REST:** `GET /tickets/{ticketID}/trace`
  - **MCP:** **get_trace** (ticket_id) – returns all steps (tool_call, observation, thought, error) so you can see what the agent did before it got stuck or before review.

### Releasing a stuck lease or forcing ticket back to pending

- **With lease token:** `DELETE /tickets/{ticketID}/lease` with `lease_token` (body or query).
- **Without lease token:** `POST /tickets/{ticketID}/transitions` with `{"trigger": "lease_expired", "actor": "system", "actor_id": "operator"}`.

For more on using Flywheel from Cursor and MCP, see **docs/cursor-mcp.md**. For the agent flow (claim → start → log_step → submit), see the Flywheel MCP agent guide (e.g. resource `flywheel://docs/agent-guide` or in-app guide).
