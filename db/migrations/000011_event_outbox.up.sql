-- Migration: Add event outbox table for durable event bus (spec v0.2 Layer 6).
-- Events are persisted here before delivery. Delivery uses Postgres LISTEN/NOTIFY
-- for real-time push with polling fallback for reliability.
--
-- At-least-once semantics: events remain in outbox until all subscribers ACK.
-- Ordering: events with the same entity_key are delivered in sequence order.

CREATE TABLE IF NOT EXISTS event_outbox (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    event_type      TEXT NOT NULL,
    entity_key      TEXT NOT NULL DEFAULT '',
    payload         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Sequence number for ordering within an entity key.
    -- Monotonically increasing per entity_key.
    sequence        BIGSERIAL
);

-- Index for polling: find undelivered events efficiently.
CREATE INDEX idx_event_outbox_created ON event_outbox(created_at);

-- Index for entity-key ordered delivery.
CREATE INDEX idx_event_outbox_entity_seq ON event_outbox(entity_key, sequence);

-- Index for event type filtering (used by pattern matching on poll).
CREATE INDEX idx_event_outbox_type ON event_outbox(event_type);

-- Delivery tracking: records which subscribers have acknowledged each event.
-- An event is fully delivered when all active subscribers have an entry here.
CREATE TABLE IF NOT EXISTS event_deliveries (
    event_id        TEXT NOT NULL REFERENCES event_outbox(id) ON DELETE CASCADE,
    subscriber_id   TEXT NOT NULL,
    delivered_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    acked_at        TIMESTAMPTZ,
    PRIMARY KEY (event_id, subscriber_id)
);

-- Index for finding unacked deliveries for a subscriber (polling fallback).
CREATE INDEX idx_event_deliveries_subscriber_unacked
    ON event_deliveries(subscriber_id, delivered_at)
    WHERE acked_at IS NULL;

-- Retention: events older than 7 days that are fully acked can be pruned.
-- This is handled by a background job, not this migration.

-- Notify channel for real-time delivery.
-- Publishers call pg_notify('event_bus', event_id) after insert.
-- The comment below documents the channel name for the application:
COMMENT ON TABLE event_outbox IS 'Durable event bus outbox. Notify channel: event_bus';
