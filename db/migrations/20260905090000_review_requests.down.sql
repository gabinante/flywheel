ALTER TABLE code_review_requests
    DROP COLUMN last_requested_event_id,
    DROP COLUMN pending_requested_at,
    DROP COLUMN request_handled_at;
