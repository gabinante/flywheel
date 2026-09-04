# Flywheel

Flywheel is a **local-first control plane for one operator's agentic software delivery**. It sits between
Linear (tickets), GitHub (pull requests), and the local coding harnesses — **Claude Code** and **Codex** — and
keeps the ticket as the contract between the human and the agents.

What it does:

- **Tickets live in Linear.** Flywheel projects map 1:1 to Linear projects you lead (spanning several repos)
  and keep a projection of each issue plus everything Linear cannot hold: sessions, workflow position, review
  findings, agent runs.
- **Code review is its own, PR-keyed workflow.** Paste PR URLs, or let Flywheel pick up `review-requested`
  PRs, run an in-depth review (Codex by default), post inline conversational comments, and approve unless a
  P0/P1 finding blocks. Reviewed PRs are watched for new commits or dismissed reviews; your own PRs are
  watched for landed reviews so an agent can address the comments.
- **Every Claude Code and Codex session is tracked** and linked to the PRs and issues it touched.
- **Workflows dispatch local harnesses.** Codex reviews, Claude Code implements — selectable per phase.

Design and phase plan: [docs/plans/revival-local-first.md](docs/plans/revival-local-first.md).

## Quick start

Requirements: Go (see `go.mod`), Node (see `.nvmrc`), Docker with Compose v2, the `migrate` CLI, and the
authenticated `gh` CLI.

```bash
make dev
```

`make dev` runs the preflight (creates a `.env` with local defaults), starts Postgres and Redis in Docker,
runs migrations, and starts the server. Ports are deliberately non-default so Flywheel never collides with
other local stacks:

| Service  | Port |
|----------|------|
| Server   | 8090 |
| Postgres | 5439 |
| Redis    | 6389 |

Then open [http://localhost:8090](http://localhost:8090) and click **Sign in** — Flywheel provisions the single
local operator and issues a session token. There is no external identity provider.

- Health: `curl -s http://localhost:8090/healthz`
- MCP: `http://localhost:8090/mcp` (Streamable HTTP; `X-API-Key` header)

## Hacking on the web app

```bash
cd web && npm install && npm run dev
```

Vite serves on `5173` and proxies API calls to `127.0.0.1:8090`. To keep browsing through `:8090` with HMR,
start the Go server with `WEB_DEV_PROXY_URL=http://127.0.0.1:5173`. After editing `api/openapi.yaml`, run
`make generate` and `cd web && npm run gen:api`.

## Tests

`make test` (no database required). Web: `cd web && npm test && npm run build`.

## Config

Everything lives in `.env.example` with comments and is validated against `.env.schema` by varlock. The usual
suspects: `PORT`, `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, and `DISPATCH_*` / `ORCHESTRATOR_*` for the
harnesses (`DISPATCH_AGENT_DRIVER=claude|codex|generic`).

## API surface

- **REST** — [api/openapi.yaml](api/openapi.yaml); `make generate` after edits. Errors:
  [docs/structured-errors.md](docs/structured-errors.md).
- **MCP** — [docs/interacting.md](docs/interacting.md); resource `flywheel://docs/agent-guide` for in-app help.

## How this project was built

Flywheel was developed using **agentic engineering**: ideation, architecture, and review were led by an
experienced engineer, the system is heavily tested, and most of the implementation was written by large
language models working in that loop. Evaluate the code and tests the same way you would any other dependency.

## License

[BSL 1.1](LICENSE): free for non-production use until the change date, then GPL-2.0-or-later. Production use
needs a commercial license from the licensor.
