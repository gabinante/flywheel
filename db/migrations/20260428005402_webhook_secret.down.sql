-- Revert webhook_secret addition.
ALTER TABLE projects DROP COLUMN IF EXISTS webhook_secret;

-- Revert notification channel constraint.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_channel_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_channel_check
    CHECK (channel IN ('slack', 'email', 'sms'));

-- Remove webhook_url from notification_preferences.
ALTER TABLE notification_preferences DROP COLUMN IF EXISTS webhook_url;
