DROP INDEX IF EXISTS pr_feedback_active;
ALTER TABLE pr_feedback_rounds DROP COLUMN IF EXISTS run_id;
ALTER TABLE workflow_callback_tokens DROP COLUMN IF EXISTS phase_entered_at;
ALTER TABLE workflow_callback_tokens DROP COLUMN IF EXISTS workflow_version;
ALTER TABLE event_deliveries DROP COLUMN IF EXISTS processing_until;

ALTER TABLE event_deliveries DROP COLUMN IF EXISTS last_attempt_at;
