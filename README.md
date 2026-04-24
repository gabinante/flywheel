# Flywheel

## What it is

Flywheel is a **work queue plus shared context** for software projects, built so **AI agents and people** can use the same system. You organize work in **organizations** and **projects**, and each project carries a **context pack** (conventions, file hints, system prompt) so agents know *how* you want work done not just *what* ticket text says.

Work is tracked as **tickets** with an objective. Tickets move from a project **queue** → claimed → in progress → **submitted for review**.

## How it works

**Agents** connect over **MCP** (e.g. Cursor, Codex, Claude Code) or the **REST API**. They **claim** a ticket from the queue, get the full ticket plus that project’s context, **log steps** while working, then **submit** (or escalate) for a human. **Humans** use the web UI or REST to review, approve, reject, or resolve escalations. Sign-in is usually **GitHub OAuth**; agents can also use API keys where that fits.

One **Go server** serves the REST API, **MCP** at `/mcp`, and the web UI. Optional **git notes** can tie traces or decisions to your repository ([docs/git-notes.md](docs/git-notes.md)).

More detail on flows: [docs/interacting.md](docs/interacting.md).

## Quick start

You need **Docker** with the **Compose v2 plugin**, and a [GitHub OAuth app](https://github.com/settings/developers) if you want sign-in. Local callback URL:

`http://localhost:8080/auth/github/callback`

**Option A — setup script** (creates `.env`, asks for secrets when possible, starts the stack):

```bash
curl -fsSL https://raw.githubusercontent.com/gabinante/flywheel/main/scripts/flywheel-docker-setup.sh | bash
```

That clones into `$HOME/flywheel` when you run it from `curl`. If you already have the repo: `./scripts/flywheel-docker-setup.sh` from the project root (symlinks are fine).

Script flags: `--ghcr` (use the published image instead of building), `--no-build`. In CI (no terminal), it generates `JWT_SECRET` and leaves GitHub OAuth blank.

**Option B — by hand:** `cp .env.example .env`, edit, then `docker compose up -d`. See `.env.example` and [docs/deployment.md](docs/deployment.md).

## After it’s running

- **Health:** `curl -s http://localhost:8080/healthz` should print `ok`.
- **Browser:** [http://localhost:8080/](http://localhost:8080/) — the SPA uses hash routes (`/#/…`) so paths like `/orgs` stay the REST API.
- **MCP:** `http://localhost:8080/mcp` — see [docs/cursor-mcp.md](docs/cursor-mcp.md).

If you leave GitHub OAuth empty, sign-in is off and some endpoints return 401.

## Hacking on the web app

```bash
cd web && npm install && npm run dev
```

Vite defaults to port **5173** and proxies API calls to `127.0.0.1:8080` (change with `VITE_API_PROXY` if your API isn’t there). After editing `api/openapi.yaml`, run `npm run gen:api` in `web/`. To run the Go server with a production UI build: `make web-build` first.

## Binary releases

[GitHub Releases](https://github.com/gabinante/flywheel/releases) has prebuilt binaries. Container: `ghcr.io/gabinante/flywheel:latest` (you supply Postgres/Redis, or use the compose files in this repo).

## Tests

`make test` (no database required). Run `make web-build` first if tests need a current `web/dist`. After OpenAPI changes: `make generate` and, in `web/`, `npm run gen:api`.

## Config

Everything lives in `.env.example` with comments. The usual suspects: `PORT`, `DATABASE_URL`, `REDIS_URL`, and for OAuth: `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `JWT_SECRET`. Dispatch now splits **runner** from **driver**:

- `DISPATCH_AGENT_RUNNER=cli|docker|openai-responses|openai-compatible`
- `DISPATCH_AGENT_DRIVER=claude|codex|generic` for CLI or Docker harnesses
- `DISPATCH_AGENT_API_KEY` as the generic explicit credential, with `OPENAI_API_KEY` as the fallback for Codex/OpenAI-compatible runners and `ANTHROPIC_API_KEY` retained as a Claude fallback
- `ORCHESTRATOR_AGENT_*` to give the command-center chat its own stronger planner profile without forcing the background dispatch workers onto the same model or runner

A common setup is a strong command-center orchestrator (`openai-responses` with `xhigh`, or your preferred Claude/Codex harness) paired with cheaper `DISPATCH_*` workers for implementation tickets. For local gateways like LiteLLM or vLLM, use `openai-compatible` with `DISPATCH_AGENT_API_BASE_URL` pointing at the local `/v1` endpoint and an explicit `DISPATCH_AGENT_MODEL`.

More in [docs/deployment.md](docs/deployment.md) and [internal/dispatch/DRIVERS.md](internal/dispatch/DRIVERS.md).

## API surface

- **REST** — [api/openapi.yaml](api/openapi.yaml); `make generate` after edits. Errors: [docs/structured-errors.md](docs/structured-errors.md).
- **MCP** — [docs/cursor-mcp.md](docs/cursor-mcp.md), [docs/interacting.md](docs/interacting.md); resource `flywheel://docs/agent-guide` for in-app help.
- **Git notes** — [docs/git-notes.md](docs/git-notes.md), [docs/git-integration-design.md](docs/git-integration-design.md). Builds: `make build-flywheel-git` → `./flywheel-git`, `make build-flywheel-mcp` → `./flywheel-mcp`.

## How this project was built

Flywheel was developed using **agentic engineering**: ideation, architecture, and review were led by an experienced engineer, the system is **heavily tested**, and most of the **implementation was written by large language models** working in that loop. We state this for transparency—evaluate the code and tests the same way you would any other dependency you ship.

## License

[BSL 1.1](LICENSE): free for non-production use until the change date, then GPL-2.0-or-later. Production use needs a commercial license from the licensor.
