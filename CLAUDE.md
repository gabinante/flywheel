# CLAUDE.md

## What this is

Flywheel is a **work queue plus shared context** for software projects — AI agents and humans use the same ticket system. The one-liner: **"Adderall for coding agents."** Agents without structure thrash; agents with structure ship.

## Tech stack

- **Backend:** Go (module `github.com/gabinante/flywheel`)
- **Frontend:** TypeScript + Vite SPA (in `web/`), hash routes (`/#/…`)
- **Database:** Postgres, Redis
- **API:** REST (`api/openapi.yaml`) + MCP (`/mcp`)
- **Infra:** Docker Compose

## Common commands

```bash
make test              # Go tests (no database required)
make generate          # Regenerate from OpenAPI spec
make web-build         # Build frontend (cd web && npm ci && npm run build)
make run               # Start server via varlock (loads .env, validates .env.schema)
make varlock-validate  # Validate .env against .env.schema without starting
make docker-up         # docker compose up -d
make migrate           # Run DB migrations (non-Docker deploys)
cd web && npm run dev  # Vite dev server on :5173, proxies to :8080
cd web && npm run gen:api  # Regenerate TS API client from openapi.yaml
```

## Product posture

We are **not** building a better Linear, Backstage, observability platform, coding agent, Terraform, or secrets manager. We **are** building the unified substrate that lets agents reason about code, infrastructure, and production reality together — the orchestration, state machine, and plan/risk model on top of commodity components.

**Integrate rather than replace.** The operator's existing tools are the ecosystem we plug into. Build new components only when existing tools are overwrought and a minimum-viable built-in is straightforward.

### Don't build these

If you catch yourself starting any of these, stop: a metrics/log/tracing platform, incident management tool, identity provider, fine-tuned LLM, visual workflow editor, full service catalog, CI/CD runner, git hosting platform, or multi-tenancy/billing layer. If a mature tool exists in the ecosystem and operators commonly run it, we integrate.

## Core principles

1. **The ticket is the contract.** The ticket is the unit of work and the contract between agents. If a change doesn't belong to a ticket, it shouldn't happen.
2. **Observable streams + declarative claims.** Reality is append-only streams. Intent is declarative claims. Drift is the gap. When designing a feature, ask: *is this a stream, a claim, or a drift detector?*
3. **Cross-layer entity identity.** Every real thing has a stable ID used across all layers. Reference by ID, never by name or path.
4. **Declared vs observed.** Every fact is either declared (human intent) or observed (system-derived). Preserve the distinction in schemas and UI.
5. **Push, don't wait.** The system contacts the human when decisions are needed. Default to a push channel with a decision, a default action, and a timeout.
6. **Advance as far as policy permits.** Auto-advance at each state transition if policy allows. Gates are explicit and configurable.
7. **Integration over ownership.** Default for commodity functionality is to plug into operator tools.
8. **Plugin interfaces are product surface.** Plugin contracts (MCP-based) are designed deliberately, documented, stable.

## Pluggability discipline

When adding or touching a layer, ask: **is this novel, pluggable-with-default, or pluggable-without-default?**

- **Novel** — we own it fully (coordinator, ticket model, change stream schema, plan object, pillar layer). Go deep.
- **Pluggable with default** — we ship a minimum-viable impl but operators can swap (code knowledge, catalog, secrets, event bus, notifications). Keep defaults deliberately minimal. Three questions: (1) minimum contract an agent needs? (2) smallest default satisfying the contract? (3) can an external MCP server replace the default with no code changes?
- **Pluggable without default** — we don't ship anything; operators plug in (observability backend, SSO, LLM provider). The work is the contract and the adapter.

## UI style guide

**Dark mode first.** Design and develop in dark mode. Light mode is a secondary theme, not the default.

### Color palette

