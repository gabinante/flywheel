-- Deduplicate explicit GitHub review requests independently of PR head changes.
ALTER TABLE code_review_requests
    ADD COLUMN last_requested_event_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN pending_requested_at timestamptz,
    ADD COLUMN request_handled_at timestamptz NOT NULL DEFAULT now();
UPDATE code_review_requests
    SET request_handled_at = CASE WHEN state='closed' THEN updated_at ELSE COALESCE(reviewed_at, created_at) END;
