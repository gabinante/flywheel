#!/bin/bash
# Acceptance test for gohighpayment-12: Agency Management in Admin Dashboard
# Validates: backend endpoints exist, tests pass, frontend files exist

set -e

echo "=== Acceptance Test: Agency Management in Admin Dashboard ==="

# 1. Verify backend admin agency routes exist
echo "1. Checking backend admin routes..."
if grep -q 'GET.*agencies' packages/backend/src/routes/admin.ts && \
   grep -q 'PATCH.*agencies' packages/backend/src/routes/admin.ts; then
  echo "   PASS: Admin agency routes exist (GET /agencies, GET /agencies/:id, PATCH /agencies/:id)"
else
  echo "   FAIL: Admin agency routes missing"
  exit 1
fi

# 2. Verify tests pass
echo "2. Running backend tests..."
cd packages/backend
npm test 2>&1 | tail -5
if [ ${PIPESTATUS[0]} -eq 0 ]; then
  echo "   PASS: All tests pass"
else
  echo "   FAIL: Tests failed"
  exit 1
fi
cd ../..

# 3. Verify admin-dashboard frontend exists
echo "3. Checking admin-dashboard frontend..."
REQUIRED_FILES=(
  "packages/admin-dashboard/src/pages/AgenciesPage.tsx"
  "packages/admin-dashboard/src/pages/AgencyDetailPage.tsx"
  "packages/admin-dashboard/src/components/Layout.tsx"
  "packages/admin-dashboard/src/lib/api.ts"
  "packages/admin-dashboard/src/App.tsx"
)
for f in "${REQUIRED_FILES[@]}"; do
  if [ -f "$f" ]; then
    echo "   PASS: $f exists"
  else
    echo "   FAIL: $f missing"
    exit 1
  fi
done

# 4. Verify AgenciesPage has filter support
echo "4. Checking AgenciesPage filters..."
if grep -q 'tierFilter' packages/admin-dashboard/src/pages/AgenciesPage.tsx && \
   grep -q 'statusFilter' packages/admin-dashboard/src/pages/AgenciesPage.tsx && \
   grep -q 'searchQuery' packages/admin-dashboard/src/pages/AgenciesPage.tsx; then
  echo "   PASS: AgenciesPage has tier, status, and search filters"
else
  echo "   FAIL: AgenciesPage missing filters"
  exit 1
fi

# 5. Verify AgencyDetailPage has tabs
echo "5. Checking AgencyDetailPage tabs..."
if grep -q 'overview' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx && \
   grep -q 'merchants' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx && \
   grep -q 'network' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx && \
   grep -q 'residuals' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx && \
   grep -q 'payouts' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx; then
  echo "   PASS: AgencyDetailPage has all required tabs"
else
  echo "   FAIL: AgencyDetailPage missing tabs"
  exit 1
fi

# 6. Verify suspend/reactivate and tier override
echo "6. Checking admin actions..."
if grep -q 'handleStatusToggle' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx && \
   grep -q 'handleTierOverride' packages/admin-dashboard/src/pages/AgencyDetailPage.tsx; then
  echo "   PASS: Suspend/reactivate and tier override actions present"
else
  echo "   FAIL: Admin actions missing"
  exit 1
fi

# 7. Verify audit log in PATCH endpoint
echo "7. Checking audit logging..."
if grep -q 'auditLog.create' packages/backend/src/routes/admin.ts && \
   grep -q 'AGENCY_SUSPENDED' packages/backend/src/routes/admin.ts && \
   grep -q 'AGENCY_REACTIVATED' packages/backend/src/routes/admin.ts; then
  echo "   PASS: Audit logging for agency status changes"
else
  echo "   FAIL: Audit logging missing"
  exit 1
fi

echo ""
echo "=== ALL ACCEPTANCE TESTS PASSED ==="
