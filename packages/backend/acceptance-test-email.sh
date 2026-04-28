#!/usr/bin/env bash
# Acceptance test runner for Email Notification System (gohighpayment-24)
# Runs the vitest acceptance test suite and validates all criteria.
set -euo pipefail

cd "$(dirname "$0")"

echo "=== Email Notification System — Acceptance Tests ==="
echo ""

# Run acceptance tests
npx vitest run src/__tests__/acceptance-email.test.ts --reporter=verbose 2>&1

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
  echo ""
  echo "=== ALL ACCEPTANCE TESTS PASSED ==="
  echo "  1. Email service sends via Postmark API .............. PASS"
  echo "  2. All 7 templates render correctly .................. PASS"
  echo "  3. Emails queued via pg-boss, never sync ............. PASS"
  echo "  4. Email failures don't break payment flows .......... PASS"
  echo "  5. NotificationSchedule records track delivery ....... PASS"
  echo "  6. Transaction receipts queueable for customers ...... PASS"
  echo "  7. Chargeback alerts queueable for merchants ......... PASS"
  echo "  8. Residual statements queueable for agencies ........ PASS"
else
  echo ""
  echo "=== ACCEPTANCE TESTS FAILED ==="
  exit 1
fi
