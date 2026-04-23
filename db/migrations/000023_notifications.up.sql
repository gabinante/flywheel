-- Notification layer: policy-driven async push to operators.
-- Supports four categories: urgent_decision, autonomous_action, calibration_review, anomaly_alert.
-- Channels: slack (default), email, sms. Track dismissal rates for classifier tuning.

-- Notifications: every notification sent or queued for digest.
CREATE TABLE IF NOT EXISTS notifications (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    ticket_id       TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL CHECK (category IN ('urgent_decision', 'autonomous_action', 'calibration_review', 'anomaly_alert')),
    urgency         TEXT NOT NULL CHECK (urgency IN ('critical', 'high', 'medium', 'low')),
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    channel         TEXT NOT NULL CHECK (channel IN ('slack', 'email', 'sms')),
    routing         TEXT NOT NULL CHECK (routing IN ('push', 'digest')) DEFAULT 'push',
    -- Decision metadata (for urgent_decision category).
    decision_json   JSONB NOT NULL DEFAULT '{}',
    -- Arbitrary context payload.
    context_json    JSONB NOT NULL DEFAULT '{}',
    -- Delivery tracking.
    status          TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'digested')) DEFAULT 'pending',
    sent_at         TIMESTAMPTZ,
    error_message   TEXT NOT NULL DEFAULT '',
    -- Dismissal tracking for classifier tuning.
    dismissed       BOOLEAN NOT NULL DEFAULT false,
    dismissed_at    TIMESTAMPTZ,
    -- Classifier that originated this notification (for tuning feedback).
    classifier      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_project ON notifications (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_project_pending ON notifications (project_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_notifications_classifier ON notifications (classifier) WHERE classifier != '';
CREATE INDEX IF NOT EXISTS idx_notifications_digest ON notifications (project_id, routing, status) WHERE routing = 'digest' AND status = 'pending';

-- Notification preferences: per-project channel routing and digest settings.
CREATE TABLE IF NOT EXISTS notification_preferences (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    -- Channel preferences per urgency level.
    critical_channel TEXT NOT NULL DEFAULT 'slack',
    high_channel     TEXT NOT NULL DEFAULT 'slack',
    medium_channel   TEXT NOT NULL DEFAULT 'slack',
    low_channel      TEXT NOT NULL DEFAULT 'slack',
    -- Digest settings.
    digest_enabled   BOOLEAN NOT NULL DEFAULT true,
    digest_interval  TEXT NOT NULL DEFAULT '1h',
    -- Urgency threshold: notifications at or above this urgency are pushed immediately.
    push_threshold   TEXT NOT NULL CHECK (push_threshold IN ('critical', 'high', 'medium', 'low')) DEFAULT 'high',
    -- Channel configuration (webhook URLs, etc.).
    slack_webhook_url TEXT NOT NULL DEFAULT '',
    email_address     TEXT NOT NULL DEFAULT '',
    sms_number        TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id)
);

-- Dismissal rate tracking: aggregated per classifier for tuning feedback.
CREATE TABLE IF NOT EXISTS notification_dismissal_rates (
    classifier      TEXT NOT NULL,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    total_sent      BIGINT NOT NULL DEFAULT 0,
    total_dismissed BIGINT NOT NULL DEFAULT 0,
    -- Rolling window stats (last 30 days).
    window_sent     BIGINT NOT NULL DEFAULT 0,
    window_dismissed BIGINT NOT NULL DEFAULT 0,
    last_updated    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (classifier, project_id)
);
