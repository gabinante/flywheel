-- Add rate_limit_override column to merchants table
-- Stores per-merchant rate limit overrides as JSON: { checkout?: number, read?: number, write?: number }
ALTER TABLE "merchants" ADD COLUMN "rate_limit_override" JSONB;
