-- +goose Up

CREATE TABLE matches (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id    UUID NOT NULL REFERENCES leagues(id),
    season_id    UUID NOT NULL REFERENCES seasons(id),
    home_team_id UUID NOT NULL REFERENCES teams(id),
    away_team_id UUID NOT NULL REFERENCES teams(id),
    kickoff_at   TIMESTAMPTZ NOT NULL,
    status       TEXT NOT NULL DEFAULT 'scheduled'
                 CHECK (status IN ('scheduled','in_play','finished',
                                   'postponed','cancelled','abandoned')),
    -- Full-time, 90 minutes only. Extra time and penalties are excluded; a cup
    -- tie level after 90 settles as a draw on 1X2.
    home_score   SMALLINT,
    away_score   SMALLINT,
    round        SMALLINT,
    finished_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (home_team_id <> away_team_id),
    -- The one that matters: a match cannot be finished without a score, and
    -- cannot carry a score unless it is finished. Settlement reads
    -- status = 'finished', so a half-written result can never be graded.
    CHECK ((status = 'finished') = (home_score IS NOT NULL AND away_score IS NOT NULL))
);

CREATE INDEX ON matches (kickoff_at);
CREATE INDEX ON matches (league_id, kickoff_at);
CREATE INDEX ON matches (status, kickoff_at) WHERE status IN ('scheduled','in_play');
-- Strength estimation reads a league's played history in kickoff order.
CREATE INDEX ON matches (league_id, kickoff_at) WHERE status = 'finished';

CREATE TABLE match_sources (
    match_id          UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL CHECK (provider IN ('football-data','api-football')),
    provider_match_id TEXT NOT NULL,
    PRIMARY KEY (provider, provider_match_id)
);
CREATE INDEX ON match_sources (match_id);

-- +goose StatementBegin
CREATE FUNCTION touch_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER matches_touch_updated_at BEFORE UPDATE ON matches
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS matches_touch_updated_at ON matches;
DROP FUNCTION IF EXISTS touch_updated_at();
DROP TABLE IF EXISTS match_sources;
DROP TABLE IF EXISTS matches;
