-- Add webhook_secret to projects for outbound webhook HMAC-SHA256 signing.
-- 32-byte hex string, generated on project creation or backfilled.
ALTER TABLE projects ADD COLUMN webhook_secret TEXT;

-- Add 'webhook' as a valid notification channel.
-- Update the channel CHECK constraint on notifications to include 'webhook'.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_channel_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_check
    CHECK (channel IN ('slack', 'email', 'sms', 'webhook'));

-- Add webhook_url to notification_preferences for outbound webhook delivery.
ALTER TABLE notification_preferences ADD COLUMN webhook_url TEXT NOT NULL DEFAULT '';

-- Backfill existing projects with generated secrets.
-- Each project gets a unique random 32-byte hex secret via gen_random_bytes (PG 13+).
UPDATE projects SET webhook_secret = encode(gen_random_bytes(32), 'hex') WHERE webhook_secret IS NULL;
