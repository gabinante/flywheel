-- Reports posted to Linear: project status updates (delta since the last one) and the weekly roundup.
CREATE TABLE IF NOT EXISTS project_reports (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('project_update', 'weekly_roundup')),
    project_id   TEXT NOT NULL DEFAULT '',          -- empty for cross-project weekly roundups
    window_start TIMESTAMPTZ NOT NULL,
    window_end   TIMESTAMPTZ NOT NULL,
    body         TEXT NOT NULL,
    health       TEXT NOT NULL DEFAULT '',          -- onTrack | atRisk | offTrack
    posted       BOOLEAN NOT NULL DEFAULT FALSE,
    url          TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_project_reports_project ON project_reports (project_id, kind, created_at DESC);
