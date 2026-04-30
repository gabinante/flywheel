-- Consolidate ticket states from 10 to 7 canonical states.
-- Removed states: specced, deploying, observing

-- Migrate any tickets in removed states to their closest canonical equivalent.
UPDATE tickets SET state = 'draft' WHERE state = 'specced';
UPDATE tickets SET state = 'validated' WHERE state = 'deploying';
UPDATE tickets SET state = 'closed' WHERE state = 'observing';

-- Replace the v0.2 10-state constraint with the canonical 7-state constraint.
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_state_v02_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_state_v7_check
    CHECK (state IN ('draft', 'planning', 'awaiting_input', 'executing',
                     'awaiting_validation', 'validated', 'closed'));
