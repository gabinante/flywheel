-- Rollback: Remove compound environment model.

-- Remove environment_id from tickets.
ALTER TABLE tickets DROP COLUMN IF EXISTS environment_id;

-- Drop environments table.
DROP TABLE IF EXISTS environments;
