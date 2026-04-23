-- Rollback: Remove event outbox tables.
DROP TABLE IF EXISTS event_deliveries;
DROP TABLE IF EXISTS event_outbox;
