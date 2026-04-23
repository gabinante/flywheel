-- Observed State Index (spec v0.2 Layer 10)
-- Continuous, queryable ground-truth view of infrastructure state.
-- Supports Steampipe-based cloud state ingestion and SQL querying,
-- attribution layer, and staleness tracking.

-- Resources observed in the infrastructure.
CREATE TABLE IF NOT EXISTS observed_resources (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    resource_type   TEXT NOT NULL,           -- e.g. 'container', 'service', 'database', 'loadbalancer', 'bucket', 'vm'
    environment     TEXT NOT NULL DEFAULT '',-- e.g. 'prod', 'staging', 'dev'
    name            TEXT NOT NULL,           -- human-readable name
    external_id     TEXT NOT NULL DEFAULT '',-- ID in the external system (e.g. AWS ARN, GCP resource name)
    provider        TEXT NOT NULL DEFAULT '',-- cloud provider or source (e.g. 'aws', 'gcp', 'azure', 'k8s')
    region          TEXT NOT NULL DEFAULT '',
    properties      JSONB NOT NULL DEFAULT '{}',  -- current observed properties
    declared_state  JSONB,                        -- declared/intended state (from IaC, config)
    observed_at     TIMESTAMPTZ NOT NULL,         -- when this state was last observed
    source          TEXT NOT NULL DEFAULT 'steampipe', -- observation source (steampipe, cloudquery, manual)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_observed_resources_project_type ON observed_resources(project_id, resource_type);
CREATE INDEX idx_observed_resources_project_env ON observed_resources(project_id, environment);
CREATE INDEX idx_observed_resources_observed_at ON observed_resources(project_id, observed_at);
CREATE INDEX idx_observed_resources_external_id ON observed_resources(external_id) WHERE external_id != '';
CREATE INDEX idx_observed_resources_staleness ON observed_resources(project_id, resource_type, observed_at);

-- State change log: append-only record of observed changes.
CREATE TABLE IF NOT EXISTS state_changes (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    resource_id     TEXT NOT NULL REFERENCES observed_resources(id),
    change_type     TEXT NOT NULL CHECK (change_type IN ('created', 'updated', 'deleted', 'drift_detected')),
    before_state    JSONB,                   -- state before the change (null for creates)
    after_state     JSONB,                   -- state after the change (null for deletes)
    diff            JSONB,                   -- computed diff of changed properties
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_state_changes_project ON state_changes(project_id, detected_at DESC);
CREATE INDEX idx_state_changes_resource ON state_changes(resource_id, detected_at DESC);

-- Attribution: links state changes to tickets, external actors, or marks as unattributed.
CREATE TABLE IF NOT EXISTS state_attributions (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    change_id       TEXT NOT NULL REFERENCES state_changes(id),
    resource_id     TEXT NOT NULL REFERENCES observed_resources(id),
    ticket_id       TEXT,                    -- ticket that caused the change (nullable)
    actor           TEXT NOT NULL DEFAULT '',-- external actor identifier (e.g. 'deploy-bot', 'john@company.com')
    actor_type      TEXT NOT NULL CHECK (actor_type IN ('ticket', 'external', 'unattributed')),
    confidence      DOUBLE PRECISION NOT NULL DEFAULT 0.0, -- attribution confidence 0.0-1.0
    rationale       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_state_attributions_project ON state_attributions(project_id, created_at DESC);
CREATE INDEX idx_state_attributions_resource ON state_attributions(resource_id);
CREATE INDEX idx_state_attributions_change ON state_attributions(change_id);
CREATE INDEX idx_state_attributions_ticket ON state_attributions(ticket_id) WHERE ticket_id IS NOT NULL;
CREATE INDEX idx_state_attributions_unattributed ON state_attributions(project_id) WHERE actor_type = 'unattributed';

-- Steampipe query cache: stores results of Steampipe SQL queries for comparison.
CREATE TABLE IF NOT EXISTS steampipe_snapshots (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    query           TEXT NOT NULL,
    resource_type   TEXT NOT NULL,
    environment     TEXT NOT NULL DEFAULT '',
    result_hash     TEXT NOT NULL,           -- hash of query result for change detection
    row_count       INTEGER NOT NULL DEFAULT 0,
    executed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_steampipe_snapshots_project ON steampipe_snapshots(project_id, resource_type, executed_at DESC);
