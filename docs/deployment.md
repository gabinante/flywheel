# Runtime & deployment

Warrant runs reliably in Docker Compose and uses the same stack for local dev and future hosted runs.

## Docker Compose (reference deployment)

- **postgres** – Postgres 16 with healthcheck; exposed on 5433 on the host when using Compose.
- **redis** – Redis 7 with healthcheck; exposed on 6379.
- **server** – Builds from the repo; depends on postgres and redis being healthy; uses `env_file: .env` and explicit `DATABASE_URL` / `REDIS_URL` for in-Compose hostnames. Runs migrations on startup (entrypoint runs `migrate up` before starting the app).

Start:

```bash
cp .env.example .env
# Edit .env: set GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, JWT_SECRET if using OAuth
docker compose up -d
```

To use the **pre-built image** instead of building from source:  
`docker compose -f docker-compose.ghcr.yml up -d`  
(standalone file with postgres, redis, and `ghcr.io/gabinante/flywheel:latest`).

Server is at http://localhost:8080.

## Config contract (env vars)

See **.env.example**. Required for a full run:

- **PORT** – Server port (default 8080).
- **DATABASE_URL** – Postgres connection string. Use `postgres:5432` when the server runs inside Compose; use `localhost:5433` when running the server on the host.
- **REDIS_URL** – Redis URL. Use `redis:6379` inside Compose; `localhost:6379` on the host.
- **GITHUB_CLIENT_ID**, **GITHUB_CLIENT_SECRET**, **JWT_SECRET** – Required for GitHub OAuth and MCP URL auth. Leave empty only if you disable auth.

Optional:

- **BASE_URL** – Public base URL (e.g. for OAuth redirects). Defaults to http://localhost:PORT.
- **AUTH_SUCCESS_REDIRECT_URL** – Override post-login redirect; the callback appends **`#token=<jwt>`** (fragment, not query). The default is **`BASE_URL/`** with the same fragment for the embedded SPA.
- **LEASE_TTL_MINUTES** – Queue lease TTL in minutes (default 10).
- **RUN_ACCEPTANCE_TEST_ON_SUBMIT** – If `true`, when a ticket has `objective.acceptance_test` (e.g. a shell command), the server runs it on **submit_ticket**; on failure the submit is rejected with the command output so the agent can fix and retry. Default `false`.
- **WEB_DIST** – Directory with the Vite/React production build (`index.html` and `assets/`). Default `web/dist`. If `index.html` is missing, the server skips mounting the web UI (REST and MCP still work). The **Dockerfile** builds `web/` and copies the output to `/app/web/dist` in the image.

### Web UI (browser)

- **Same-origin production:** The server serves the SPA from `/` and static chunks from `/assets/*`. The app uses **hash-based client routes** (`/#/orgs`, `/#/orgs/{id}/projects`, …) so first-party REST paths such as `/orgs` and `/projects/{id}/tickets` are not shadowed by the frontend router.
- **Local dev:** `cd web && npm run dev` (Vite, default port 5173). **vite.config.ts** proxies API prefixes (`/orgs`, `/projects`, `/tickets`, `/agents`, `/auth`, `/oauth`, `/mcp`, `/.well-known`, `/healthz`, `/me`) to **`http://127.0.0.1:8080`**. Point elsewhere with env **`VITE_API_PROXY`** (e.g. another port or host).
- **OpenAPI → TypeScript:** From `web/`, run **`npm run gen:api`** after editing **api/openapi.yaml** to regenerate **`web/src/lib/api/v1.d.ts`** (openapi-typescript). The UI calls the API through **openapi-fetch** typed with that file.

## Security

- **Secrets only in env:** No secrets in the repo or in the image. Use **.env** (not committed; copy from `.env.example`) for local dev. For production or hosted runs, inject **GITHUB_CLIENT_ID**, **GITHUB_CLIENT_SECRET**, **JWT_SECRET**, **DATABASE_URL**, and **REDIS_URL** from a vault or your provider. Never bake secrets into the image or commit them.
- **HTTPS-ready (production/hosted):** The app serves **HTTP** only. For production, put **TLS termination in front** (reverse proxy, load balancer, or ingress). The server listens on PORT; the proxy terminates TLS and forwards to the app. Set **BASE_URL** to the public HTTPS URL (e.g. `https://warrant.example.com`) so OAuth redirects and MCP URL auth work. No code change required; only deploy topology and env.
- **CORS:** The built-in **web UI** is served from the **same origin** as the API (`GET /`, `GET /assets/*`), so the browser does not need CORS for that layout. For **Vite local dev**, the dev server proxies API routes to the Go server (see above), which also avoids CORS. If you host static files on a **different origin** without a proxy (e.g. a CDN) while the API stays on another host, add **CORS** middleware with an allowlist (e.g. env-driven `CORS_ORIGINS`) and document it in `.env.example`.
- **Rate limiting:** For a future hosted deployment, rate limiting belongs in **middleware** (per-IP or per-agent) or in the **proxy/load balancer** (e.g. nginx, cloud LB). The app does not implement rate limiting today; document that it should be added at the edge or in a middleware layer when scaling to multi-tenant hosted use.

