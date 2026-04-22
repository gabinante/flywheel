-- Multi-repo support: project_repositories table and ticket.target_repo column.
-- A project can span multiple repositories. Each repo has a unique alias within
-- the project. The primary repo (is_primary = true) corresponds to the legacy
-- project.repo_url field. Tickets can specify which repo they target via target_repo.

CREATE TABLE IF NOT EXISTS project_repositories (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    alias         TEXT NOT NULL,         -- unique per project, e.g. "backend", "frontend", "infra"
    repo_url      TEXT NOT NULL,         -- git clone URL
    default_branch TEXT NOT NULL DEFAULT 'main',
    is_primary    BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (project_id, alias)
);

CREATE INDEX idx_project_repositories_project ON project_repositories(project_id);

-- Add target_repo to tickets: nullable alias referencing which repo this ticket targets.
-- NULL means "use the project's primary repo" (backward compatible).
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS target_repo TEXT;

-- Ensure at most one primary repo per project.
CREATE UNIQUE INDEX IF NOT EXISTS idx_project_repositories_primary
    ON project_repositories(project_id) WHERE is_primary = true;
