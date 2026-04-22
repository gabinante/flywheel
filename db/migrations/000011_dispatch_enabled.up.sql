-- Per-project dispatch control: dispatch_enabled boolean (default true).
-- When false, the dispatcher skips tickets for this project.
ALTER TABLE projects ADD COLUMN dispatch_enabled BOOLEAN NOT NULL DEFAULT true;
