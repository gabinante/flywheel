-- Layer 14: Project Map (Catalog)
-- Lightweight built-in service catalog: entities, edges, and deployment matrix.

CREATE TABLE IF NOT EXISTS catalog_entities (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    type        TEXT NOT NULL,  -- service, datastore, integration, infrastructure, repository, environment
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    labels      JSONB NOT NULL DEFAULT '{}',
    metadata    JSONB NOT NULL DEFAULT '{}',
    source      TEXT NOT NULL DEFAULT 'declared',  -- declared or observed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_catalog_entities_project ON catalog_entities(project_id, type);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_name ON catalog_entities(project_id, name);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_labels ON catalog_entities USING gin(labels);

CREATE TABLE IF NOT EXISTS catalog_edges (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    from_id    TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    to_id      TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,  -- depends_on, provides, consumes, deployed_to, backed_by, monitors, owns
    metadata   JSONB NOT NULL DEFAULT '{}',
    source     TEXT NOT NULL DEFAULT 'declared',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, from_id, to_id, type)
);

CREATE INDEX IF NOT EXISTS idx_catalog_edges_from ON catalog_edges(project_id, from_id);
CREATE INDEX IF NOT EXISTS idx_catalog_edges_to ON catalog_edges(project_id, to_id);

CREATE TABLE IF NOT EXISTS catalog_deployments (
    service_id     TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    version        TEXT NOT NULL DEFAULT '',
    source         TEXT NOT NULL DEFAULT 'declared',
    observed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, environment_id)
);

CREATE INDEX IF NOT EXISTS idx_catalog_deployments_project ON catalog_deployments(project_id);
