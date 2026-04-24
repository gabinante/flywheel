ALTER TABLE projects
ADD COLUMN IF NOT EXISTS dispatch_config JSONB NOT NULL DEFAULT '{}'::jsonb;
