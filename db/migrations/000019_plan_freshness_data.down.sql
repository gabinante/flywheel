-- Rollback plan freshness stamps (warrant-45).
DROP TABLE IF EXISTS plan_staleness_thresholds;
DROP INDEX IF EXISTS plans_environment;
ALTER TABLE plans DROP COLUMN IF EXISTS environment;
ALTER TABLE plans DROP COLUMN IF EXISTS freshness_data;
