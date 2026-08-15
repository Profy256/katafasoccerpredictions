-- +goose Up

CREATE TABLE analysts (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug      TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL,
    handle    TEXT NOT NULL UNIQUE,
    initials  TEXT NOT NULL,
    bio       TEXT NOT NULL DEFAULT '',
    joined_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE packages (
    code              TEXT PRIMARY KEY
                      CHECK (code IN ('ordinary','vip','akatambula')),
    name              TEXT NOT NULL,
    tagline           TEXT NOT NULL,
    description       TEXT NOT NULL,
    typical_price_ugx BIGINT NOT NULL CHECK (typical_price_ugx > 0),
    highlights        JSONB NOT NULL,
    sort_order        SMALLINT NOT NULL
);

CREATE TABLE slips (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_code TEXT NOT NULL REFERENCES packages(code),
    title        TEXT NOT NULL,
    price_ugx    BIGINT NOT NULL CHECK (price_ugx > 0),
    total_odds   NUMERIC(10,3) NOT NULL CHECK (total_odds >= 1),
    tip_count    SMALLINT NOT NULL CHECK (tip_count > 0),
    status       TEXT NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft','open','settled','void')),
    published_at TIMESTAMPTZ,
    settled_at   TIMESTAMPTZ,
    won_tips     SMALLINT,
    -- The accumulator after void legs were removed. Written separately because
    -- total_odds is frozen at publication and must keep showing what buyers
    -- were advertised; both figures are displayed.
    settled_odds NUMERIC(10,3),
    created_by   UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK ((status = 'draft') = (published_at IS NULL)),
    CHECK ((status = 'settled') = (settled_at IS NOT NULL))
);

CREATE INDEX ON slips (status, published_at DESC);
CREATE INDEX ON slips (package_code, published_at DESC);

-- slips is the one table with a legitimate lifecycle, so it cannot be fully
-- immutable. It is constrained instead: price cannot move after publication,
-- or a slip could be repriced after buyers committed, and status advances
-- draft → open → settled and never back.
-- +goose StatementBegin
CREATE FUNCTION slips_guard_update() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status <> 'draft' THEN
        IF NEW.price_ugx    <> OLD.price_ugx
        OR NEW.package_code <> OLD.package_code
        OR NEW.title        <> OLD.title
        OR NEW.total_odds   <> OLD.total_odds
        OR NEW.tip_count    <> OLD.tip_count
        OR NEW.published_at IS DISTINCT FROM OLD.published_at THEN
            RAISE EXCEPTION 'slip % is published; commercial terms are frozen', OLD.id;
        END IF;
    END IF;
    IF OLD.status = 'settled' AND NEW.status <> 'settled' THEN
        RAISE EXCEPTION 'slip % is settled and cannot reopen', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER slips_guarded BEFORE UPDATE ON slips
    FOR EACH ROW EXECUTE FUNCTION slips_guard_update();

CREATE TABLE slip_analysts (
    slip_id    UUID NOT NULL REFERENCES slips(id) ON DELETE CASCADE,
    analyst_id UUID NOT NULL REFERENCES analysts(id),
    PRIMARY KEY (slip_id, analyst_id)
);
CREATE INDEX ON slip_analysts (analyst_id);

CREATE TABLE tips (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slip_id         UUID NOT NULL REFERENCES slips(id) ON DELETE CASCADE,
    analyst_id      UUID NOT NULL REFERENCES analysts(id),
    match_id        UUID REFERENCES matches(id),
    fixture_label   TEXT NOT NULL,
    market_label    TEXT NOT NULL,
    selection_label TEXT NOT NULL,
    market_code     TEXT REFERENCES market_types(code),
    selection_value TEXT,
    odds            NUMERIC(7,3) NOT NULL CHECK (odds > 1),
    kickoff_at      TIMESTAMPTZ NOT NULL,
    note            TEXT,
    position        SMALLINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (slip_id, position),
    -- A tip is either fully structured and auto-gradable, or free text an
    -- admin grades by hand. One with a match_id but no selection_value is
    -- neither, and would silently never settle.
    CHECK (num_nonnulls(match_id, market_code, selection_value) IN (0, 3))
);
CREATE INDEX ON tips (slip_id);
CREATE INDEX ON tips (analyst_id);
CREATE INDEX ON tips (match_id) WHERE match_id IS NOT NULL;

CREATE TRIGGER tips_immutable BEFORE UPDATE ON tips
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

CREATE TABLE tip_results (
    tip_id          UUID PRIMARY KEY REFERENCES tips(id),
    was_correct     BOOLEAN NOT NULL,
    actual_outcome  TEXT NOT NULL,
    settled_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_by      TEXT NOT NULL CHECK (settled_by IN ('auto','admin')),
    settled_by_user UUID REFERENCES users(id),
    -- A human overriding the record must be named in it.
    CHECK ((settled_by = 'admin') = (settled_by_user IS NOT NULL))
);
CREATE INDEX ON tip_results (settled_at);

CREATE TRIGGER tip_results_immutable BEFORE UPDATE ON tip_results
    FOR EACH ROW EXECUTE FUNCTION forbid_update();

-- +goose Down
DROP TABLE IF EXISTS tip_results;
DROP TABLE IF EXISTS tips;
DROP TABLE IF EXISTS slip_analysts;
DROP TRIGGER IF EXISTS slips_guarded ON slips;
DROP FUNCTION IF EXISTS slips_guard_update();
DROP TABLE IF EXISTS slips;
DROP TABLE IF EXISTS packages;
DROP TABLE IF EXISTS analysts;
