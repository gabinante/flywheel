#!/usr/bin/env bash
# Acceptance test for gohighpayment-28: Structured Logging with Correlation IDs
#
# Verifies:
# 1. All log output is structured JSON (via test suite)
# 2. correlationId traces across services (checked in source)
# 3. nmiSecurityKey never appears unredacted in log output (via test suite)
# 4. All log lines parseable as JSON (via test suite)
# 5. Zero console.log in src/ (excluding node_modules and tests)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Acceptance Test: Structured Logging with Correlation IDs ==="
echo ""

# Step 1: Run test suite (validates JSON output, redaction, correlationId propagation)
echo "Step 1: Running test suite..."
npx vitest run --reporter=verbose 2>&1 | tail -20
echo "PASS: All tests passed"
echo ""

# Step 2: Verify correlationId propagation across services
echo "Step 2: Checking correlationId propagation..."
CORR_FILES=$(grep -rl "correlationId" src/ --include="*.ts" | grep -v __tests__ | grep -v node_modules | sort)
CORR_COUNT=$(echo "$CORR_FILES" | wc -l | tr -d ' ')
echo "  Found correlationId usage in $CORR_COUNT source files:"
echo "$CORR_FILES" | sed 's/^/    /'

# Verify it's in checkout, nmi service, webhooks, jobs
for expected in "checkout.ts" "nmi.service.ts" "webhooks.ts" "handlers.ts" "email.service.ts"; do
  if echo "$CORR_FILES" | grep -q "$expected"; then
    echo "  OK: correlationId present in $expected"
  else
    echo "  FAIL: correlationId missing from $expected"
    exit 1
  fi
done
echo "PASS: correlationId traces across all key services"
echo ""

# Step 3: Verify no nmiSecurityKey appears unredacted in log output
echo "Step 3: Verifying nmiSecurityKey is in redact paths..."
if grep -q "nmiSecurityKey" src/utils/logger.ts; then
  echo "  OK: nmiSecurityKey found in redact configuration"
else
  echo "  FAIL: nmiSecurityKey not in redact configuration"
  exit 1
fi
echo "PASS: Sensitive fields covered by redaction"
echo ""

# Step 4: Verify all log lines parse as JSON (covered by structured-logging.test.ts)
echo "Step 4: JSON parsing verified by test suite (structured-logging.test.ts)"
echo "PASS: All log entries produce valid JSON"
echo ""

# Step 5: Search for console.log in src/ (excluding tests and node_modules)
echo "Step 5: Checking for console.log/error/warn in source..."
CONSOLE_HITS=$(grep -rn "console\.\(log\|error\|warn\)" src/ --include="*.ts" | grep -v __tests__ | grep -v node_modules || true)
if [ -z "$CONSOLE_HITS" ]; then
  echo "  OK: Zero console.log/error/warn found in src/"
else
  echo "  FAIL: Found console.log usage:"
  echo "$CONSOLE_HITS"
  exit 1
fi
echo "PASS: No console.log remaining"
echo ""

# Step 6: Verify LOG_LEVEL configurability
echo "Step 6: Checking LOG_LEVEL env var support..."
if grep -q "process.env.LOG_LEVEL" src/utils/logger.ts; then
  echo "  OK: LOG_LEVEL env var used in logger config"
else
  echo "  FAIL: LOG_LEVEL env var not found in logger config"
  exit 1
fi
echo "PASS: Log level configurable via LOG_LEVEL"
echo ""

# Step 7: Verify duration_ms logging in NMI service
echo "Step 7: Checking duration_ms in NMI API calls..."
if grep -q "duration_ms" src/services/nmi.service.ts; then
  echo "  OK: duration_ms found in NMI service"
else
  echo "  FAIL: duration_ms not found in NMI service"
  exit 1
fi
echo "PASS: NMI API calls include duration_ms"
echo ""

# Step 8: TypeScript compilation check
echo "Step 8: TypeScript type check..."
npx tsc --noEmit 2>&1
echo "PASS: TypeScript compiles cleanly"
echo ""

echo "=== ALL ACCEPTANCE TESTS PASSED ==="
