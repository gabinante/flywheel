-- Workflow phase status tracking: enables async phase processing and accurate timing.
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS workflow_phase_status TEXT NOT NULL DEFAULT '';
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS workflow_phase_entered_at TIMESTAMPTZ;

-- Callback tokens for async external phases (replaces Redis-based storage).
CREATE TABLE IF NOT EXISTS workflow_callback_tokens (
    token       TEXT PRIMARY KEY,
    ticket_id   TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    workflow_id TEXT NOT NULL,
    phase_id    TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_wf_cb_ticket ON workflow_callback_tokens (ticket_id);
