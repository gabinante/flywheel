-- Add worker_type column to execution_steps for tracing which type of worker
-- produced each step. This supports the worker type differentiation feature
-- (planner, executor, validator, deployer, investigator).
ALTER TABLE execution_steps ADD COLUMN IF NOT EXISTS worker_type TEXT;

-- Index for querying steps by worker type (useful for calibration and debugging).
CREATE INDEX IF NOT EXISTS idx_execution_steps_worker_type ON execution_steps(worker_type) WHERE worker_type IS NOT NULL;
