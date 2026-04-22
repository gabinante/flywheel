-- Cost tracking and budget management tables.
-- All monetary values stored as millicents (1/1000 of a cent USD) for sub-cent precision without floats.

-- LLM call records: every LLM call attributed to project, ticket, worker role, model.
CREATE TABLE IF NOT EXISTS llm_call_records (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    ticket_id       TEXT NOT NULL,
    worker_role     TEXT NOT NULL,
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    model_tier      TEXT NOT NULL,
    operation_type  TEXT NOT NULL DEFAULT 'general',
    input_tokens    BIGINT NOT NULL DEFAULT 0,
    output_tokens   BIGINT NOT NULL DEFAULT 0,
    cost_millicent  BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_llm_calls_project_time ON llm_call_records (project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_llm_calls_ticket ON llm_call_records (ticket_id);
CREATE INDEX IF NOT EXISTS idx_llm_calls_project_month ON llm_call_records (project_id, date_trunc('month', created_at));

-- Budgets: spending limits per project, optionally scoped to ticket or month.
CREATE TABLE IF NOT EXISTS budgets (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    ticket_id       TEXT NOT NULL DEFAULT '',
    month           TEXT NOT NULL DEFAULT '',
    limit_millicent BIGINT NOT NULL,
    warn_at         DOUBLE PRECISION NOT NULL DEFAULT 0.8,
    hard_stop       BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_budgets_scope ON budgets (project_id, ticket_id, month);

-- Budget alerts: pushed to operators when projection exceeds budget.
CREATE TABLE IF NOT EXISTS budget_alerts (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    ticket_id       TEXT NOT NULL DEFAULT '',
    alert_type      TEXT NOT NULL,
    message         TEXT NOT NULL DEFAULT '',
    status_json     JSONB NOT NULL DEFAULT '{}',
    acknowledged    BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_budget_alerts_project_active ON budget_alerts (project_id) WHERE NOT acknowledged;

-- Rate limit events: track when rate limits are hit and backoff/resume.
CREATE TABLE IF NOT EXISTS rate_limit_events (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    ticket_id       TEXT NOT NULL DEFAULT '',
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    retry_after_ms  BIGINT NOT NULL DEFAULT 0,
    reset_at        TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resumed         BOOLEAN NOT NULL DEFAULT false,
    resumed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rate_limits_project_active ON rate_limit_events (project_id) WHERE NOT resumed;

-- Model pricing: provider-agnostic cost-per-token lookup.
CREATE TABLE IF NOT EXISTS model_pricing (
    provider              TEXT NOT NULL,
    model                 TEXT NOT NULL,
    tier                  TEXT NOT NULL,
    input_per_million     BIGINT NOT NULL,
    output_per_million    BIGINT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, model)
);

-- Fallback policies: which operations can use cheaper models.
CREATE TABLE IF NOT EXISTS fallback_policies (
    project_id      TEXT NOT NULL REFERENCES projects(id),
    operation_type  TEXT NOT NULL,
    preferred_tier  TEXT NOT NULL DEFAULT 'mid',
    allow_fallback  BOOLEAN NOT NULL DEFAULT true,
    max_tier        TEXT NOT NULL DEFAULT 'flagship',
    PRIMARY KEY (project_id, operation_type)
);

-- Seed default model pricing for common models.
INSERT INTO model_pricing (provider, model, tier, input_per_million, output_per_million) VALUES
    ('anthropic', 'claude-sonnet-4-20250514', 'mid', 300, 1500),
    ('anthropic', 'claude-opus-4-20250514', 'flagship', 1500, 7500),
    ('anthropic', 'claude-haiku-3-20250307', 'fast', 25, 125),
    ('openai', 'gpt-4o', 'flagship', 250, 1000),
    ('openai', 'gpt-4o-mini', 'fast', 15, 60)
ON CONFLICT DO NOTHING;
