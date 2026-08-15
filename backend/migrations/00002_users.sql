-- +goose Up

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

-- Opaque 32-byte tokens hashed at rest, not JWTs. A purchase must be revocable
-- immediately; a stolen JWT stays valid until it expires, and slip access is
-- worth money.
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

-- Rate limiting lives in Postgres, not in memory, so it survives a restart and
-- works across replicas. One row per (bucket, window); the window start is
-- truncated by the caller so counting is a plain upsert.
CREATE TABLE rate_limits (
    bucket       TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket, window_start)
);
CREATE INDEX ON rate_limits (window_start);

-- +goose Down
DROP TABLE IF EXISTS rate_limits;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
