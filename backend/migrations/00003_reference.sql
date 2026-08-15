-- +goose Up

CREATE TABLE leagues (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    short_name   TEXT NOT NULL,
    country      TEXT NOT NULL,
    country_code TEXT NOT NULL,
    tier         SMALLINT NOT NULL DEFAULT 1,
    region       TEXT NOT NULL
                 CHECK (region IN ('europe','east-africa','africa','americas','asia')),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    -- Publication gate. A league whose teams have too little history produces
    -- worthless strength estimates, so it runs in shadow mode: predictions are
    -- generated and settled internally, nothing is published, until the
    -- calibration chart says the model is sane there.
    is_published BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seasons (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id  UUID NOT NULL REFERENCES leagues(id),
    label      TEXT NOT NULL,          -- '2025/26'
    start_year SMALLINT NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (league_id, start_year)
);
CREATE UNIQUE INDEX ON seasons (league_id) WHERE is_current;

CREATE TABLE teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id  UUID NOT NULL REFERENCES leagues(id),
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    short_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON teams (league_id);

-- Provider identifiers never appear on the core tables above. They live here,
-- because one real-world league, team or match can arrive from two providers,
-- and switching a competition between providers must not orphan its history.
CREATE TABLE competition_sources (
    league_id               UUID NOT NULL REFERENCES leagues(id),
    provider                TEXT NOT NULL CHECK (provider IN ('football-data','api-football')),
    provider_competition_id TEXT NOT NULL,
    is_primary              BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (league_id, provider),
    UNIQUE (provider, provider_competition_id)
);
CREATE UNIQUE INDEX ON competition_sources (league_id) WHERE is_primary;

CREATE TABLE team_sources (
    team_id          UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('football-data','api-football')),
    provider_team_id TEXT NOT NULL,
    PRIMARY KEY (provider, provider_team_id)
);
CREATE INDEX ON team_sources (team_id);

-- +goose Down
DROP TABLE IF EXISTS team_sources;
DROP TABLE IF EXISTS competition_sources;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS leagues;
