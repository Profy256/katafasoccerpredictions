-- +goose Up

-- DATA-MODEL.md gives purchases CHECK ((status = 'paid') = (paid_at IS NOT NULL)),
-- and PAYMENTS.md § Refunds sets status = 'refunded' on a paid purchase. Those
-- two rules contradict each other: a refunded row still carries the paid_at it
-- was given when the money arrived, so the check fails and the refund errors.
--
-- The fix is not to clear paid_at. When the money arrived is part of the
-- record, and a refund does not un-happen the payment — "refunds do not delete
-- the purchase row or the slip; the history stays". Nulling it to satisfy a
-- constraint would destroy exactly the history the constraint exists to
-- protect.
--
-- So the invariant becomes: paid_at is set if and only if the purchase was
-- ever paid, which is true of both 'paid' and 'refunded' and of neither
-- 'pending' nor 'failed'.

ALTER TABLE purchases DROP CONSTRAINT purchases_check;

ALTER TABLE purchases
    ADD CONSTRAINT purchases_paid_at_matches_status
    CHECK ((status IN ('paid','refunded')) = (paid_at IS NOT NULL));

-- +goose Down
ALTER TABLE purchases DROP CONSTRAINT purchases_paid_at_matches_status;
ALTER TABLE purchases
    ADD CONSTRAINT purchases_check
    CHECK ((status = 'paid') = (paid_at IS NOT NULL));
