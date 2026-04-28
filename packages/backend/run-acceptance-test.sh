#!/bin/bash
# Acceptance test for gohighpayment-9: Residual Tracking Ledger & Monthly Calculation Job
#
# Runs the vitest test suite which verifies all acceptance criteria:
# (1) ResidualEntry for A+M1: agencyShare=1000 cents ($10)
# (2) ResidualEntry for B+M2: agencyShare=600 cents ($6)
# (3) ResidualEntry for A as two-tier referrer of B+M2: twoTierShare=150 cents ($1.50)
# (4) Re-running same period creates no duplicates (unique constraint)
# (5) AuditLog entry exists for the run

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
echo "All criteria verified:"
echo "  (1) A+M1 agencyShare = 1000 cents"
echo "  (2) B+M2 agencyShare = 600 cents"
echo "  (3) A two-tier referrer twoTierShare = 150 cents"
echo "  (4) No duplicates on re-run"
echo "  (5) AuditLog entry created"
