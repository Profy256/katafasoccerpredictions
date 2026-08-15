-- +goose Up

-- The receipt drawer. When a settlement is disputed, the answer is the
-- archived response that produced it. It also makes provider client changes
-- replayable offline without spending request budget.
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
-- Retention is 180 days, then drop. See katafa prune-payloads.
CREATE INDEX ON provider_payloads (fetched_at);

CREATE TABLE provider_budget (
    provider      TEXT NOT NULL,
    day           DATE NOT NULL,
    requests_used INTEGER NOT NULL DEFAULT 0,
    -- Set when a 429 is seen, so the limiter stays halved for the rest of the
    -- day rather than resetting on the next worker restart.
    throttled_at  TIMESTAMPTZ,
    PRIMARY KEY (provider, day)
);

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('system','admin','job')),
    actor_id   UUID,
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    entity_id  UUID,
    before     JSONB,
    after      JSONB,
    reason     TEXT,
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON audit_log (entity, entity_id, at DESC);
CREATE INDEX ON audit_log (at DESC);

-- POST /admin/matches/{id}/correct is the only path that may change a finished
-- score. Because prediction_results is immutable it does not silently re-grade:
-- it flags the affected predictions here, and the team publishes a correction.
CREATE TABLE correction_reviews (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id      UUID NOT NULL REFERENCES matches(id),
    prediction_id UUID REFERENCES predictions(id),
    reason        TEXT NOT NULL,
    resolved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON correction_reviews (resolved_at) WHERE resolved_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS correction_reviews;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS provider_budget;
DROP TABLE IF EXISTS provider_payloads;
