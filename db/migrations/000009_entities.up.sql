-- Entity identity system (spec v0.2 Layer 0, section 2.1)
-- Every entity has: stable UUID, type, logical name, optional environment qualification.

-- Entity type enum
CREATE TYPE entity_type AS ENUM (
    'service',
    'datastore',
    'integration',
    'ticket',
    'finding'
);

-- Entities table: stable identity for all cross-layer references
CREATE TABLE entities (
    id              TEXT PRIMARY KEY,           -- stable UUID, minted once, never reused
    type            entity_type NOT NULL,
    logical_name    TEXT NOT NULL,              -- human-readable, mutable
    project_id      TEXT NOT NULL REFERENCES projects(id),
    description     TEXT NOT NULL DEFAULT '',
    attributes      JSONB NOT NULL DEFAULT '{}', -- external system IDs and other metadata
    retired_at      TIMESTAMPTZ,               -- soft-delete: non-null = retired
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX entities_project_type ON entities(project_id, type);
CREATE INDEX entities_project_name ON entities(project_id, logical_name);
CREATE INDEX entities_retired ON entities(project_id) WHERE retired_at IS NULL;

-- Environment-qualified entity instances
-- e.g. OrderService@prod vs OrderService@dev
CREATE TABLE entity_instances (
    id              TEXT PRIMARY KEY,           -- stable UUID for the instance
    entity_id       TEXT NOT NULL REFERENCES entities(id),
    environment     TEXT NOT NULL,              -- e.g. 'prod', 'staging', 'dev'
    attributes      JSONB NOT NULL DEFAULT '{}', -- instance-specific metadata (endpoints, versions, etc.)
    retired_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, environment)
);

CREATE INDEX entity_instances_entity ON entity_instances(entity_id);

-- Entity stream: append-only log of entity lifecycle events
CREATE TABLE entity_stream (
    id              TEXT PRIMARY KEY,
    entity_id       TEXT NOT NULL REFERENCES entities(id),
    event_type      TEXT NOT NULL,              -- 'created', 'renamed', 'retired', 'instance_created', 'instance_retired', 'attribute_changed'
    payload         JSONB NOT NULL DEFAULT '{}',
    actor           TEXT NOT NULL,              -- who/what caused the event
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX entity_stream_entity ON entity_stream(entity_id, created_at);
CREATE INDEX entity_stream_time ON entity_stream(created_at);
