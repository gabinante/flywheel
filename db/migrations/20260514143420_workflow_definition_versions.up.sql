-- Immutable version history for workflow definitions.
-- Each create/update snapshots the phases so in-flight tickets
-- can execute against the version they were assigned.
CREATE TABLE IF NOT EXISTS workflow_definition_versions (
    workflow_id TEXT NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    phases      JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workflow_id, version)
);

-- Seed from existing definitions so current in-flight tickets work.
INSERT INTO workflow_definition_versions (workflow_id, version, name, description, phases, created_at)
SELECT id, version, name, description, phases, updated_at
FROM workflow_definitions
ON CONFLICT DO NOTHING;

-- Pin workflow version on tickets.
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS workflow_version INTEGER;
