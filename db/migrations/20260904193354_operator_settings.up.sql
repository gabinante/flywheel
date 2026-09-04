-- Operator settings edited in the UI. Environment variables seed the defaults; once
-- saved here, this row wins. Single row: Flywheel is single-operator.
CREATE TABLE IF NOT EXISTS operator_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    data       JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
