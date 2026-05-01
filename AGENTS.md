# AGENTS.md

This file is the shared operating guide for coding agents working in this repo.
`CLAUDE.md` should point here.

## Product frame

- Flywheel is a control plane for software delivery. It treats the ticket as the contract between humans and agents across planning, execution, validation, deploy, and observation.
- Project hierarchy: `org -> project -> work stream -> ticket`.
- PRs come from the Git provider, not Flywheel's DB. Infrastructure and deployment state come from pluggable integrations.
- Humans stay in the loop, but the system should advance work until it hits a real approval, policy, or judgment boundary.

## Repo map

- `cmd/server`: main server binary. Serves REST, MCP, auth, and the web UI.
- `internal/`: core services.
  - `dispatch/`: worker runners/drivers, sandboxing, queue execution.
  - `orchestrator/`: command-center planning conversation.
  - `delivery/`: PR and deployment pipeline surfaces and provider integrations.
  - `ticket/`, `workstream/`, `plan/`, `observation/`, `stateindex/`, `claims/`, `catalog/`: core SDLC/data-plane layers.
- `api/openapi.yaml`: REST contract.
- `api/rest/`: HTTP handlers.
- `db/migrations/`: SQL migrations.
- `web/`: Vite/React frontend.
- `scripts/varlock`: env/secrets loader and validator.

## Local dev

- `make dev` — **the primary dev command**. Ensures Docker Postgres+Redis are running, runs migrations, then starts the Go server. Re-invoke to restart after code changes.
- `make dev-infra` — start only Postgres and Redis (useful before running tests).
- `make dev-stop` — kill the server without stopping infra.
- `make run` — start the server only (assumes infra is already up).
- `make test` — Go tests.
- `make web-build` — production web build.
- `make generate` — regenerate from OpenAPI.
- `cd web && npm install && npm run dev` — Vite dev server (HMR).
- `cd web && npm run gen:api` — regenerate TS client after OpenAPI changes.
- `make varlock-validate` — validate `.env` against `.env.schema`.

## MCP setup

Flywheel exposes MCP at `/mcp` (Streamable HTTP). Both Claude Code and Cursor use the same endpoint.

Add to `~/.claude/.mcp.json`:
```json
{
  "mcpServers": {
    "flywheel": {
      "url": "http://localhost:8080/mcp",
      "headers": { "X-API-Key": "<DISPATCH_API_KEY from .env>" }
    }
  }
}
```

The SSE transport (`/sse`) is still available as a fallback for older clients.

Dispatched workers receive MCP config automatically — no manual setup needed.

After restarting the server (`make dev`), MCP clients reconnect automatically. If tools error after restart, start a new Claude Code session.

## OAuth and local URL rule

- Treat `http://localhost:8080` as the canonical local origin.
- GitHub OAuth callback is `http://localhost:8080/auth/github/callback`.
- `127.0.0.1` vs `localhost` matters for OAuth redirects and cookies.
- If you want HMR while keeping the app origin on `:8080`, run Vite separately and start the Go server with `WEB_DEV_PROXY_URL=http://127.0.0.1:5173`.

## Current platform rules

- Tickets are the unit of work and the contract between agents.
- Work streams group tickets into an initiative; they are not a substitute for the ticket dependency DAG.
- PRs should use the Git provider as source of truth.
- Delivery and infrastructure integrations must stay modular. Fly.io is the first in-tree infra provider.
- Project integrations config is currently stored in `context_pack.extra["delivery_config"]`; keep provider interfaces modular even if storage is still transitional.
- Command-center orchestration is planning-only; implementation belongs to dispatch workers.
- When touching orchestrator UI or API, preserve both conversation `messages` and planner `runs/events`; the final assistant message alone is not enough to show progress.

## Delivery workflow

- Flywheel is currently in alpha. Optimize for getting working code merged to `main`.
- The goal is merge-to-`main`, not branch hygiene or PR throughput.
- A feature is not done until the code is merged to `main`.
- Pushing a branch, opening a PR, or leaving work in review is not sufficient completion.
- Unless a task explicitly requires a separate branch or PR-based handoff, prefer the shortest path to landing validated changes on `main`.

## Dispatch and LLM config

- Runner and driver are separate:
  - runners: `cli`, `docker`, `openai-responses`, `openai-compatible`
  - drivers: `claude`, `codex`, `generic` for CLI or Docker harnesses
- Use `openai-compatible` for LiteLLM, vLLM, and other OpenAI-compatible `/v1/chat/completions` endpoints. Always set an explicit model and API base URL.
- Orchestrator and dispatch worker configs can differ. Use `ORCHESTRATOR_AGENT_*` when you want a stronger planner than the implementation workers.

## Coding rules

- Prefer integration over rebuilding commodity systems.
- Preserve the distinction between declared state, observed state, streams, claims, plans, and policy.
- Keep schemas additive. Avoid breaking public API or plugin/MCP contracts.
- Do not read `.env` directly in application code. Use config and a varlock-loaded environment.
- Dark mode first in the web UI. The existing UI language is glassy and operational, not marketing-style.
- Avoid broad refactors unless they directly serve the task.

## Migrations

- Existing legacy migrations are numbered.
- New migrations should be created with `make migrate-create NAME=...`, which uses timestamp-based filenames.
- Do not renumber or rewrite old migrations.

## When to flag a human

- Public schema or MCP contract changes.
- New privileged surface or secret-handling changes.
- New operator-managed external dependency.
- Anything that drifts toward Flywheel replacing Git hosting, CI, observability, identity, or other commodity systems.
