CREATE TABLE IF NOT EXISTS orchestrator_runs (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_message_id      TEXT NOT NULL REFERENCES orchestrator_messages(id) ON DELETE CASCADE,
    assistant_message_id TEXT REFERENCES orchestrator_messages(id) ON DELETE SET NULL,
    status               TEXT NOT NULL,
    worker_id            TEXT,
    worker_name          TEXT,
    runner               TEXT,
    driver               TEXT,
    model                TEXT,
    error                TEXT,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_orchestrator_runs_project
    ON orchestrator_runs(project_id, started_at);

CREATE TABLE IF NOT EXISTS orchestrator_run_events (
    id         TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES orchestrator_runs(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestrator_run_events_run
    ON orchestrator_run_events(run_id, created_at);