- **Backgrounds:** Dark moss greens and dark greys. Layer surfaces with subtle lightness shifts rather than borders.
- **Accents:** Lighter, vibrant greens for interactive elements, active states, and highlights. Use sparingly — accent draws the eye, so it should mark what matters.
- **Text:** Off-white on dark backgrounds. Muted grey for secondary text. Never pure white (#fff) — it's harsh on dark surfaces.
- **Semantic colors:** Destructive/error in muted red. Warnings in amber. Success inherits the green accent.
- **Color space:** OKLCH (already in use). Keep hue consistent across the green family; vary lightness and chroma for hierarchy.

### Visual language

- **Glassmorphism.** Semi-transparent surfaces with backdrop blur for cards, modals, and overlays. Use `backdrop-blur-md` or higher with translucent backgrounds (e.g., `bg-white/5` or `bg-green-950/40`). Layer depth through transparency, not drop shadows.
- **Animations and transitions.** UI should feel alive. State changes, page transitions, and interactive elements should animate smoothly. Use CSS transitions for hover/focus states, Framer Motion or CSS keyframes for entrances and layout shifts. Keep durations snappy (150–300ms) — fluid, not sluggish.
- **Rounded corners.** Generous radii. Cards and containers should feel soft.
- **Spacing.** Breathe. Don't cram elements. Generous padding inside cards, clear separation between sections.
- **Typography.** Geist (already in use). Use font weight and size for hierarchy, not color alone.

### Interaction patterns

- **Keyboard-driven where possible.** Power users shouldn't need a mouse for common operations.
- **Hover states are informative.** Subtle background shifts, not just cursor changes.
- **Loading states.** Skeleton shimmer animations, not spinners. Keep layout stable during loads.
- **Transitions between views.** Cross-fade or slide, not hard cuts.

## Code style

- Prefer boring, readable code over clever code. The system is complex structurally; code shouldn't add cognitive load.
- Go: standard idioms, strong typing.
- TypeScript: strict mode, no `any`, Zod for runtime validation at boundaries.
- Schemas (ticket, plan, change event, entity) are load-bearing product — add fields additively, avoid breaking changes.

## Architecture notes

- **Event-driven, not request-driven.** State transitions emit events. Workers subscribe. Don't write synchronous request-response for long-running work. The UI subscribes to events for real-time updates.
- **Sandboxed workers.** Worker code runs in containers with controlled egress. Don't mount the Docker socket or home directories into workers. Respect time/resource limits.
- **Secrets.** Never read from `.env` directly in application code — use `varlock load` or `varlock run`. Never log secrets. When adding a new integration, add its secret names to the relevant `.env.schema` with proper `@sensitive` annotations, then fetch via varlock. The coordinator should never have raw secret values in context.

## Secrets management (varlock)

varlock (`scripts/varlock`) is the project's secrets-loading tool. It reads `.env`, validates against `.env.schema`, and ensures sensitive values are never printed or logged.

### Usage patterns

```bash
# Local development — start the server with secrets loaded from .env:
make run                            # wraps: varlock run -- go run ./cmd/server

# Validate your .env against the schema without running anything:
make varlock-validate               # wraps: varlock validate

# In Docker Compose — varlock is baked into the image CMD, validating on startup.

# Directly (for scripts):
./scripts/varlock run -- <any-command>
eval "$(./scripts/varlock load)"    # export into current shell (secrets redacted in output)
```

### Adding a new secret

1. Add the variable to `.env.schema` with `@sensitive` and `@required`/`@optional` annotations.
2. Add the variable name (without value) to `.env.example`.
3. Reference via `os.Getenv("KEY")` in Go code (or `config.Load()`) — never read `.env` directly.
4. varlock handles loading the value from `.env` at process start.

### Schema annotations

- `@sensitive` — Value is a secret. varlock will never echo it; reviewers know it needs secure storage.
- `@required` — Server refuses to start if unset (varlock exits non-zero).
- `@optional` — Has a sensible default or is not needed in all environments.

## When you're unsure

- **Build or integrate?** Integrate. Building is a commitment; integrating is reversible.
- **Novel core or plugin?** Plugin. Moving into core later is easier than extracting.
- **Schema shape?** Smaller and additive. We can grow; we can't easily shrink.
- **Needs human approval?** Yes. Loosening is easier than tightening.
- **Which layer?** Stop and ask. Layer boundaries are load-bearing.

## Flag to the human

Proactively call out (don't silently handle):
- Changes to a public schema or plugin contract
- Anything increasing privileged surface beyond what plugins can access
- New dependencies on external services the operator must run
- Anything requiring multi-tenancy reasoning (explicitly deferred)
- Scope creep toward the "don't build these" list
