-- pillar_entries: per-entity pillar strategy records (Layer 15).
-- Seven fixed pillars: observability, mutability, scalability, availability, security, resiliency, cost.
CREATE TABLE IF NOT EXISTS pillar_entries (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    entity_id       TEXT NOT NULL,  -- references catalog entity by ID (cross-layer)
    entity_type     TEXT NOT NULL,  -- informational: service, datastore, etc.
    pillar_type     TEXT NOT NULL,
    strategy        TEXT NOT NULL DEFAULT '',
    gaps            JSONB NOT NULL DEFAULT '[]',
    review_cadence  TEXT NOT NULL DEFAULT 'monthly',
    last_reviewed_at TIMESTAMPTZ,
    next_review_at  TIMESTAMPTZ,
    version         INT NOT NULL DEFAULT 1,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pillar_entries_type_check CHECK (pillar_type IN (
        'observability', 'mutability', 'scalability', 'availability',
        'security', 'resiliency', 'cost'
    )),
    CONSTRAINT pillar_entries_cadence_check CHECK (review_cadence IN (
        'weekly', 'biweekly', 'monthly', 'quarterly'
    )),
    UNIQUE (project_id, entity_id, pillar_type)
);

CREATE INDEX IF NOT EXISTS idx_pillar_entries_project ON pillar_entries(project_id);
CREATE INDEX IF NOT EXISTS idx_pillar_entries_entity ON pillar_entries(entity_id);
CREATE INDEX IF NOT EXISTS idx_pillar_entries_type ON pillar_entries(pillar_type);
CREATE INDEX IF NOT EXISTS idx_pillar_entries_review ON pillar_entries(next_review_at) WHERE next_review_at IS NOT NULL;

-- pillar_claims: structured assertions citing map entities as evidence.
CREATE TABLE IF NOT EXISTS pillar_claims (
    id               TEXT PRIMARY KEY,
    pillar_entry_id  TEXT NOT NULL REFERENCES pillar_entries(id),
    statement        TEXT NOT NULL,
    entity_ref_id    TEXT NOT NULL,  -- catalog entity ID cited as evidence
    entity_ref_type  TEXT NOT NULL,  -- type of the referenced entity
    evidence         TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'unverified',
    last_evaluated_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pillar_claims_status_check CHECK (status IN (
        'verified', 'unverified', 'stale', 'invalid'
    ))
);

CREATE INDEX IF NOT EXISTS idx_pillar_claims_entry ON pillar_claims(pillar_entry_id);
CREATE INDEX IF NOT EXISTS idx_pillar_claims_entity_ref ON pillar_claims(entity_ref_id);

-- pillar_evaluations: continuous evaluation loop results.
CREATE TABLE IF NOT EXISTS pillar_evaluations (
    id               TEXT PRIMARY KEY,
    pillar_entry_id  TEXT NOT NULL REFERENCES pillar_entries(id),
    check_type       TEXT NOT NULL,
    outcome          TEXT NOT NULL,
    details          TEXT NOT NULL DEFAULT '',
    evaluated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pillar_eval_check_type CHECK (check_type IN (
        'evidence_intact', 'claim_matches_reality', 'strategy_appropriate'
    )),
    CONSTRAINT pillar_eval_outcome CHECK (outcome IN ('pass', 'warn', 'fail'))
);

CREATE INDEX IF NOT EXISTS idx_pillar_evaluations_entry ON pillar_evaluations(pillar_entry_id, evaluated_at DESC);
