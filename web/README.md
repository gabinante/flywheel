# Flywheel web UI

React, TypeScript, and Vite. See the [root README](../README.md) for the local server and database setup.

```bash
npm ci
npm run dev       # Vite on :5173, API proxy to :8090
npm test
npm run lint
npm run build
```

The server serves the production build from `web/dist`. API types come from `../api/openapi.yaml`; regenerate them with `npm run gen:api`.

Browser tests live in `e2e/`. Run `./scripts/test-hardening.sh` from the repository root after installing Playwright Chromium. The script provisions an isolated server and disposable databases; do not point mutation tests at your real workspace.

Live activity uses the shared `ActivityProvider`. Refresh query data on scoped activity notices while preserving local editor drafts. See [the activity pattern](../docs/activity-stream.md).
