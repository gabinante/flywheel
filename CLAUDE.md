# CLAUDE.md — guidance for AI agents in this repo

**Product:** **Shamroq** — NMI ISO referral partner platform. We board merchants onto NMI via the Partner Boarding API, process payments through Collect.js (SAQ-A PCI scope), and share residuals with referring agencies. Rebranding from GoHighPayment is in progress.

## Project shape

- **Monorepo** (`npm` workspaces): `packages/backend` (Fastify API, **port 3000**), `packages/admin-dashboard` (React/Vite, **port 5174**), `packages/merchant-dashboard` (React/Vite, **port 5175**), `packages/checkout` (React/Vite, **port 5176**), `packages/sdk` (vanilla TS, UMD + ESM bundle).
- **`make dev`** starts Postgres + Redis via Docker Compose, then runs backend + all 3 frontends concurrently.
- **Database:** PostgreSQL on port 5433 (localhost), Redis on 6379. Prisma ORM with schema at `prisma/schema.prisma`.
- **Job queue:** pg-boss for async work (webhook delivery, residual calculation, NMI sync, email sending).
- **Path aliases:** `@gohighpayment/*` maps to `packages/*/src` via root tsconfig.

## Playwright MCP

**Always use Playwright MCP to test changes whenever any changes are made that can be tested via the web UI.** After making frontend changes to any package under `packages/`, open the relevant dev server in the browser and verify the changes visually and interactively before considering the task complete.

Use the **Playwright MCP** tools for:
- Loading pages and verifying rendered output after frontend changes
- Clicking through onboarding flows, checkout, and dashboard interactions
- Asserting UI state (element visibility, text content, form validation)
- Capturing screenshots for visual verification
- Testing embed/iframe behavior (SDK mount, checkout embed mode)
- Testing responsive layouts (use `browser_resize` for mobile viewports)

**Dev server URLs for Playwright:**
- Admin dashboard: `http://localhost:5174`
- Merchant dashboard: `http://localhost:5175`
- Checkout: `http://localhost:5176`
- Backend API: `http://localhost:3000`

## UI Design & Styling

- **Glassmorphism is the primary design language.** All cards, modals, panels, and elevated surfaces should use glass-effect styling: semi-transparent backgrounds with `backdrop-filter: blur()`, subtle borders, and soft shadows. This applies across all 3 frontend apps.
- **Animations are critical to the product feel.** Every state change, step transition, list load, card entrance, and form interaction should be animated. Not flashy — clean, subtle, and responsive. If something pops in without a transition, it's a bug.
  - Use **Framer Motion** (`motion.div` with `initial`/`animate`/`exit`) for entrance animations, page transitions, stagger effects, and layout animations.
  - Step/wizard transitions: slide in the direction of navigation (forward = slide from right, back = slide from left) with a fade. ~200-300ms timing.
  - List entrances: stagger cards with increasing `delay` (50-80ms per item). Fade + `translateY: 12` to `0`.
  - Loading to content: crossfade from skeleton placeholders to real content. Never show a blank screen or a raw spinner — always show skeletons shaped like the content that's coming.
  - Button press effects: subtle `scale(0.97)` on press with CSS transitions.
  - Form validation: slide-down animation for error messages, green checkmark fade-in for valid fields.
- **Color palette:** Primary blue `#3B82F6`, with a modern gradient blue for CTAs (`linear-gradient(135deg, #3B82F6, #2563EB)`). Background: light blue-gray `#F0F4FF` for page backgrounds, white `#FFFFFF` for cards. Text: `#1F2937` primary, `#6B7280` secondary. Success green `#10B981`, error red `#EF4444`.
- **Glass card recipe:**
  ```css
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.3);
  border-radius: 16px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.06);
  ```
- **Typography:** System font stack (`Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`). Use Tailwind's font-weight utilities. Headings are `font-semibold` or `font-bold`.
- **Styling framework:** Tailwind CSS across all frontend packages. Use Tailwind utilities; avoid inline styles except for dynamic values.
- **Loading states:** Use skeleton placeholders (`animate-pulse` with gray shapes matching content layout), never raw spinners.
- **Mobile-first:** All layouts must work on 375px viewport width. Use Tailwind responsive prefixes (`sm:`, `md:`, `lg:`).

## Key services & integrations

- **NMI** — Payment gateway. Per-merchant `nmiSecurityKey` and `nmiTokenizationKey`. Collect.js handles PCI-compliant card tokenization in the browser (SAQ-A scope — card data never touches our servers). Partner Boarding API for merchant onboarding.
- **Seamlesschex** — ACH/eCheck processing. Per-merchant `seamlesschexApiKey`.
- **GoHighLevel (GHL)** — CRM integration. OAuth install flow, SSO, Custom Payment Provider (queryUrl). GHL endpoints at `services.leadconnectorhq.com`.
- **pg-boss** — PostgreSQL-backed job queue. Handlers in `packages/backend/src/jobs/`.
- **Prisma** — ORM. Schema at `prisma/schema.prisma`. Generate: `npm run db:generate`. Migrate: `npm run db:migrate`.

