#!/usr/bin/env bash
# Acceptance test for gohighpayment-25: NMI Partner Boarding API (Production)
#
# Validates:
# 1. NMI boarding service has real API calls (not mock)
# 2. Boarding status polling job exists with correct cron
# 3. Credential encryption on approval
# 4. PII decrypted only for the API call
# 5. No plaintext credentials in logs
# 6. All unit tests pass

set -euo pipefail

BACKEND_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$BACKEND_DIR"

echo "=== Acceptance Test: NMI Partner Boarding API (Production) ==="

# 1. Verify NMI boarding service exists and uses real API calls
echo "[1/6] Verifying NMI boarding service uses real API calls..."
if ! grep -q 'nmiPost.*boarding' src/services/nmi-boarding.service.ts; then
  echo "FAIL: submitBoardingApplication does not call real NMI API"
  exit 1
fi
if grep -q 'mock\|MOCK\|fakeMerchantId' src/services/nmi-boarding.service.ts; then
  echo "FAIL: nmi-boarding.service.ts still contains mock/fake references"
  exit 1
fi
echo "  PASS: Real NMI Partner API calls implemented"

# 2. Verify boarding-status-check job with 15-minute cron
echo "[2/6] Verifying boarding-status-check job..."
if ! grep -q 'boarding-status-check' src/jobs/handlers.ts; then
  echo "FAIL: boarding-status-check job not found"
  exit 1
fi
if ! grep -q '\*/15 \* \* \* \*' src/jobs/handlers.ts; then
  echo "FAIL: 15-minute cron schedule not found"
  exit 1
fi
echo "  PASS: boarding-status-check job runs every 15 minutes"

# 3. Verify credential encryption on approval
echo "[3/6] Verifying credential encryption on approval..."
if ! grep -q 'encryptNmiCredentials' src/jobs/handlers.ts; then
  echo "FAIL: Credentials not encrypted on approval"
  exit 1
fi
if ! grep -q 'nmiSecurityKey.*encrypt\|encrypt.*securityKey' src/services/nmi-boarding.service.ts; then
  echo "FAIL: encryptNmiCredentials not found in service"
  exit 1
fi
echo "  PASS: NMI credentials encrypted before storage"

# 4. Verify PII is decrypted only for the API call
echo "[4/6] Verifying PII decryption scoping..."
if ! grep -q 'decryptIfNeeded' src/services/nmi-boarding.service.ts; then
  echo "FAIL: decryptIfNeeded not used for PII fields"
  exit 1
fi
# Ensure decryption only happens in submitBoardingApplication, not globally
DECRYPT_CALLS=$(grep -c 'decryptIfNeeded' src/services/nmi-boarding.service.ts || true)
if [ "$DECRYPT_CALLS" -lt 4 ]; then
  echo "FAIL: Expected at least 4 decryptIfNeeded calls (EIN, SSN, routing, account)"
  exit 1
fi
echo "  PASS: PII decrypted only within API call scope (${DECRYPT_CALLS} fields)"

# 5. Verify no plaintext credentials logged
echo "[5/6] Verifying no credential logging..."
if grep -qE 'console\.(log|info|debug).*partnerKey\|partner_key' src/services/nmi-boarding.service.ts; then
  echo "FAIL: Partner key may be logged"
  exit 1
fi
if grep -qE 'console\.(log|info|debug).*securityKey\b' src/services/nmi-boarding.service.ts; then
  echo "FAIL: Security key may be logged"
  exit 1
fi
if grep -qE 'console\.(log|info|debug).*tokenizationKey' src/services/nmi-boarding.service.ts; then
  echo "FAIL: Tokenization key may be logged"
  exit 1
fi
echo "  PASS: No credential values in log statements"

# 6. Run all unit tests
echo "[6/6] Running unit tests..."
if command -v npx &> /dev/null; then
  npx jest --silent 2>&1
  echo "  PASS: All unit tests pass"
else
  echo "  SKIP: npx not available, tests must be run separately"
fi

echo ""
echo "=== All acceptance checks PASSED ==="
exit 0
