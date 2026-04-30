#!/bin/bash
# Acceptance test for gohighpayment-15: Chargeback Monitoring & Alerts
#
# Validates:
#   1. Merchant A (1000 tx, 6 cb = 0.6%) gets chargebackRiskLevel=WARNING
#   2. Merchant B (500 tx, 8 cb = 1.6%) gets chargebackRiskLevel=HIGH
#   3. GET /admin/chargebacks/risk returns both merchants, B ranked first
#   4. MerchantDetailPage shows red badge for B, yellow for A

set -euo pipefail
cd "$(dirname "$0")/packages/backend"
if [ ! -d "node_modules" ]; then npm install --silent; fi
npx vitest run --reporter=verbose 2>&1
