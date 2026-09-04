-- Linear as the ticket store: Flywheel projects map 1:1 to Linear projects, and
-- tickets carry a projection of the Linear issue they mirror.
CREATE TABLE IF NOT EXISTS project_linear_links (
    project_id          TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    linear_project_id   TEXT NOT NULL UNIQUE,
    linear_project_name TEXT NOT NULL DEFAULT '',
    linear_project_url  TEXT NOT NULL DEFAULT '',
    team_ids            TEXT[] NOT NULL DEFAULT '{}',
    team_keys           TEXT[] NOT NULL DEFAULT '{}',
    synced_at           TIMESTAMPTZ,
    last_error          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ticket_external_refs (
    ticket_id     TEXT PRIMARY KEY REFERENCES tickets(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL DEFAULT 'linear',
    external_id   TEXT NOT NULL,                 -- Linear issue UUID
    identifier    TEXT NOT NULL,                 -- RLETD-465
    url           TEXT NOT NULL DEFAULT '',
    state_name    TEXT NOT NULL DEFAULT '',      -- e.g. In Review
    state_type    TEXT NOT NULL DEFAULT '',      -- triage|backlog|unstarted|started|completed|canceled
    assignee      TEXT NOT NULL DEFAULT '',
    team_key      TEXT NOT NULL DEFAULT '',
    priority      INTEGER NOT NULL DEFAULT 0,    -- Linear priority 0-4
    labels        TEXT[] NOT NULL DEFAULT '{}',
    branch_name   TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ,                   -- Linear updatedAt
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, external_id)
);

CREATE INDEX IF NOT EXISTS idx_ticket_external_refs_identifier ON ticket_external_refs (identifier);
