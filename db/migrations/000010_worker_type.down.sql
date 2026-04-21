DROP INDEX IF EXISTS idx_execution_steps_worker_type;
ALTER TABLE execution_steps DROP COLUMN IF EXISTS worker_type;
