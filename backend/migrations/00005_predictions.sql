-- +goose Up

CREATE TABLE market_types (
    code         TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    short_name   TEXT NOT NULL,
    tab_label    TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    outcomes     JSONB NOT NULL,   -- [{value,label,shortLabel}, …] in display order
    sort_order   SMALLINT NOT NULL
);

CREATE TABLE predictions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id         UUID NOT NULL REFERENCES matches(id),
    market_code      TEXT NOT NULL REFERENCES market_types(code),
    prediction_value TEXT NOT NULL,
    confidence_pct   NUMERIC(5,2) NOT NULL CHECK (confidence_pct BETWEEN 0 AND 100),
    distribution     JSONB NOT NULL,
    model_version    TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Permits a new model_version to produce a second row for the same match
    -- and market. Does not permit overwriting the first: a model upgrade adds
    -- predictions, it never rewrites what was published.
    UNIQUE (match_id, market_code, model_version)
);

CREATE INDEX ON predictions (match_id);
CREATE INDEX ON predictions (created_at);

-- A pick written after the whistle is not a prediction. Enforced by trigger,
-- not by convention, because this is the rule an outsider would check first if
-- they doubted the record.
-- +goose StatementBegin
CREATE FUNCTION assert_prediction_before_kickoff() RETURNS TRIGGER AS $$
DECLARE ko TIMESTAMPTZ;
BEGIN
    SELECT kickoff_at INTO ko FROM matches WHERE id = NEW.match_id;
    IF NEW.created_at >= ko THEN
        RAISE EXCEPTION
          'prediction % for match % created at % is not before kickoff %',
          NEW.id, NEW.match_id, NEW.created_at, ko;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER predictions_before_kickoff
    BEFORE INSERT ON predictions
    FOR EACH ROW EXECUTE FUNCTION assert_prediction_before_kickoff();

CREATE TRIGGER predictions_immutable BEFORE UPDATE ON predictions
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

CREATE TABLE prediction_results (
    prediction_id  UUID PRIMARY KEY REFERENCES predictions(id),
    actual_outcome TEXT NOT NULL,
    was_correct    BOOLEAN NOT NULL,
    settled_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON prediction_results (settled_at);

-- Primary key on prediction_id is the idempotency guarantee: settling twice
-- conflicts instead of double-counting a win.
CREATE TRIGGER prediction_results_immutable BEFORE UPDATE ON prediction_results
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

-- A voided prediction has no outcome to report, so it gets no result row. It
-- is recorded here rather than left as an indistinguishable pending row, for
-- two reasons: the gap between "predictions made" and "predictions settled"
-- must be explainable in SQL, and settlement must stop re-examining a
-- cancelled match every 30 minutes forever.
--
-- Voiding is not the same as excluding a loss. A lost pick has an outcome and
-- is counted; a voided one never produced a full-time score.
CREATE TABLE prediction_voids (
    prediction_id UUID PRIMARY KEY REFERENCES predictions(id),
    reason        TEXT NOT NULL,
    voided_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER prediction_voids_immutable BEFORE UPDATE ON prediction_voids
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

-- A snapshot of what the model saw at the time it decided, never recomputed on
-- read. Recomputing from today's data would show a different, flattering
-- picture on the match detail page.
CREATE TABLE match_reasoning (
    match_id       UUID PRIMARY KEY REFERENCES matches(id),
    xg_home        NUMERIC(5,3) NOT NULL,
    xg_away        NUMERIC(5,3) NOT NULL,
    home_form      JSONB NOT NULL,
    away_form      JSONB NOT NULL,
    head_to_head   JSONB NOT NULL,
    top_scorelines JSONB NOT NULL,
    -- Backs the honest small-sample disclosure the frontend already renders.
    sample_home    SMALLINT NOT NULL,
    sample_away    SMALLINT NOT NULL,
    model_version  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS match_reasoning;
DROP TABLE IF EXISTS prediction_voids;
DROP TABLE IF EXISTS prediction_results;
DROP TRIGGER IF EXISTS predictions_immutable ON predictions;
DROP TRIGGER IF EXISTS predictions_before_kickoff ON predictions;
DROP FUNCTION IF EXISTS assert_prediction_before_kickoff();
DROP TABLE IF EXISTS predictions;
DROP TABLE IF EXISTS market_types;
