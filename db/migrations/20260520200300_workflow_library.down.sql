DROP INDEX IF EXISTS idx_workflow_definitions_library;

ALTER TABLE workflow_definitions
  DROP COLUMN IF EXISTS is_library;
