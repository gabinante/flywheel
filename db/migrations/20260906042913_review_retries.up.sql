ALTER TABLE code_review_requests
    ADD COLUMN retry_count integer NOT NULL DEFAULT 0,
    ADD COLUMN retry_at timestamptz;
