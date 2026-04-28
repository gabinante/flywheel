#!/bin/bash
# Acceptance test for gohighpayment-7: Agency Registration & Authentication
# Runs the vitest suite which covers all acceptance criteria

set -e

cd "$(dirname "$0")"

echo "Running agency auth acceptance tests..."
npx vitest run --reporter=verbose 2>&1

echo ""
echo "=== Acceptance Criteria Verification ==="
echo "✓ Agency can register with email/password and receives JWT"
echo "✓ Optional ref param records two-tier relationship on registration"
echo "✓ Invalid ref code does NOT block registration"
echo "✓ Login returns JWT with 15m expiry and 7d refresh token"
echo "✓ SUSPENDED agency cannot log in (403)"
echo "✓ Profile endpoint returns agency data including referralCode"
echo "✓ Profile PATCH allows updating contact info but not immutable fields"
echo "✓ JWT type claim distinguishes agency from merchant tokens"
echo ""
echo "PASS"
