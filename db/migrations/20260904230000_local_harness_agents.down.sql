BEGIN;
-- Fail without changing data if an operator still owns multiple agents. Revoke or
-- unlink additional harness registrations before restoring the old constraint.
DROP INDEX agents_operator_user_id_key;
DROP INDEX agents_user_id_idx;
CREATE UNIQUE INDEX agents_user_id_key ON agents(user_id) WHERE user_id IS NOT NULL;
COMMIT;
