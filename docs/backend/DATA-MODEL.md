# Data model

PostgreSQL 16+. Every timestamp is `TIMESTAMPTZ` stored in UTC. Every money
column is `BIGINT` UGX shillings. Every odds column is `NUMERIC(7,3)`.

Enums are `TEXT` + `CHECK` rather than native `ENUM` types — adding a value to a
Postgres enum is fine, but removing or reordering one requires a table rewrite,
and market codes will change as the product grows.

## Conventions

- Extensions required in the first migration: `pgcrypto` (for
  `gen_random_uuid()`) and `citext` (for case-insensitive emails).
- Primary keys are `UUID`, except lookup tables keyed by their natural code
  (`market_types`, `packages`).
- Migration order is `users` → `leagues`/`seasons`/`teams` → `matches` →
  `market_types`/`predictions` → Pro tier → payments, since `slips.created_by`
  references `users`.
- Provider identifiers never appear on core tables. They live in `*_sources`
  join tables, because one real-world match can arrive from two providers.
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()` on every table.

---

## Reference data

```sql
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
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seasons (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id  UUID NOT NULL REFERENCES leagues(id),
    label      TEXT NOT NULL,          -- '2025/26'
    start_year SMALLINT NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT FALSE,
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
```

`region` gains `'asia'` over the frontend's current union — add it to
`Region` in `src/api/types.ts` and to `REGIONS` when the backend lands.

## Matches

```sql
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
    home_score   SMALLINT,
    away_score   SMALLINT,
    round        SMALLINT,
    finished_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (home_team_id <> away_team_id),
    CHECK ((status = 'finished') = (home_score IS NOT NULL AND away_score IS NOT NULL))
);

CREATE INDEX ON matches (kickoff_at);
CREATE INDEX ON matches (league_id, kickoff_at);
CREATE INDEX ON matches (status, kickoff_at) WHERE status IN ('scheduled','in_play');
```

The final `CHECK` is the one that matters: a match cannot be `finished` without
a score, and cannot carry a score unless it is `finished`. Settlement reads
`status = 'finished'`, so a half-written result can never be graded.

`matches` is the only table the frontend's `MatchStatus` narrows — it exposes
`'scheduled' | 'finished'`. Map `in_play` to `scheduled` and the three
non-completions to a `void` presentation at the API boundary.

```sql
CREATE TABLE match_sources (
    match_id          UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL CHECK (provider IN ('football-data','api-football')),
    provider_match_id TEXT NOT NULL,
    PRIMARY KEY (provider, provider_match_id)
);
CREATE INDEX ON match_sources (match_id);
```

`league_sources` and `team_sources` follow the same shape. These exist so that
switching a competition from one provider to another does not orphan its
history.

## Predictions

```sql
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

    UNIQUE (match_id, market_code, model_version)
);

CREATE INDEX ON predictions (match_id);
CREATE INDEX ON predictions (created_at);
```

The unique constraint permits a new `model_version` to produce a second row for
the same match and market. It does **not** permit overwriting the first. A model
upgrade adds predictions; it never rewrites what was published, and the accuracy
dashboard reports per `model_version`.

### Trigger: no prediction after kickoff

```sql
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

CREATE TRIGGER predictions_before_kickoff
    BEFORE INSERT ON predictions
    FOR EACH ROW EXECUTE FUNCTION assert_prediction_before_kickoff();
```

In the database rather than the application because it must hold for the admin
CLI, a migration, a backfill script and a psql session too. This is the rule an
outsider would check first if they doubted the record.

### Trigger: immutability

```sql
CREATE FUNCTION forbid_update() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable once written', TG_TABLE_NAME;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER predictions_immutable BEFORE UPDATE ON predictions
    FOR EACH ROW EXECUTE FUNCTION forbid_update();
