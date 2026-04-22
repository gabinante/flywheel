-- Drop append-only triggers
DROP TRIGGER IF EXISTS change_stream_no_delete ON change_stream;
DROP TRIGGER IF EXISTS change_stream_no_update ON change_stream;
DROP TRIGGER IF EXISTS state_stream_no_delete ON state_stream;
DROP TRIGGER IF EXISTS state_stream_no_update ON state_stream;
DROP TRIGGER IF EXISTS entity_stream_no_delete ON entity_stream;
DROP TRIGGER IF EXISTS entity_stream_no_update ON entity_stream;
DROP FUNCTION IF EXISTS deny_stream_mutation();

-- Drop tables
DROP TABLE IF EXISTS change_stream;
DROP TABLE IF EXISTS state_stream;
DROP TABLE IF EXISTS entity_stream;
