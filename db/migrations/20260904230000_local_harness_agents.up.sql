BEGIN;
-- One stable operator identity can own several independently registered harnesses.
DROP INDEX agents_user_id_key;
CREATE UNIQUE INDEX agents_operator_user_id_key ON agents(user_id)
    WHERE user_id IS NOT NULL AND api_key IS NULL;
CREATE INDEX agents_user_id_idx ON agents(user_id) WHERE user_id IS NOT NULL;
COMMIT;
