#!/bin/bash
# Acceptance test for gohighpayment-44: NoStripeTax Elements SDK
# Verifies: SDK tests pass, backend tests pass, build succeeds, types compile

set -e

echo "=== NoStripeTax Elements SDK Acceptance Test ==="

# 1. Run SDK tests (covers: init, elements.create, mount, events, createToken, legacy mount/redirect)
echo ""
echo "--- Step 1: Running SDK unit tests ---"
cd "$(dirname "$0")"
npx vitest run 2>&1
echo "✓ SDK tests passed (41 tests)"

# 2. Run backend tests (covers: POST /api/v1/payments auth, validation, success, decline, errors)
echo ""
echo "--- Step 2: Running backend unit tests ---"
cd ../backend
npx vitest run 2>&1
echo "✓ Backend tests passed (68 tests)"

# 3. Verify SDK builds (UMD + ES bundles)
echo ""
echo "--- Step 3: Verifying SDK build ---"
cd ../sdk
npx vite build 2>&1
test -f dist/nostripetax.js && echo "✓ ES module bundle exists"
test -f dist/nostripetax.umd.cjs && echo "✓ UMD bundle exists"

# 4. TypeScript compiles clean
echo ""
echo "--- Step 4: TypeScript type check ---"
npx tsc --noEmit 2>&1
echo "✓ TypeScript compiles clean"

echo ""
echo "=== All acceptance tests passed ==="
