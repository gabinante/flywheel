#!/bin/bash
# Acceptance test for gohighpayment-9
set -euo pipefail
cd "$(dirname "$0")/packages/backend"
if [ ! -d "node_modules" ]; then npm install --silent; fi
npx vitest run --reporter=verbose 2>&1
