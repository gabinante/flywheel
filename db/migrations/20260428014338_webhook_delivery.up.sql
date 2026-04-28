-- Webhook delivery system: outbound webhook events and delivery tracking.
-- Adds webhook_url and webhook_secret to projects, plus webhook_events and
-- webhook_deliveries tables for the delivery pipeline.

-- Add webhook configuration columns to projects.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS webhook_url TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS webhook_secret TEXT NOT NULL DEFAULT '';

-- Webhook events: records created by the post-payment pipeline (or any internal trigger).
-- Each event represents a payload that should be delivered to the project's webhook_url.
CREATE TABLE IF NOT EXISTS webhook_events (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    event_type      TEXT NOT NULL,                              -- e.g. 'payment.completed', 'chargeback.created'
    payload         JSONB NOT NULL DEFAULT '{}',                -- the full event-specific payload
    status          TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'failed')) DEFAULT 'pending',
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 4,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_project ON webhook_events (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_events_status ON webhook_events (status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_webhook_events_project_status ON webhook_events (project_id, status);

-- Webhook deliveries: one row per delivery attempt for each webhook event.
-- Records status code, response body (truncated), duration, and any error.
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              TEXT PRIMARY KEY,
    webhook_event_id TEXT NOT NULL REFERENCES webhook_events(id),
    project_id      TEXT NOT NULL REFERENCES projects(id),
    attempt_number  INTEGER NOT NULL,
    status_code     INTEGER,                                     -- HTTP status code (NULL for network errors)
    response_body   TEXT NOT NULL DEFAULT '',                     -- truncated to 1KB
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    success         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event ON webhook_deliveries (webhook_event_id, attempt_number);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_project ON webhook_deliveries (project_id, created_at DESC);
