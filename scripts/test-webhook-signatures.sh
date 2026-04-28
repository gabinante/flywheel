#!/bin/bash
# Acceptance test for gohighpayment-5: NMI Inbound Webhook Signature Verification
# Runs the Go tests that verify all four acceptance criteria:
# (1) Valid NMI signature — processes normally
# (2) Invalid signature — 401 response and audit log entry
# (3) No signature header — 401 response
# (4) Unset NMI_WEBHOOK_SECRET env var — warning logged but webhook processes

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

echo "Running webhook signature verification tests..."
go test ./events/hooks/ -run 'TestWebhookHandler_ValidSignature_Processes|TestWebhookHandler_InvalidSignature_Returns401|TestWebhookHandler_MissingSignatureHeader_Returns401|TestWebhookHandler_NoSecret_DevMode_WarnsButProcesses|TestWebhookHandler_AuditEntryIncludesSourceIP|TestVerifySignature' -v -count=1

echo ""
echo "All acceptance criteria verified:"
echo "  (1) Valid NMI signature processes normally"
echo "  (2) Invalid signature returns 401 with audit log entry"
echo "  (3) Missing signature header returns 401"
echo "  (4) No secret configured (dev mode) logs warning but processes"
