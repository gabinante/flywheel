ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS processing_until timestamptz;
ALTER TABLE workflow_callback_tokens ADD COLUMN IF NOT EXISTS workflow_version integer;
ALTER TABLE workflow_callback_tokens ADD COLUMN IF NOT EXISTS phase_entered_at timestamptz;
ALTER TABLE pr_feedback_rounds ADD COLUMN IF NOT EXISTS run_id uuid;
CREATE INDEX IF NOT EXISTS pr_feedback_active ON pr_feedback_rounds(repo,number) WHERE state = 'dispatched';

ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz;
