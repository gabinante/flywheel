-- Reverse multi-repo migration.
ALTER TABLE tickets DROP COLUMN IF EXISTS target_repo;
DROP INDEX IF EXISTS idx_project_repositories_primary;
DROP INDEX IF EXISTS idx_project_repositories_project;
DROP TABLE IF EXISTS project_repositories;
