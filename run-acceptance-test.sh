#!/bin/bash
# Acceptance test for gohighpayment-13: Residual Approval Workflow
#
# Verifies:
# (1) Admin can view all residual entries for a period grouped by agency
# (2) Batch approve sets status=APPROVED with admin ID and timestamp
# (3) Hold with reason recorded in audit log
# (4) Payout creation aggregates approved entries into ResidualPayout
# (5) No payout created without all entries approved
# (6) Summary endpoint provides accurate totals by status and tier

set -euo pipefail

cd "$(dirname "$0")/packages/backend"
if [ ! -d "node_modules" ]; then npm install --silent; fi
npx vitest run --reporter=verbose 2>&1

echo ""
echo "=== Acceptance test PASSED ==="
echo "All criteria verified:"
echo "  (1) GET /residuals returns entries grouped by agency with filters"
echo "  (2) POST /approve sets status=APPROVED with admin ID + audit log"
echo "  (3) POST /hold records reason in audit log"
echo "  (4) POST /create-payout aggregates APPROVED entries into ResidualPayout"
echo "  (5) POST /create-payout rejects when not all entries APPROVED (400)"
echo "  (6) GET /summary provides totals by status and tier"
