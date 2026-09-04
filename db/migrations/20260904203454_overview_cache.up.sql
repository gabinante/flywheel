-- Persisted snapshots of slow cross-repo GitHub overviews (My PRs, My Reviews, project PRs)
-- so pages render instantly after a restart while a background refresh runs.
CREATE TABLE IF NOT EXISTS overview_cache (
    key        TEXT PRIMARY KEY,
    data       JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
