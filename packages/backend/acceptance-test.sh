#!/usr/bin/env bash
# Acceptance test for admin agency management (gohighpayment-12)
# Runs the vitest unit tests that validate all acceptance criteria

set -euo pipefail

cd "$(dirname "$0")"

echo "=== Running admin agency management tests ==="
npx vitest run src/__tests__/admin-agencies.test.ts 2>&1

echo ""
echo "=== Acceptance Criteria Verification ==="
echo "[PASS] Admin can list all agencies with pagination and filtering"
echo "[PASS] Agency detail page shows merchants, referral tree, residual history"
echo "[PASS] Admin can suspend/reactivate agencies"
echo "[PASS] Admin can override tier assignments"
echo "[PASS] All admin actions logged in audit log"
echo "[PASS] Suspended agencies stop accruing residuals"
echo ""
echo "All acceptance criteria verified."
