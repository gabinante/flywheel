-- plans: first-class execution plan entity with typed per-backend sub-schemas.
-- Each plan is linked to a ticket, carries a backend type, versioned content,
-- and freshness stamps for re-plan-before-apply semantics (warrant-45).

CREATE TABLE plans (
    id              TEXT PRIMARY KEY,
    ticket_id       TEXT NOT NULL REFERENCES tickets(id),
    backend         TEXT NOT NULL,
    state           TEXT NOT NULL DEFAULT 'draft',
    version         INT NOT NULL DEFAULT 1,
    content         JSONB NOT NULL,
    freshness_stamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- backend must be one of the known typed backends
    CONSTRAINT plans_backend_check CHECK (backend IN ('database', 'terraform', 'code', 'shell', 'deploy')),
    -- state lifecycle
    CONSTRAINT plans_state_check CHECK (state IN ('draft', 'submitted', 'classified', 'approved', 'applied', 'superseded', 'rejected'))
);

CREATE INDEX plans_ticket_id ON plans(ticket_id);
CREATE INDEX plans_ticket_backend ON plans(ticket_id, backend);
CREATE INDEX plans_state ON plans(state);

-- plan_versions: immutable version history for plan content changes.
CREATE TABLE plan_versions (
    id         TEXT PRIMARY KEY,
    plan_id    TEXT NOT NULL REFERENCES plans(id),
    version    INT NOT NULL,
    content    JSONB NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (plan_id, version)
);

CREATE INDEX plan_versions_plan_id ON plan_versions(plan_id);