## Business model context

We are an **NMI ISO referral partner**, NOT a payment facilitator or money transmitter. NMI handles all underwriting, settlement, and fund movement. Our platform:
1. Refers merchants to NMI via the Partner Boarding API
2. NMI settles directly to each merchant's bank account
3. NMI pays us residuals based on processing volume
4. We share a cut of those residuals with referring agencies (two-tier affiliate system)

**Critical rules:**
- Attribution is **immutable** — once a merchant is linked to an agency, it cannot be reassigned
- Residual ledger is **append-only** — never update finalized entries, create adjustment entries instead
- Per-merchant NMI credentials must be **encrypted at rest** (AES-256-GCM via `ENCRYPTION_KEY`)

## Docker / local development

- `make dev` brings up Compose (Postgres on 5433, Redis on 6379) then starts all dev servers concurrently.
- `make dev-backend` / `make dev-admin` / `make dev-merchant` / `make dev-checkout` for individual services.
- `make db-up` / `make db-down` for database services only.
- `make db-migrate` / `make db-push` / `make db-seed` for Prisma operations.
- API health: `GET http://localhost:3000/health`.
- Backend entry: `packages/backend/src/index.ts` — Fastify server with `tsx watch`.

## Deployment

- **Fly.io** hosts all services. Dev apps prefixed `ghp-dev-*`, production: `ghp-*` (to be renamed to `smq-*` after rebrand).
- Fly configs in `deploy/dev/` and `deploy/prod/` — separate `fly.*.toml` per service.
- `make deploy-dev` / `make deploy-prod` deploys all services. `make deploy-dev-api` for API only.
- `deploy/setup-fly.sh` creates Fly apps + Postgres + Redis + secrets from scratch.
- **Backend Dockerfile** (`deploy/Dockerfile.backend`): multi-stage, node:20-alpine, Prisma generate, compiles TS.
- **Frontend Dockerfile** (`deploy/Dockerfile.frontend`): multi-stage, Vite build, nginx runner. ARGs for PACKAGE and PORT.
- Production URLs: `*.gohighpayment.com` (will become `*.shamroq.com`).

## Env var naming

- Server-side (API): unprefixed (`DATABASE_URL`, `JWT_SECRET`, `NMI_PARTNER_KEY`, `ENCRYPTION_KEY`, etc.)
- Frontend apps: `VITE_*` prefix for anything bundled into the client (`VITE_API_URL`, etc.)
- See `.env.example` for the full list with documentation.
- **Do not** commit `.env` files. Use `.env.example` as reference.

## Prisma migrations

- **Never generate migrations with `prisma migrate dev` if manual migrations already cover the same schema change.** Prisma's auto-generated migrations include all pending schema drift, duplicating column additions from earlier manual migrations.
- **One migration per schema change.** Inspect SQL before committing to ensure no duplicate `ALTER TABLE` or `CREATE TABLE` statements.
- **After creating any migration**, run `npm run db:migrate` locally to verify it applies cleanly.

## Testing

- **Playwright MCP is the primary testing method for UI changes.** Always verify visually.
- Backend: no test framework yet (planned in WS8). Validate API behavior via curl or Playwright network inspection.
- **Test cards for NMI sandbox:** `4111111111111111` (Visa approve), `4000000000000002` (decline).
- When `NMI_MOCK_MODE=true`, boarding service returns mock IDs that never resolve — useful for UI development but not integration testing.

## Git workflow

- **Pre-alpha stage:** All work happens directly on the `main` branch. No feature branches, no PRs. Commit and push directly to `main`.

## House rules

- Prefer **absolute paths** in tool calls.
- Do not commit secrets; use `.env` patterns from `.env.example`.
- **Rebrand in progress:** Code still references `GoHighPayment` / `gohighpayment` in many places. The target brand is **Shamroq** / `shamroq`. Update branding whenever touching affected files.
- **SAQ-A PCI compliance:** Card data must NEVER touch our servers. All card capture goes through Collect.js (NMI's hosted iframe). Never log, store, or transmit raw card numbers, CVVs, or full track data.
- **Encryption:** Sensitive fields (NMI keys, Seamlesschex keys, SSN, bank account numbers) must be encrypted at rest using AES-256-GCM before storage. The `ENCRYPTION_KEY` env var provides the 32-byte key.
