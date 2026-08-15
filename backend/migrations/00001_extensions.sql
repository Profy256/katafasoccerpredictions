-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive emails

-- Applied to every table whose rows are published claims about the past:
-- predictions, prediction_results, free_tips, tips, tip_results.
--
-- Corrections are new rows plus an audit_log entry, never an UPDATE. This
-- lives in the database rather than the application because it must also hold
-- for the admin CLI, a migration, a backfill script and a psql session.
-- +goose StatementBegin
CREATE FUNCTION forbid_update() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable once written', TG_TABLE_NAME;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS forbid_update();
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
