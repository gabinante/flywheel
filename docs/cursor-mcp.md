# Configuring Flywheel MCP in Cursor

Use the Flywheel MCP server in Cursor so the AI can claim tickets, get context, log steps, submit, and escalate. Cursor supports two ways to connect:

1. **URL + OAuth (recommended)** – Cursor connects to the Flywheel server by URL; when sign-in is required, Cursor opens a browser and handles the token for you.
2. **Stdio + env token** – Cursor runs the MCP process locally and you pass a JWT via env (manual copy from the OAuth success page).

---

## Option 1: URL + OAuth (no token paste)

When you add the MCP server by **URL**, Cursor will get a 401 on first connect, discover our OAuth metadata, and open a browser for you to sign in with GitHub. After that, Cursor stores the token and uses it automatically.

### Prerequisites

1. **Flywheel server running** — from the repo root:
   ```bash
   cp .env.example .env
   # Edit .env: set GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, JWT_SECRET
   docker compose up -d
   ```
   Server at http://localhost:8080. Add `BASE_URL=http://localhost:8080` to `.env` if using MCP URL auth.
2. Set `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `JWT_SECRET` in `.env`. Set `BASE_URL` to the URL Cursor will use (e.g. `http://localhost:8080` for local dev).

### Cursor configuration

In **Cursor Settings → Tools & MCP** or `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "flywheel": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

For a deployed server, use your public base URL, e.g. `"url": "https://flywheel.example.com/mcp"`.

- On first use, Cursor will prompt for sign-in and open the browser; complete GitHub OAuth.
- No token or env vars needed; `agent_id` is inferred from the token for tools like `claim_ticket` and `start_ticket`.

---

## Option 2: Stdio + env token

Cursor runs the MCP process and you pass a JWT via env. Use this when you can't use the URL (e.g. no HTTP server) or prefer a long-lived token from the success page.

### Prerequisites

1. **Flywheel server running** (same DB and Redis the MCP server will use):
   ```bash
   cp .env.example .env
   # Edit .env: set GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, JWT_SECRET
   docker compose up -d
   ```

2. **GitHub OAuth** so you have an identity and token:
   - Set `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `JWT_SECRET` in the server env.
   - Open **http://localhost:8080/auth/github** in a browser and complete sign-in.
   - On the success page, copy your **Agent ID** and **Token** (use the token in Cursor's MCP env).

### Cursor configuration

Create **`.cursor/mcp.json`** (or use global MCP config):

```json
{
  "mcpServers": {
    "flywheel": {
      "command": "go",
      "args": ["run", "./cmd/mcp"],
      "env": {
        "DATABASE_URL": "postgres://flywheel:flywheel@localhost:5433/flywheel?sslmode=disable",
        "REDIS_URL": "redis://localhost:6379/0",
        "FLYWHEEL_TOKEN": "PASTE_YOUR_JWT_HERE"
      }
    }
  }
}
```

Replace `PASTE_YOUR_JWT_HERE` with the token from the sign-in success page.

- **Working directory:** If the Flywheel code is in a subfolder, set `"cwd": "flywheel"` or use an absolute path in `args`.
- With stdio, you must pass `agent_id` in tool calls for `claim_ticket` and `start_ticket` (or set `FLYWHEEL_TOKEN` so the server can infer it if you add support in the stdio path).

### Using a built binary

```bash
cd /path/to/flywheel && go build -o flywheel-mcp ./cmd/mcp
```

Then in `mcp.json` use `"command": "/path/to/flywheel/flywheel-mcp"` with the same `env`.

---

## Env vars (stdio only)

| Variable        | Required | Description |
|----------------|----------|-------------|
| `DATABASE_URL` | Yes      | Same Postgres URL the REST server uses (e.g. port 5433 if using Docker). |
| `REDIS_URL`    | Yes      | Same Redis URL the REST server uses. |
| `FLYWHEEL_TOKEN`| No*      | JWT from the sign-in page. Lets the MCP server know which agent is calling. |

\* If you don't set `FLYWHEEL_TOKEN`, you must pass `agent_id` explicitly when calling tools that require it (e.g. `claim_ticket`, `start_ticket`).

---

## After changing config

- **Restart Cursor** (or reload the window) so it picks up the new MCP server.
- With URL auth, on first use Cursor will open the browser for GitHub sign-in and then store the token.

## Troubleshooting tickets and agents

For the full **operations runbook** (run locally, migrations, inspect DB/Redis, health and logs, OAuth/MCP), see **docs/troubleshooting.md** → **Operations runbook**.

If a ticket is stuck **claimed** or an agent crashed after claiming, see **docs/troubleshooting.md** → **Tickets: agent stuck or wrong state**. That runbook covers: what to do when a ticket is claimed but not started, when the lease expires, how to inspect ticket state and trace (REST and MCP), and how to release a stuck lease or force a ticket back to **pending**. For how the MCP server can ask the human for help via notifications, sampling, or elicitation, see **docs/mcp-human-in-the-loop.md**.

**Error responses:** REST and MCP return errors in a consistent JSON shape (`error`, `code`, `retriable`). For the list of codes and when agents should retry vs stop, see **docs/structured-errors.md**.

---

## Quick check

1. Flywheel server: `curl -s http://localhost:8080/healthz` → `ok`.
2. **URL:** Add `"url": "http://localhost:8080/mcp"` to MCP config; restart Cursor; use a tool – Cursor should prompt for sign-in once.
3. **Stdio:** Sign in at http://localhost:8080/auth/github, copy Token and Agent ID; add `flywheel` to MCP config with env and token; restart Cursor and ask the AI to list projects or tickets.
