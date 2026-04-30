-- Add ghl_order_id to transactions for GHL reconciliation
ALTER TABLE "transactions" ADD COLUMN IF NOT EXISTS "ghl_order_id" TEXT;
