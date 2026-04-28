#!/usr/bin/env bash
set -euo pipefail

echo "=== NoStripeTax Migration Acceptance Tests ==="

# Start a temporary PostgreSQL container
CONTAINER_NAME="gohighpayment-test-$$"
echo "Starting PostgreSQL container: $CONTAINER_NAME"
docker run --name "$CONTAINER_NAME" \
  -e POSTGRES_USER=gohighpayment \
  -e POSTGRES_PASSWORD=gohighpayment \
  -e POSTGRES_DB=gohighpayment \
  -p 5435:5432 -d postgres:16-alpine >/dev/null 2>&1

cleanup() {
  echo "Cleaning up container..."
  docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL..."
for i in $(seq 1 30); do
  if docker exec "$CONTAINER_NAME" pg_isready -U gohighpayment >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

export DATABASE_URL="postgresql://gohighpayment:gohighpayment@localhost:5435/gohighpayment?schema=public"

cd "$(dirname "$0")"

# Install deps if needed
if [ ! -d node_modules ]; then
  npm install --silent 2>&1
fi

# (1) Migration applies without errors
echo ""
echo "Test 1: prisma migrate deploy runs cleanly..."
npx prisma migrate deploy 2>&1
echo "PASS: Migration applied without errors"

# (2) SELECT * FROM Agency returns empty table
echo ""
echo "Test 2: Agency table is empty with correct schema..."
AGENCY_COUNT=$(docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -t -c 'SELECT COUNT(*) FROM "Agency"' | tr -d ' ')
if [ "$AGENCY_COUNT" -eq 0 ]; then
  echo "PASS: Agency table is empty"
else
  echo "FAIL: Agency table has $AGENCY_COUNT rows"
  exit 1
fi

# Verify Agency columns exist
docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -t -c "
  SELECT column_name FROM information_schema.columns
  WHERE table_name = 'Agency' ORDER BY ordinal_position
" | tr -d ' ' | grep -q "referralCode" && echo "PASS: Agency has referralCode column" || { echo "FAIL: Missing referralCode"; exit 1; }

# (3) Merchant table has nullable agencyId
echo ""
echo "Test 3: Merchant has nullable agencyId and attributedAt..."
docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -t -c "
  SELECT column_name, is_nullable FROM information_schema.columns
  WHERE table_name = 'Merchant' AND column_name IN ('agencyId', 'attributedAt')
  ORDER BY column_name
" | grep -q "agencyId" && echo "PASS: agencyId column exists" || { echo "FAIL: Missing agencyId"; exit 1; }
docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -t -c "
  SELECT is_nullable FROM information_schema.columns
  WHERE table_name = 'Merchant' AND column_name = 'agencyId'
" | tr -d ' ' | grep -q "YES" && echo "PASS: agencyId is nullable" || { echo "FAIL: agencyId is not nullable"; exit 1; }

# (4) Insert agency, verify unique constraint on referralCode
echo ""
echo "Test 4: Unique constraint on referralCode..."
docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -c "
  INSERT INTO \"Agency\" (id, name, \"contactEmail\", \"passwordHash\", \"referralCode\", tier, status, \"tierOverride\", \"taxIdOnFile\", \"createdAt\", \"updatedAt\")
  VALUES ('t1', 'Test', 'a@b.com', 'h', 'code1', 'TIER_1', 'ACTIVE', false, false, NOW(), NOW());
" >/dev/null 2>&1

# Capture output (stderr merged) in a variable to avoid pipefail issues
DUP_OUTPUT=$(docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -c "
  INSERT INTO \"Agency\" (id, name, \"contactEmail\", \"passwordHash\", \"referralCode\", tier, status, \"tierOverride\", \"taxIdOnFile\", \"createdAt\", \"updatedAt\")
  VALUES ('t2', 'Test2', 'c@d.com', 'h', 'code1', 'TIER_1', 'ACTIVE', false, false, NOW(), NOW());
" 2>&1 || true)

if echo "$DUP_OUTPUT" | grep -q "duplicate key"; then
  echo "PASS: Duplicate referralCode correctly rejected"
else
  echo "FAIL: Duplicate referralCode was not rejected"
  echo "Output: $DUP_OUTPUT"
  exit 1
fi

# (5) Insert ResidualEntry, verify composite unique constraint
echo ""
echo "Test 5: Composite unique on ResidualEntry..."
docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -c "
  INSERT INTO \"Merchant\" (id, name, email, status, \"createdAt\", \"updatedAt\")
  VALUES ('m1', 'Merch', 'merch@x.com', 'ACTIVE', NOW(), NOW());
  INSERT INTO \"ResidualEntry\" (id, \"agencyId\", \"merchantId\", \"periodStart\", \"periodEnd\", \"merchantVolume\", \"nmiResidualEarned\", \"agencyBps\", \"agencyShare\", \"twoTierShare\", status, \"createdAt\", \"updatedAt\")
  VALUES ('r1', 't1', 'm1', '2026-04-01', '2026-04-30', 100000, 5000, 50, 2500, 0, 'PENDING', NOW(), NOW());
" >/dev/null 2>&1

DUP_RE_OUTPUT=$(docker exec "$CONTAINER_NAME" psql -U gohighpayment -d gohighpayment -c "
  INSERT INTO \"ResidualEntry\" (id, \"agencyId\", \"merchantId\", \"periodStart\", \"periodEnd\", \"merchantVolume\", \"nmiResidualEarned\", \"agencyBps\", \"agencyShare\", \"twoTierShare\", status, \"createdAt\", \"updatedAt\")
  VALUES ('r2', 't1', 'm1', '2026-04-01', '2026-04-30', 200000, 10000, 50, 5000, 0, 'PENDING', NOW(), NOW());
" 2>&1 || true)

if echo "$DUP_RE_OUTPUT" | grep -q "duplicate key"; then
  echo "PASS: Duplicate ResidualEntry correctly rejected"
else
  echo "FAIL: Duplicate ResidualEntry was not rejected"
  echo "Output: $DUP_RE_OUTPUT"
  exit 1
fi

echo ""
echo "=== All acceptance tests PASSED ==="