```

Applied to `predictions`, `prediction_results`, `free_tips`, `tips`, and
`tip_results`. `slips` needs a narrower version — see below.

### Results

```sql
CREATE TABLE prediction_results (
    prediction_id  UUID PRIMARY KEY REFERENCES predictions(id),
    actual_outcome TEXT NOT NULL,
    was_correct    BOOLEAN NOT NULL,
    settled_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Primary key on `prediction_id` is the idempotency guarantee: settling twice
raises a conflict instead of double-counting a win.

### Reasoning

```sql
CREATE TABLE match_reasoning (
    match_id       UUID PRIMARY KEY REFERENCES matches(id),
    xg_home        NUMERIC(5,3) NOT NULL,
    xg_away        NUMERIC(5,3) NOT NULL,
    home_form      JSONB NOT NULL,
    away_form      JSONB NOT NULL,
    head_to_head   JSONB NOT NULL,
    top_scorelines JSONB NOT NULL,
    sample_home    SMALLINT NOT NULL,
    sample_away    SMALLINT NOT NULL,
    model_version  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Stored as a snapshot, not recomputed on read. The whole point of the
`/matches/[id]` page is showing what the model saw *at the time it decided* —
recomputing it from today's data would show a different, flattering picture.
`sample_home`/`sample_away` back the honest small-sample disclosure the frontend
already renders.

## Free tier

```sql
CREATE TABLE free_tip_days (
    day          DATE PRIMARY KEY,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    model_version TEXT NOT NULL,
    total_tips   SMALLINT NOT NULL
);

CREATE TABLE free_tips (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    day           DATE NOT NULL REFERENCES free_tip_days(day),
    market_code   TEXT NOT NULL REFERENCES market_types(code),
    prediction_id UUID NOT NULL REFERENCES predictions(id),
    odds          NUMERIC(7,3) NOT NULL CHECK (odds >= 1.25),
    rank          SMALLINT NOT NULL,
    UNIQUE (day, market_code, rank),
    UNIQUE (day, prediction_id)
);
```

This table is why the free tier can be audited at all. See
[SETTLEMENT.md § Why the free shortlist is frozen](SETTLEMENT.md#why-the-free-shortlist-is-frozen).

## Pro tier

```sql
CREATE TABLE analysts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    handle     TEXT NOT NULL UNIQUE,
    initials   TEXT NOT NULL,
    bio        TEXT NOT NULL DEFAULT '',
    joined_at  TIMESTAMPTZ NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE
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
    created_by   UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK ((status = 'draft') = (published_at IS NULL)),
    CHECK ((status = 'settled') = (settled_at IS NOT NULL))
);

CREATE INDEX ON slips (status, published_at DESC);
CREATE INDEX ON slips (package_code, published_at DESC);
```

`slips` is the one table with a legitimate lifecycle, so it cannot be fully
immutable. It is constrained instead:

```sql
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
```

Price cannot move after publication — otherwise a slip could be repriced after
buyers committed. Status may advance `draft → open → settled` and never back.

```sql
CREATE TABLE slip_analysts (
    slip_id    UUID NOT NULL REFERENCES slips(id) ON DELETE CASCADE,
    analyst_id UUID NOT NULL REFERENCES analysts(id),
    PRIMARY KEY (slip_id, analyst_id)
);

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
    -- auto-gradable requires all three; otherwise none
    CHECK (num_nonnulls(match_id, market_code, selection_value) IN (0, 3))
);

CREATE TABLE tip_results (
    tip_id         UUID PRIMARY KEY REFERENCES tips(id),
    was_correct    BOOLEAN NOT NULL,
    actual_outcome TEXT NOT NULL,
    settled_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_by     TEXT NOT NULL CHECK (settled_by IN ('auto','admin')),
    settled_by_user UUID REFERENCES users(id),
    CHECK ((settled_by = 'admin') = (settled_by_user IS NOT NULL))
);
```

The `num_nonnulls` check encodes the frontend's rule that a tip is either fully
structured and auto-gradable, or free text an admin grades by hand. A tip with a
`match_id` but no `selection_value` is neither, and would silently never settle.

`settled_by_user` is not optional for admin settlements. A human overriding the
record must be named in it.

## Accounts and payments

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    phone         TEXT,
    role          TEXT NOT NULL DEFAULT 'user'
                  CHECK (role IN ('user','analyst','admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,          -- sha256 of the opaque token
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX ON sessions (user_id);
CREATE INDEX ON sessions (expires_at);
```

