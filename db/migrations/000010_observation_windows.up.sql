-- Migration: Add observation windows, signals, and attributions tables.
-- Supports production observation with attribution under concurrent changes.

-- observation_windows: tracks the monitoring period after a ticket's change is deployed.
CREATE TABLE IF NOT EXISTS observation_windows (
    id         TEXT PRIMARY KEY,
    ticket_id  TEXT NOT NULL REFERENCES tickets(id),
    project_id TEXT NOT NULL REFERENCES projects(id),
    scope      JSONB NOT NULL DEFAULT '{}',
    started_at TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ NOT NULL,
    closed_at  TIMESTAMPTZ,
    state      TEXT NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'closed', 'extended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_obs_windows_project_state ON observation_windows(project_id, state);
CREATE INDEX idx_obs_windows_ticket ON observation_windows(ticket_id);
CREATE INDEX idx_obs_windows_time_range ON observation_windows(project_id, started_at, ends_at);

-- signals: production observations (regressions, anomalies, incidents).
CREATE TABLE IF NOT EXISTS signals (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    source      TEXT NOT NULL,
    signal_type TEXT NOT NULL CHECK (signal_type IN ('regression', 'anomaly', 'incident', 'alert')),
    severity    TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    title       TEXT NOT NULL,
    detail      TEXT,
    scope       JSONB NOT NULL DEFAULT '{}',
    metadata    JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_signals_project_time ON signals(project_id, occurred_at);
CREATE INDEX idx_signals_source ON signals(source);

-- attributions: links signals to candidate tickets with confidence scores.
CREATE TABLE IF NOT EXISTS attributions (
    id          TEXT PRIMARY KEY,
    signal_id   TEXT NOT NULL REFERENCES signals(id),
    project_id  TEXT NOT NULL REFERENCES projects(id),
    candidates  JSONB NOT NULL DEFAULT '[]',
    confidence  DOUBLE PRECISION NOT NULL DEFAULT 0,
    rationale   TEXT NOT NULL DEFAULT '',
    retroactive BOOLEAN NOT NULL DEFAULT false,
    resolved    BOOLEAN NOT NULL DEFAULT false,
    resolved_by TEXT,
    resolution  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attributions_project_resolved ON attributions(project_id, resolved);
CREATE INDEX idx_attributions_signal ON attributions(signal_id);
CREATE INDEX idx_attributions_created ON attributions(project_id, created_at);

-- signal_rules: configurable signal-to-ticket routing rules.
CREATE TABLE IF NOT EXISTS signal_rules (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id),
    source       TEXT NOT NULL DEFAULT '',
    signal_type  TEXT NOT NULL DEFAULT '',
    min_severity TEXT NOT NULL DEFAULT 'low',
    action       TEXT NOT NULL CHECK (action IN ('create_ticket', 'annotate_ticket', 'surface_human', 'ignore')),
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_signal_rules_project ON signal_rules(project_id, enabled);
