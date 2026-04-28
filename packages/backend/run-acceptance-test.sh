#!/bin/bash
# Acceptance test for gohighpayment-13: Residual Approval Workflow
#
# Runs the vitest test suite which verifies all acceptance criteria:
#
# Approval workflow (gohighpayment-13):
# (1) Admin can view residual entries for a period grouped by agency
# (2) Batch approve sets status=APPROVED with admin ID and timestamp
# (3) Hold with reason recorded in audit log
# (4) Payout creation aggregates approved entries into ResidualPayout
# (5) No payout created without all entries approved
# (6) Summary endpoint provides accurate totals by status and tier
#
# Residual calculation (from dependency gohighpayment-9):
# (7) ResidualEntry for A+M1: agencyShare=1000 cents ($10)
# (8) ResidualEntry for B+M2: agencyShare=600 cents ($6)
# (9) Two-tier referrer share=150 cents ($1.50)
# (10) No duplicates on re-run (unique constraint)
# (11) AuditLog entry created

set -euo pipefail

cd "$(dirname "$0")"

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
  npm install --silent
fi

# Run the test suite
npx vitest run --reporter=verbose 2>&1

echo ""
echo "=== Acceptance test PASSED ==="
echo "All gohighpayment-13 criteria verified:"
echo "  (1) GET /residuals returns entries grouped by agency"
echo "  (2) POST /residuals/approve sets APPROVED + adminId + timestamp"
echo "  (3) POST /residuals/hold records reason in audit log"
echo "  (4) POST /residuals/create-payout aggregates into ResidualPayout"
echo "  (5) Payout rejected when not all entries APPROVED"
echo "  (6) GET /residuals/summary returns accurate totals by status and tier"
