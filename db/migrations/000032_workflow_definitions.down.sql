DROP INDEX IF EXISTS idx_wpc_ticket;
DROP TABLE IF EXISTS workflow_phase_completions;
ALTER TABLE tickets DROP COLUMN IF EXISTS workflow_phase;
ALTER TABLE tickets DROP COLUMN IF EXISTS workflow_id;
DROP INDEX IF EXISTS idx_workflow_definitions_scope;
DROP TABLE IF EXISTS workflow_definitions;
