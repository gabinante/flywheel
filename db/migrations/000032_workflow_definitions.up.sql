-- Workflow definitions: configurable SDLC pipelines per system/org/project.
CREATE TABLE IF NOT EXISTS workflow_definitions (
    id          TEXT PRIMARY KEY,
    scope       TEXT NOT NULL CHECK (scope IN ('system', 'org', 'project')),
    scope_id    TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version     INTEGER NOT NULL DEFAULT 1,
    phases      JSONB NOT NULL DEFAULT '[]',
    is_active   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workflow_definitions_scope ON workflow_definitions (scope, scope_id, is_active);

-- Track which workflow and phase a ticket is currently on.
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS workflow_id TEXT REFERENCES workflow_definitions(id);
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS workflow_phase TEXT;

-- Phase completion audit trail.
CREATE TABLE IF NOT EXISTS workflow_phase_completions (
    id           TEXT PRIMARY KEY,
    ticket_id    TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    workflow_id  TEXT NOT NULL,
    phase_id     TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    outcome      TEXT NOT NULL CHECK (outcome IN ('success', 'failed', 'skipped')),
    metadata     JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_wpc_ticket ON workflow_phase_completions (ticket_id);
