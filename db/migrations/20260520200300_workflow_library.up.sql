ALTER TABLE workflow_definitions
  ADD COLUMN IF NOT EXISTS is_library BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_workflow_definitions_library
  ON workflow_definitions (scope, scope_id) WHERE is_library = true;
