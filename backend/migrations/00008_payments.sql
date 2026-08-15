-- +goose Up

CREATE TABLE purchases (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    slip_id    UUID NOT NULL REFERENCES slips(id),
    -- Copied onto the purchase rather than read from the slip: it is what the
    -- user was actually charged, and must survive independently of the slip.
    price_ugx  BIGINT NOT NULL CHECK (price_ugx > 0),
    status     TEXT NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending','paid','failed','refunded')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at    TIMESTAMPTZ,
    CHECK ((status = 'paid') = (paid_at IS NOT NULL))
);
CREATE INDEX ON purchases (user_id, created_at DESC);
CREATE INDEX ON purchases (slip_id);

-- One *paid* purchase per user per slip; retries after a failure stay allowed.
-- This plus the guarded UPDATE in webhook processing makes a double grant
-- structurally impossible rather than merely unlikely.
CREATE UNIQUE INDEX purchases_one_paid
    ON purchases (user_id, slip_id) WHERE status = 'paid';

CREATE TABLE payment_transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_id     UUID NOT NULL REFERENCES purchases(id),
    provider        TEXT NOT NULL DEFAULT 'marzpay',
    -- Our idempotency key, sent as `reference`. MarzPay enforces uniqueness on
    -- it, so a retried attempt generates a new one and a new row against the
    -- same purchase; reusing a reference is rejected by the provider, which is
    -- the desired behaviour.
    reference       UUID NOT NULL UNIQUE,
    provider_uuid   TEXT,                     -- MarzPay transaction uuid
    provider_txn_id TEXT,                     -- provider_transaction_id from the callback
    status          TEXT NOT NULL DEFAULT 'initiated'
                    CHECK (status IN ('initiated','processing','pending',
                                      'completed','failed','expired')),
    amount_ugx      BIGINT NOT NULL CHECK (amount_ugx BETWEEN 500 AND 10000000),
    phone_number    TEXT NOT NULL,
    mobile_provider TEXT,                     -- mtn | airtel
    raw_request     JSONB NOT NULL,
    raw_response    JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at      TIMESTAMPTZ
);
CREATE INDEX ON payment_transactions (status, created_at)
    WHERE status IN ('initiated','processing','pending');
CREATE INDEX ON payment_transactions (provider_txn_id);
CREATE INDEX ON payment_transactions (purchase_id);

CREATE TRIGGER payment_transactions_touch_updated_at BEFORE UPDATE ON payment_transactions
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- Append-only, and written *before* any processing. Every callback that
-- arrives is recorded even if it is malformed, unsigned, or about a
-- transaction we do not recognise. When a user says they were charged and got
-- nothing, this table is the first place to look, and it must not have gaps.
CREATE TABLE payment_webhook_events (
    id              BIGSERIAL PRIMARY KEY,
    provider        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    provider_txn_id TEXT,
    reference       TEXT,
    payload         JSONB NOT NULL,
    signature_valid BOOLEAN NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ,
    process_error   TEXT
);
CREATE INDEX ON payment_webhook_events (processed_at) WHERE processed_at IS NULL;
CREATE INDEX ON payment_webhook_events (provider_txn_id);
CREATE INDEX ON payment_webhook_events (reference);

CREATE TABLE refunds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_id UUID NOT NULL REFERENCES purchases(id),
    amount_ugx  BIGINT NOT NULL CHECK (amount_ugx > 0),
    reason      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','sent','completed','failed')),
    provider_uuid TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Partial refunds are not supported: a slip is one indivisible product.
CREATE UNIQUE INDEX refunds_one_per_purchase ON refunds (purchase_id);

CREATE TRIGGER refunds_touch_updated_at BEFORE UPDATE ON refunds
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- +goose Down
DROP TABLE IF EXISTS refunds;
DROP TABLE IF EXISTS payment_webhook_events;
DROP TABLE IF EXISTS payment_transactions;
DROP TABLE IF EXISTS purchases;
