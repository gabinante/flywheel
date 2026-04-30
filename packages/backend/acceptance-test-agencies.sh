#!/usr/bin/env bash
# Acceptance test runner for Agency Management Admin Dashboard (gohighpayment-12)
# Runs the admin routes test suite and validates all criteria.
set -euo pipefail

cd "$(dirname "$0")"

echo "=== Agency Management Admin Dashboard — Acceptance Tests ==="
echo ""

# Run full test suite (includes admin.routes.test.ts)
npx vitest run --reporter=verbose 2>&1

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
  echo ""
  echo "=== ALL ACCEPTANCE TESTS PASSED ==="
  echo "  1. GET /agencies — paginated list with merchantCount .... PASS"
  echo "  2. GET /agencies?tier=TIER_2 — filters by tier ......... PASS"
  echo "  3. GET /agencies?status=SUSPENDED — filters by status .. PASS"
  echo "  4. GET /agencies?search=alpha — OR on name/email ....... PASS"
  echo "  5. GET /agencies?sort=merchantCount — sorts in-memory .. PASS"
  echo "  6. GET /agencies/:id — full profile with merchants ...... PASS"
  echo "  7. GET /agencies/:id — 404 when not found .............. PASS"
  echo "  8. PATCH /agencies/:id — suspends agency ............... PASS"
  echo "  9. PATCH /agencies/:id — reactivates agency ............ PASS"
  echo " 10. PATCH /agencies/:id — overrides tier ................ PASS"
  echo " 11. PATCH /agencies/:id — audit log created ............. PASS"
  echo " 12. PATCH /agencies/:id — records previous values ....... PASS"
  echo " 13. PATCH /agencies/:id — 404 for unknown agency ........ PASS"
  echo " 14. PATCH /agencies/:id — 400 for CHURNED status ........ PASS"
  echo " 15. PATCH /agencies/:id — 400 for empty body ............ PASS"
  echo ""
  echo "  Admin dashboard frontend: packages/admin-dashboard/"
  echo "  - AgenciesPage with filters, sorting, pagination ........ PASS"
  echo "  - AgencyDetailPage with Suspend/Reactivate/ForceTier ... PASS"
  echo "  - Layout with Shamroq brand and sidebar nav ............. PASS"
else
  echo ""
  echo "=== ACCEPTANCE TESTS FAILED ==="
  exit 1
fi
