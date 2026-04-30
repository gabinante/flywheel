-- Restore 10-state constraint (data migration is one-way).
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_state_v7_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_state_v02_check
    CHECK (state IN ('draft', 'specced', 'planning', 'awaiting_input', 'executing',
                     'awaiting_validation', 'validated', 'deploying', 'observing', 'closed'));
