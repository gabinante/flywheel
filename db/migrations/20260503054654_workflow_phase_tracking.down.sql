DROP TABLE IF EXISTS workflow_callback_tokens;
ALTER TABLE tickets DROP COLUMN IF EXISTS workflow_phase_entered_at;
ALTER TABLE tickets DROP COLUMN IF EXISTS workflow_phase_status;
