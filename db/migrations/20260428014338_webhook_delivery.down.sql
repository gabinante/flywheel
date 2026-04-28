DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_events;
ALTER TABLE projects DROP COLUMN IF EXISTS webhook_url;
ALTER TABLE projects DROP COLUMN IF EXISTS webhook_secret;