## Auth & multi-tenancy

REST endpoints that return or mutate tenant data (orgs, projects, tickets, queue, trace, reviews) require authentication (Bearer JWT or X-API-Key) and OAuth-linked agents (user identity). Access is scoped by **org membership**: the caller’s user must be a member of the org that owns the resource (project → org, ticket → project → org). There is **no admin bypass** without an explicit admin role; the data model supports roles (owner, admin, member) in `org_members` for future collaboration and invite flows.

**Org creation without OAuth:** If an agent is authenticated via API key but not linked to a user (no OAuth), **POST /orgs** still creates an org, but the creating agent is **not** added to `org_members`. That agent will not see the org in **GET /orgs** (which requires OAuth) and will not pass `EnsureOrgAccess` for that org. Use OAuth-linked agents for normal org/project workflows; API-key-only org creation is for automation where the caller does not need to list or access the org via membership (e.g. creating a shell org by ID for later use).

**Project status:** Projects have a status **active** (default) or **closed**. **GET /orgs/{id}/projects** and MCP **list_projects** return only active projects by default; use query `?status=closed` or `?status=all` (REST) or **include_closed: true** (MCP) to include closed. **PATCH /projects/{id}** with `{"status": "active"|"closed"}` or MCP **update_project_status** to close or reopen. Creating tickets and claiming from the queue are **rejected** for closed projects (code `project_closed`). See **docs/structured-errors.md**.

## Observability

- **Request ID:** Every request gets a unique **request_id** (chi middleware). It is set in the response header `X-Request-Id` and is available for logging and tracing. Use it to correlate logs and errors for a single request.
- **Logging:** The server uses chi’s request logger (key=value style) for each request. Log lines include method, path, status, duration, and request_id. For JSON-structured logs in production, replace or wrap the logger with a handler that outputs JSON (e.g. slog with JSON handler) and include request_id in each log record.
- **Error handling:** API error responses use a **consistent structured shape** (JSON with `error`, `code`, `retriable`). **No stack traces** are ever returned in responses; internal errors are logged server-side only. See **docs/structured-errors.md** for codes and client behavior.
- **Metrics:** **GET /metrics** returns 200 with a placeholder body. Wire a Prometheus exporter or other metrics implementation here when you need request counts, latency histograms, or business metrics. For hosted runs, you can also expose metrics from a separate middleware or path.

## Health checks

- **GET /healthz** – Liveness: returns 200 and `ok` if the HTTP server is up. Use this for orchestrators (e.g. Docker, Kubernetes) and load balancers.
- Optional deep health (e.g. ping DB and Redis) can be added later (e.g. `GET /healthz?deep=1`) for readiness; the server does not depend on it today.

## Graceful shutdown

The server listens for **SIGTERM** and **SIGINT**. On receipt it stops accepting new requests, waits up to 10 seconds for in-flight requests to finish, then exits. Use this in Docker (e.g. `docker stop`) and in production so deploys and restarts drain cleanly.

## Data & persistence

### Migrations

- Migrations are **versioned and ordered** in **db/migrations/** (e.g. `000001_initial_schema.up.sql`, `000002_github_oauth.up.sql`). In Docker Compose, migrations run automatically in the server container. For non-Docker (hosted), use **make migrate** or equivalent before starting the app.
- **New deploys:** In Docker Compose, migrations run on every server start. In hosted: run migrations as a one-off job or init container before starting the app. Do not run migrations from multiple app instances concurrently.
- **No destructive defaults:** Migrations and scripts do not use defaults that destroy data (e.g. no unconditional DROP COLUMN or TRUNCATE in normal up migrations). Down migrations exist for rollback and explicitly drop objects in reverse dependency order.

### Postgres backup & restore

- **Backup:** Use `pg_dump` (or your provider’s backup). Example for a single database:  
  `pg_dump -U warrant -d warrant -Fc -f warrant_backup.dump`  
  With Docker: `docker compose exec postgres pg_dump -U warrant -d warrant -Fc > warrant_backup.dump`
- **Restore:** Use `pg_restore` (or provider restore). Example:  
  `pg_restore -U warrant -d warrant --clean --if-exists warrant_backup.dump`  
  Run against an empty or existing DB as needed; for production, follow your provider’s restore and point-in-time recovery docs.

For local troubleshooting (DB not ready, migrations not run), see **docs/troubleshooting.md**.
