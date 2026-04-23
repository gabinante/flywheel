-- Migration: Add compound environment model (spec v0.2 section 4.1).
-- Environments are compound tuples: infrastructure × data_tenancy × integration_mode.
-- Ticket states become environment-qualified (e.g. executing-dev, validated-staging).

-- Create environments table.
CREATE TABLE IF NOT EXISTS environments (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id),
    name             TEXT NOT NULL,
    slug             TEXT NOT NULL,
    infrastructure   TEXT NOT NULL CHECK (infrastructure IN ('dev', 'staging', 'prod')),
    data_tenancy     TEXT NOT NULL CHECK (data_tenancy IN ('synthetic', 'anonymized', 'real')),
    integration_mode TEXT NOT NULL CHECK (integration_mode IN ('sandbox', 'test', 'live')),
    is_default       BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);

CREATE INDEX environments_project_id ON environments(project_id);

-- Add environment_id column to tickets (nullable for backward compat during migration).
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS environment_id TEXT REFERENCES environments(id);

-- Index for environment-scoped ticket queries.
CREATE INDEX IF NOT EXISTS tickets_environment_id ON tickets(environment_id);

-- Provision default environments for existing projects that don't have any.
-- Each project gets "dev" (dev/synthetic/sandbox) and "staging" (staging/anonymized/test).
INSERT INTO environments (id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at)
SELECT
    gen_random_uuid()::text,
    p.id,
    'Development',
    'dev',
    'dev',
    'synthetic',
    'sandbox',
    true,
    now(),
    now()
FROM projects p
WHERE NOT EXISTS (SELECT 1 FROM environments e WHERE e.project_id = p.id);

INSERT INTO environments (id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at)
SELECT
    gen_random_uuid()::text,
    p.id,
    'Staging',
    'staging',
    'staging',
    'anonymized',
    'test',
    false,
    now(),
    now()
FROM projects p
WHERE NOT EXISTS (SELECT 1 FROM environments e WHERE e.project_id = p.id AND e.slug = 'staging');

-- Migrate existing tickets: assign them to the default environment for their project.
UPDATE tickets t
SET environment_id = (
    SELECT e.id FROM environments e
    WHERE e.project_id = t.project_id AND e.is_default = true
    LIMIT 1
)
WHERE t.environment_id IS NULL;
