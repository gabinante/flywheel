-- State transition history for tickets.
-- Records every state change for timeline visualization and audit.
CREATE TABLE state_transitions (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    ticket_id   TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    trigger     TEXT NOT NULL,
    actor_id    TEXT NOT NULL DEFAULT '',
    actor_type  TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_state_transitions_ticket ON state_transitions(ticket_id, created_at);
