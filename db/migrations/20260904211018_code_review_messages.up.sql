-- Conversation with the agent that performed a code review (operator asks, agent answers
-- by resuming its own harness session).
CREATE TABLE IF NOT EXISTS code_review_messages (
    id          TEXT PRIMARY KEY,
    review_id   TEXT NOT NULL REFERENCES code_review_requests(id) ON DELETE CASCADE,
    role        TEXT NOT NULL, -- user | assistant | system
    content     TEXT NOT NULL,
    session_id  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_code_review_messages_review ON code_review_messages (review_id, created_at);
