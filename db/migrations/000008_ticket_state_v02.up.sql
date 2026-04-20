-- Migration: Expand ticket state machine to spec v0.2 lifecycle.
-- Maps old states to new v0.2 states and adds environment column.
--
-- State mapping:
--   pending        → draft
--   claimed        → planning
--   executing      → executing (unchanged)
--   awaiting_review → awaiting_validation
--   done           → closed
--   needs_human    → awaiting_input
--   blocked        → awaiting_input
--   failed         → draft
--
-- New v0.2 states: draft, specced, planning, awaiting_input, executing,
--                  awaiting_validation, validated, deploying, observing, closed

-- Add environment column for environment-scoped state machine (spec 4.1).
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS environment TEXT DEFAULT 'development';

-- Migrate existing state values to v0.2 equivalents.
UPDATE tickets SET state = 'draft' WHERE state = 'pending';
UPDATE tickets SET state = 'planning' WHERE state = 'claimed';
UPDATE tickets SET state = 'awaiting_validation' WHERE state = 'awaiting_review';
UPDATE tickets SET state = 'closed' WHERE state = 'done';
UPDATE tickets SET state = 'awaiting_input' WHERE state IN ('needs_human', 'blocked');
UPDATE tickets SET state = 'draft' WHERE state = 'failed';

-- Add a CHECK constraint to enforce only valid v0.2 states.
ALTER TABLE tickets ADD CONSTRAINT tickets_state_v02_check
    CHECK (state IN ('draft', 'specced', 'planning', 'awaiting_input', 'executing',
                     'awaiting_validation', 'validated', 'deploying', 'observing', 'closed'));

-- Update index for common query patterns with new states.
DROP INDEX IF EXISTS tickets_project_state;
CREATE INDEX tickets_project_state ON tickets(project_id, state);

-- Index for environment-scoped queries.
CREATE INDEX tickets_project_env_state ON tickets(project_id, environment, state);
