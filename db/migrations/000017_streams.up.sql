-- Three foundational append-only streams per spec v0.2 section 2.2.
-- All share the common change event shape from section 2.3:
--   change_type, affected_entities, before/after, initiator, environment, timestamp.

-- =============================================================================
-- Entity Stream: what exists — creation, renaming, retirement of entities.
-- =============================================================================
CREATE TABLE IF NOT EXISTS entity_stream (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL REFERENCES projects(id),
    -- Common shape (spec 2.3)
    change_type     TEXT        NOT NULL CHECK (change_type IN (
        'created',
        'renamed',
        'retired',
        'updated',
        'instance_created',
        'instance_retired',
        'attribute_changed'
    )),
    affected_entities TEXT[]    NOT NULL DEFAULT '{}',
    before_state    JSONB,
    after_state     JSONB,
    initiator_id    TEXT        NOT NULL DEFAULT '',
    initiator_type  TEXT        NOT NULL DEFAULT 'system' CHECK (initiator_type IN ('human', 'agent', 'system')),
    environment     TEXT        NOT NULL DEFAULT '',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Entity-specific fields
    entity_id       TEXT        NOT NULL,
    entity_type     TEXT        NOT NULL CHECK (entity_type IN ('service', 'datastore', 'integration', 'ticket', 'finding', 'infrastructure', 'other')),
    entity_name     TEXT        NOT NULL DEFAULT ''
);

-- Rich indexing: by time, entity, type, environment, attribution
CREATE INDEX idx_entity_stream_project_time ON entity_stream (project_id, timestamp DESC);
CREATE INDEX idx_entity_stream_entity ON entity_stream (entity_id, timestamp DESC);
CREATE INDEX idx_entity_stream_type ON entity_stream (project_id, change_type);
CREATE INDEX idx_entity_stream_env ON entity_stream (project_id, environment) WHERE environment != '';
CREATE INDEX idx_entity_stream_initiator ON entity_stream (initiator_id) WHERE initiator_id != '';
CREATE INDEX idx_entity_stream_entity_type ON entity_stream (project_id, entity_type);

-- =============================================================================
-- State Stream: current values — observations from APIs, K8s, databases.
-- =============================================================================
CREATE TABLE IF NOT EXISTS state_stream (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL REFERENCES projects(id),
    -- Common shape (spec 2.3)
    change_type     TEXT        NOT NULL CHECK (change_type IN ('observed', 'updated', 'drifted', 'reconciled')),
    affected_entities TEXT[]    NOT NULL DEFAULT '{}',
    before_state    JSONB,
    after_state     JSONB,
    initiator_id    TEXT        NOT NULL DEFAULT '',
    initiator_type  TEXT        NOT NULL DEFAULT 'system' CHECK (initiator_type IN ('human', 'agent', 'system')),
    environment     TEXT        NOT NULL DEFAULT '',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- State-specific fields
    entity_id       TEXT        NOT NULL,
    source          TEXT        NOT NULL DEFAULT '',
    observed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_state_stream_project_time ON state_stream (project_id, timestamp DESC);
CREATE INDEX idx_state_stream_entity ON state_stream (entity_id, timestamp DESC);
CREATE INDEX idx_state_stream_type ON state_stream (project_id, change_type);
CREATE INDEX idx_state_stream_env ON state_stream (project_id, environment) WHERE environment != '';
CREATE INDEX idx_state_stream_initiator ON state_stream (initiator_id) WHERE initiator_id != '';
CREATE INDEX idx_state_stream_source ON state_stream (project_id, source) WHERE source != '';

-- =============================================================================
-- Change Stream: what happened — deploys, config updates, failovers, ticket events.
-- =============================================================================
CREATE TABLE IF NOT EXISTS change_stream (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL REFERENCES projects(id),
    -- Common shape (spec 2.3)
    change_type     TEXT        NOT NULL,
    affected_entities TEXT[]    NOT NULL DEFAULT '{}',
    before_state    JSONB,
    after_state     JSONB,
    initiator_id    TEXT        NOT NULL DEFAULT '',
    initiator_type  TEXT        NOT NULL DEFAULT 'system' CHECK (initiator_type IN ('human', 'agent', 'system')),
    environment     TEXT        NOT NULL DEFAULT '',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Change-specific: no strict enum on change_type to allow extensibility
    -- (deploy, config_update, failover, ticket_state_changed, etc.)
    source          TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX idx_change_stream_project_time ON change_stream (project_id, timestamp DESC);
CREATE INDEX idx_change_stream_type ON change_stream (project_id, change_type);
CREATE INDEX idx_change_stream_env ON change_stream (project_id, environment) WHERE environment != '';
CREATE INDEX idx_change_stream_initiator ON change_stream (initiator_id) WHERE initiator_id != '';
CREATE INDEX idx_change_stream_source ON change_stream (project_id, source) WHERE source != '';
-- GIN index on affected_entities array for containment queries
CREATE INDEX idx_change_stream_entities ON change_stream USING GIN (affected_entities);

-- Append-only enforcement: deny UPDATE and DELETE on all three stream tables
-- via a trigger that raises an exception.
CREATE OR REPLACE FUNCTION deny_stream_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'stream tables are append-only; UPDATE and DELETE are not allowed';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER entity_stream_no_update
    BEFORE UPDATE ON entity_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();
CREATE TRIGGER entity_stream_no_delete
    BEFORE DELETE ON entity_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();

CREATE TRIGGER state_stream_no_update
    BEFORE UPDATE ON state_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();
CREATE TRIGGER state_stream_no_delete
    BEFORE DELETE ON state_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();

CREATE TRIGGER change_stream_no_update
    BEFORE UPDATE ON change_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();
CREATE TRIGGER change_stream_no_delete
    BEFORE DELETE ON change_stream FOR EACH ROW EXECUTE FUNCTION deny_stream_mutation();
