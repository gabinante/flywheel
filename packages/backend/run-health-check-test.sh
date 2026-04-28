#!/usr/bin/env bash
# Acceptance test for gohighpayment-29: Health Checks & Sentry Monitoring
#
# Verifies:
# 1. /health/live returns 200
# 2. /health/ready checks Postgres, Redis, pg-boss
# 3. 503 returned when any dependency is down
# 4. Sentry captures unhandled errors with context
# 5. Sentry configurable via SENTRY_DSN env var
# 6. Fly.io health check configs exist
# 7. Frontend packages have Sentry error boundary

set -euo pipefail

PASS=0
FAIL=0
RESULTS=()

check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    RESULTS+=("PASS: $desc")
    PASS=$((PASS + 1))
  else
    RESULTS+=("FAIL: $desc")
    FAIL=$((FAIL + 1))
  fi
}

cd "$(dirname "$0")/../.."

echo "=== gohighpayment-29: Health Checks & Sentry Monitoring ==="
echo ""

# ── 1. Unit tests pass ──────────────────────────────────────────
check "Health check tests pass (14 tests)" \
  bash -c "cd packages/backend && npx vitest run src/__tests__/health.test.ts 2>&1 | grep -q '14 passed'"

check "Sentry tests pass (16 tests)" \
  bash -c "cd packages/backend && npx vitest run src/__tests__/sentry.test.ts 2>&1 | grep -q '16 passed'"

check "All backend tests pass (71 tests)" \
  bash -c "cd packages/backend && npx vitest run 2>&1 | grep -q '71 passed'"

# ── 2. Health route files exist ─────────────────────────────────
check "/health/live endpoint defined" \
  grep -q "health/live" packages/backend/src/routes/health.ts

check "/health/ready endpoint defined" \
  grep -q "health/ready" packages/backend/src/routes/health.ts

check "Postgres check (SELECT 1)" \
  grep -q "SELECT 1" packages/backend/src/routes/health.ts

check "Redis check (PING)" \
  grep -q "redis.ping" packages/backend/src/routes/health.ts

check "pg-boss check (getQueueSize)" \
  grep -q "getQueueSize" packages/backend/src/routes/health.ts

check "503 status for degraded" \
  grep -q "503" packages/backend/src/routes/health.ts

check "Detailed health includes version" \
  grep -q "version" packages/backend/src/routes/health.ts

check "Detailed health includes uptime" \
  grep -q "uptime_seconds" packages/backend/src/routes/health.ts

check "Detailed health includes memory" \
  grep -q "memory" packages/backend/src/routes/health.ts

# ── 3. Sentry backend integration ──────────────────────────────
check "Sentry init in backend index.ts" \
  grep -q "initSentry" packages/backend/src/index.ts

check "Sentry error handler registered" \
  grep -q "registerSentryErrorHandler" packages/backend/src/index.ts

check "Sentry DSN from env" \
  grep -q "SENTRY_DSN" packages/backend/src/utils/sentry.ts

check "Sentry environment tag" \
  grep -q "SENTRY_ENVIRONMENT" packages/backend/src/utils/sentry.ts

check "Sentry captures merchantId context" \
  grep -q "merchantId" packages/backend/src/utils/sentry.ts

check "Sentry captures correlationId context" \
  grep -q "correlationId" packages/backend/src/utils/sentry.ts

check "Sentry captures requestId context" \
  grep -q "requestId" packages/backend/src/utils/sentry.ts

check "@sentry/node in backend dependencies" \
  grep -q "@sentry/node" packages/backend/package.json

check "Sentry tracing helper (sentryTrace) exists" \
  grep -q "sentryTrace" packages/backend/src/utils/sentry.ts

check "Checkout route uses sentryTrace" \
  grep -q "sentryTrace" packages/backend/src/routes/checkout.ts

check "Webhook route uses sentryTrace" \
  grep -q "sentryTrace" packages/backend/src/routes/webhooks.ts

check "Checkout route sets Sentry context" \
  grep -q "setSentryContext" packages/backend/src/routes/checkout.ts

check "Webhook route sets Sentry context" \
  grep -q "setSentryContext" packages/backend/src/routes/webhooks.ts

# ── 4. Sentry frontend integration ─────────────────────────────
for pkg in admin-dashboard merchant-dashboard checkout agency-dashboard; do
  check "$pkg has Sentry init in main.tsx" \
    grep -q "initSentryFrontend" "packages/$pkg/src/main.tsx"

  check "$pkg has SentryErrorBoundary in main.tsx" \
    grep -q "SentryErrorBoundary" "packages/$pkg/src/main.tsx"

  check "$pkg has @sentry/react dependency" \
    grep -q "@sentry/react" "packages/$pkg/package.json"
done

# ── 5. Fly.io health check configs ─────────────────────────────
for svc in backend admin-dashboard merchant-dashboard checkout agency-dashboard; do
  check "$svc fly.toml exists" \
    test -f "deploy/$svc/fly.toml"

  check "$svc fly.toml points to /health/ready" \
    grep -q "/health/ready" "deploy/$svc/fly.toml"

  check "$svc fly.toml grace_period 30s" \
    grep -q '30s' "deploy/$svc/fly.toml"

  check "$svc fly.toml interval 15s" \
    grep -q '15s' "deploy/$svc/fly.toml"

  check "$svc fly.toml timeout 5s" \
    grep -q '5s' "deploy/$svc/fly.toml"
done

# ── 6. .env.example has SENTRY_DSN ─────────────────────────────
check "SENTRY_DSN in .env.example" \
  grep -q "SENTRY_DSN" .env.example

check "VITE_SENTRY_DSN in .env.example" \
  grep -q "VITE_SENTRY_DSN" .env.example

# ── Summary ─────────────────────────────────────────────────────
echo ""
for r in "${RESULTS[@]}"; do echo "  $r"; done
echo ""
echo "Total: $((PASS + FAIL)) | Passed: $PASS | Failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS"
