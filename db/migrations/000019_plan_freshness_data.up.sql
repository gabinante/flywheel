-- Plan freshness stamps and re-plan-before-apply mechanics (warrant-45).
-- Adds rich freshness data (commit SHA, state-index snapshot, claims-registry snapshot,
-- observed entity versions) and per-environment staleness configuration.

-- Rich freshness data: structured JSONB carrying commit SHA, state-index timestamp,
-- claims-registry timestamp, and observed entity versions at plan-generation time.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS freshness_data JSONB;

-- Environment the plan targets (e.g., "dev", "staging", "prod").
-- Used to select per-environment staleness thresholds.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS environment TEXT NOT NULL DEFAULT '';

-- Index for querying plans by environment (used by staleness sweep).
CREATE INDEX IF NOT EXISTS plans_environment ON plans(environment) WHERE environment != '';

-- Per-environment staleness thresholds configuration table.
-- The dispatcher reads these to determine whether plans need re-plan before apply.
CREATE TABLE IF NOT EXISTS plan_staleness_thresholds (
    environment             TEXT PRIMARY KEY,
    max_age_seconds         INT NOT NULL DEFAULT 0,
    require_re_plan         BOOLEAN NOT NULL DEFAULT true,
    allow_apply_anyway_classes TEXT[] NOT NULL DEFAULT '{}',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed conservative defaults: prod always re-plans, staging allows 30 min, dev allows 4 hours.
INSERT INTO plan_staleness_thresholds (environment, max_age_seconds, require_re_plan, allow_apply_anyway_classes)
VALUES
    ('prod', 0, true, '{}'),
    ('staging', 1800, true, '{staging-deploy}'),
    ('dev', 14400, false, '{}')
ON CONFLICT (environment) DO NOTHING;
