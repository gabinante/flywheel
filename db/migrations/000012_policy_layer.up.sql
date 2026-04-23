-- Policy sets: named collections of composable rules per project.
-- Only one policy set is active per project at a time.
CREATE TABLE IF NOT EXISTS policy_sets (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id),
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    posture           TEXT,                          -- posture bundle name if from a preset, NULL if custom
    rules             JSONB NOT NULL DEFAULT '[]',   -- array of policy rules with predicates and actions
    credential_scopes JSONB NOT NULL DEFAULT '{}',   -- credential access levels for the posture
    is_active         BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure at most one active policy set per project.
CREATE UNIQUE INDEX IF NOT EXISTS policy_sets_project_active
    ON policy_sets (project_id) WHERE is_active = true;

-- Fast lookup of policy sets by project.
CREATE INDEX IF NOT EXISTS policy_sets_project_id ON policy_sets (project_id);

-- Policy change audit log: every policy modification is recorded as a change event.
CREATE TABLE IF NOT EXISTS policy_changes (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    policy_set_id  TEXT NOT NULL,
    change_type    TEXT NOT NULL,     -- created, activated, deactivated, updated, deleted
    changed_by     TEXT NOT NULL,
    old_rules      JSONB,
    new_rules      JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup of policy changes by project (for audit/preview).
CREATE INDEX IF NOT EXISTS policy_changes_project_id ON policy_changes (project_id, created_at DESC);
