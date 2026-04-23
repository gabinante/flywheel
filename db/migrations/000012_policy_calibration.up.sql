-- Policy calibration feedback loop tables.
-- Policies gate ticket advancement; decisions and outcomes are tracked for calibration.

CREATE TABLE IF NOT EXISTS policies (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    rules       JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    min_sample  INT NOT NULL DEFAULT 20,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS policies_project_enabled ON policies(project_id, enabled);

-- Rolling record of every gated decision with eventual outcome.
CREATE TABLE IF NOT EXISTS policy_decisions (
    id          TEXT PRIMARY KEY,
    policy_id   TEXT NOT NULL REFERENCES policies(id),
    ticket_id   TEXT NOT NULL REFERENCES tickets(id),
    decision    TEXT NOT NULL,  -- auto_approved, required_review, blocked
    outcome     TEXT,           -- success, rollback, incident (NULL = pending)
    reason      TEXT NOT NULL DEFAULT '',
    decided_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    outcome_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS policy_decisions_policy ON policy_decisions(policy_id, decided_at DESC);
CREATE INDEX IF NOT EXISTS policy_decisions_ticket ON policy_decisions(ticket_id);
CREATE INDEX IF NOT EXISTS policy_decisions_outcome_pending ON policy_decisions(policy_id) WHERE outcome IS NULL;

-- Auditable record of every policy edit (change events in the stream).
CREATE TABLE IF NOT EXISTS policy_change_events (
    id          TEXT PRIMARY KEY,
    policy_id   TEXT NOT NULL REFERENCES policies(id),
    actor_id    TEXT NOT NULL,
    change_type TEXT NOT NULL,  -- created, updated, enabled, disabled, broadened, narrowed
    prev_rules  JSONB,
    new_rules   JSONB,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS policy_change_events_policy ON policy_change_events(policy_id, created_at DESC);

-- System-generated proposals for broadening or review flagging.
CREATE TABLE IF NOT EXISTS policy_proposals (
    id             TEXT PRIMARY KEY,
    policy_id      TEXT NOT NULL REFERENCES policies(id),
    proposal_type  TEXT NOT NULL,  -- broaden, review
    suggestion     JSONB NOT NULL DEFAULT '{}',
    statistics     JSONB NOT NULL DEFAULT '{}',
    status         TEXT NOT NULL DEFAULT 'pending',  -- pending, accepted, rejected, dismissed
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at    TIMESTAMPTZ,
    resolved_by    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS policy_proposals_policy_status ON policy_proposals(policy_id, status);
