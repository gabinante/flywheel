-- Rollback: Revert ticket state machine from v0.2 back to legacy states.
-- Reverse state mapping:
--   draft              → pending (or failed, ambiguous — defaulting to pending)
--   specced            → pending
--   planning           → claimed
--   awaiting_input     → needs_human
--   executing          → executing (unchanged)
--   awaiting_validation → awaiting_review
--   validated          → awaiting_review (closest equivalent)
--   deploying          → executing (closest equivalent)
--   observing          → executing (closest equivalent)
--   closed             → done

-- Drop the v0.2 check constraint.
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_state_v02_check;

-- Revert states to legacy values.
UPDATE tickets SET state = 'pending' WHERE state IN ('draft', 'specced');
UPDATE tickets SET state = 'claimed' WHERE state = 'planning';
UPDATE tickets SET state = 'needs_human' WHERE state = 'awaiting_input';
UPDATE tickets SET state = 'awaiting_review' WHERE state IN ('awaiting_validation', 'validated');
UPDATE tickets SET state = 'executing' WHERE state IN ('deploying', 'observing');
UPDATE tickets SET state = 'done' WHERE state = 'closed';

-- Drop environment column.
DROP INDEX IF EXISTS tickets_project_env_state;
ALTER TABLE tickets DROP COLUMN IF EXISTS environment;

-- Recreate original index.
DROP INDEX IF EXISTS tickets_project_state;
CREATE INDEX tickets_project_state ON tickets(project_id, state);
