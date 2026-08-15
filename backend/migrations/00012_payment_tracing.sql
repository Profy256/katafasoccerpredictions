-- +goose Up

-- Money arriving in the MarzPay account has to be attributable to a slip and a
-- buyer. The reference UUID is exact but unreadable; trace_code is the short
-- form embedded in the payment description, so it is what appears on a MarzPay
-- statement and in the payer's SMS.
--
-- Generated rather than written by the application: it is a pure function of
-- the reference, and a column the application could set is a column the
-- application could set inconsistently.
ALTER TABLE payment_transactions
    ADD COLUMN trace_code TEXT
    GENERATED ALWAYS AS (
        'KTF-' || upper(substring(replace(reference::text, '-', '') from 1 for 8))
    ) STORED;

-- The lookup that matters operationally: somebody reads a code off a
-- statement, or a user quotes it from an SMS, and it has to resolve to one
-- payment immediately.
CREATE INDEX ON payment_transactions (trace_code);

-- Reporting reads paid purchases by date; without this it is a sequential scan
-- over every purchase ever made, which is the query that gets slower every day
-- the product succeeds.
CREATE INDEX ON purchases (paid_at DESC) WHERE status = 'paid';

-- +goose Down
DROP INDEX IF EXISTS purchases_paid_at_idx;
DROP INDEX IF EXISTS payment_transactions_trace_code_idx;
ALTER TABLE payment_transactions DROP COLUMN IF EXISTS trace_code;