Opaque tokens hashed at rest, not JWTs. A purchase must be revocable
immediately; a stolen JWT stays valid until it expires, and slip access is worth
money.

```sql
CREATE TABLE purchases (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    slip_id    UUID NOT NULL REFERENCES slips(id),
    price_ugx  BIGINT NOT NULL CHECK (price_ugx > 0),
    status     TEXT NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending','paid','failed','refunded')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at    TIMESTAMPTZ,
    -- A refunded purchase keeps the paid_at it was given when the money
    -- arrived. Refunding does not un-happen the payment, and when it arrived
    -- is part of the record — see PAYMENTS.md § Refunds.
    CHECK ((status IN ('paid','refunded')) = (paid_at IS NOT NULL))
);

-- one *paid* purchase per user per slip; retries after failure stay allowed
CREATE UNIQUE INDEX purchases_one_paid
    ON purchases (user_id, slip_id) WHERE status = 'paid';
```

`price_ugx` is copied onto the purchase rather than read from the slip. It is
what the user was actually charged, and it must survive independently of the
slip row.

Payment tables are in [PAYMENTS.md](PAYMENTS.md).

## Operational tables

```sql
CREATE TABLE provider_payloads (
    id           BIGSERIAL PRIMARY KEY,
    provider     TEXT NOT NULL,
    endpoint     TEXT NOT NULL,
    request_url  TEXT NOT NULL,
    http_status  SMALLINT NOT NULL,
    body         JSONB,
    content_hash BYTEA NOT NULL,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON provider_payloads (provider, endpoint, fetched_at DESC);

CREATE TABLE provider_budget (
    provider      TEXT NOT NULL,
    day           DATE NOT NULL,
    requests_used INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (provider, day)
);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    actor_type  TEXT NOT NULL CHECK (actor_type IN ('system','admin','job')),
    actor_id    UUID,
    action      TEXT NOT NULL,
    entity      TEXT NOT NULL,
    entity_id   UUID,
    before      JSONB,
    after       JSONB,
    reason      TEXT,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON audit_log (entity, entity_id, at DESC);
```

`provider_payloads` is the receipt drawer. When a settlement is disputed, the
answer is the archived response that produced it — retention 180 days, then
partition-drop. It also makes provider client changes replayable offline
without spending request budget.

## Rollups

```sql
CREATE MATERIALIZED VIEW accuracy_rollup AS
SELECT p.model_version, p.market_code, m.league_id,
       width_bucket(p.confidence_pct, 50, 90, 4) AS confidence_band,
       date_trunc('day', r.settled_at)::date     AS settled_day,
       count(*)                                  AS total,
       count(*) FILTER (WHERE r.was_correct)     AS correct
FROM prediction_results r
JOIN predictions p ON p.id = r.prediction_id
JOIN matches     m ON m.id = p.match_id
GROUP BY 1,2,3,4,5;

CREATE UNIQUE INDEX ON accuracy_rollup
    (model_version, market_code, league_id, confidence_band, settled_day);
```

The unique index is required for `REFRESH MATERIALIZED VIEW CONCURRENTLY`,
which is what keeps the accuracy page readable during a refresh.

No `WHERE` clause filters anything out. Every settled prediction is in there,
which is the point — see non-negotiable 5.

`analyst_rollup` mirrors this over `tip_results` joined to `tips` and `slips`,
carrying `profit_units` as `SUM(odds - 1) FILTER (WHERE was_correct) - COUNT(*)
FILTER (WHERE NOT was_correct)` to match the frontend's flat-stake definition.
