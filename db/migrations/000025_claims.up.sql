-- Claims registry: environment-scoped concurrency control (spec v0.2 §4.3).
-- When a ticket enters execution, its plan's declared touches are registered as claims.
-- Claims release on ticket completion/abandonment. Conflict classification runs at
-- ticket creation and execution dispatch.

CREATE TABLE IF NOT EXISTS claims (
    id           TEXT PRIMARY KEY,
    ticket_id    TEXT NOT NULL REFERENCES tickets(id),
    entity_id    TEXT NOT NULL,
    environment  TEXT NOT NULL DEFAULT '',
    claim_type   TEXT NOT NULL,
    state        TEXT NOT NULL DEFAULT 'active',
    metadata     JSONB NOT NULL DEFAULT '{}',
    claimed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at  TIMESTAMPTZ,

    CONSTRAINT claims_type_check CHECK (claim_type IN (
        'file_write', 'symbol', 'service', 'schema', 'resource', 'deploy_target'
    )),
    CONSTRAINT claims_state_check CHECK (state IN ('active', 'released'))
);

-- Primary query pattern: find active claims for a given entity+environment
CREATE INDEX IF NOT EXISTS claims_entity_env_active ON claims(entity_id, environment) WHERE state = 'active';

-- Find all active claims for a ticket (release on completion)
CREATE INDEX IF NOT EXISTS claims_ticket_active ON claims(ticket_id) WHERE state = 'active';

-- Find all active claims in a project's environment (conflict detection)
CREATE INDEX IF NOT EXISTS claims_env_active ON claims(environment, state);

-- Conflict detection table: records detected conflicts between tickets
CREATE TABLE IF NOT EXISTS claim_conflicts (
    id              TEXT PRIMARY KEY,
    ticket_id       TEXT NOT NULL REFERENCES tickets(id),
    blocking_ticket TEXT NOT NULL REFERENCES tickets(id),
    claim_id        TEXT NOT NULL REFERENCES claims(id),
    conflict_type   TEXT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'hard',
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ,

    CONSTRAINT conflicts_type_check CHECK (conflict_type IN (
        'same_file_write', 'same_symbol', 'same_service', 'same_schema', 'disjoint'
    )),
    CONSTRAINT conflicts_severity_check CHECK (severity IN ('hard', 'soft'))
);

CREATE INDEX IF NOT EXISTS conflicts_ticket ON claim_conflicts(ticket_id) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS conflicts_blocking ON claim_conflicts(blocking_ticket) WHERE resolved_at IS NULL;
