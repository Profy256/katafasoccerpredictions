-- +goose Up

-- The daily free shortlist is materialised at publish time and never
-- recomputed on read.
--
-- Computed live it would be a function of *current* data: once matches finish
-- they stop being scheduled, so yesterday's shortlist could not be
-- reconstructed — and if it could, a model rerun would reconstruct a different
-- one. "We went 4 from 5 yesterday" requires a row saying what yesterday's
-- five were, written before those matches kicked off.
CREATE TABLE free_tip_days (
    day           DATE PRIMARY KEY,
    published_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    model_version TEXT NOT NULL,
    total_tips    SMALLINT NOT NULL
);

CREATE TABLE free_tips (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    day           DATE NOT NULL REFERENCES free_tip_days(day),
    market_code   TEXT NOT NULL REFERENCES market_types(code),
    prediction_id UUID NOT NULL REFERENCES predictions(id),
    -- The odds floor is part of the published record, not a render-time
    -- decision: MIN_PUBLISHABLE_ODDS in internal/tips.
    odds          NUMERIC(7,3) NOT NULL CHECK (odds >= 1.25),
    rank          SMALLINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (day, market_code, rank),
    UNIQUE (day, prediction_id)
);
CREATE INDEX ON free_tips (day, market_code);
CREATE INDEX ON free_tips (prediction_id);

CREATE TRIGGER free_tips_immutable BEFORE UPDATE ON free_tips
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

-- +goose Down
DROP TABLE IF EXISTS free_tips;
DROP TABLE IF EXISTS free_tip_days;
