# Payments

MarzPay mobile money collection, UGX, one slip per purchase. No subscriptions.

Base URL `https://wallet.wearemarz.com/api/v1`, HTTP Basic auth with the API
user and key. Verified against the [collections
documentation](https://wallet.wearemarz.com/documentation/collections).

## UGX is zero-decimal

Ugandan shillings have no minor unit in practice. Money is `int64` and `BIGINT`
throughout — never float, never "cents", never a `Money` type that secretly
multiplies by 100. MarzPay's `amount` field takes whole shillings and enforces
500 – 10,000,000 per collection, so validate that range before calling out.

```go
type UGX int64  // whole shillings
```

## Tables

```sql
CREATE TABLE payment_transactions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_id   UUID NOT NULL REFERENCES purchases(id),
    provider      TEXT NOT NULL DEFAULT 'marzpay',
    reference     UUID NOT NULL UNIQUE,     -- our idempotency key, sent as `reference`
    provider_uuid TEXT,                     -- MarzPay transaction uuid
    provider_txn_id TEXT,                   -- provider_transaction_id from the callback
    status        TEXT NOT NULL DEFAULT 'initiated'
                  CHECK (status IN ('initiated','processing','pending',
                                    'completed','failed','expired')),
    amount_ugx    BIGINT NOT NULL CHECK (amount_ugx BETWEEN 500 AND 10000000),
    phone_number  TEXT NOT NULL,
    mobile_provider TEXT,                   -- mtn | airtel
    raw_request   JSONB NOT NULL,
    raw_response  JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at    TIMESTAMPTZ
);
CREATE INDEX ON payment_transactions (status, created_at)
    WHERE status IN ('initiated','processing','pending');
CREATE INDEX ON payment_transactions (provider_txn_id);

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
```

`payment_webhook_events` is append-only and written **before** any processing.
Every callback that arrives is recorded even if it is malformed, unsigned, or
about a transaction we do not recognise. When a user says they were charged and
got nothing, this table is the first place to look, and it must not have gaps.

## Purchase flow

```
POST /v1/slips/{id}/purchase  { phone_number }
  │
  ├─ 1. validate: slip is 'open', not already owned, price in MarzPay's range
  ├─ 2. INSERT purchases (status='pending', price_ugx = slips.price_ugx)
  ├─ 3. INSERT payment_transactions (status='initiated', reference = uuid_v4())
  │     ── both in ONE transaction, committed before the outbound call ──
  ├─ 4. POST /collect-money
  │        { amount, country: "UG", reference, phone_number,
  │          method: "mobile_money", description,
  │          callback_url: PUBLIC_BASE_URL + "/v1/webhooks/marzpay",
  │          metadata: [{purchase_id}, {slip_id}] }
  ├─ 5. UPDATE payment_transactions SET provider_uuid, status, raw_response
  └─ 6. 202 { purchase_id, status: "processing" }
```

Step 3 commits **before** step 4. If the process dies between them, a
`payment_transactions` row exists with `status='initiated'` and reconciliation
will find it. Calling MarzPay first and recording after would produce a charge
with no local record — the one failure mode that costs a user money and leaves
no evidence.

The user's phone then gets a mobile money prompt. The API returns immediately;
the frontend polls `GET /v1/purchases/{id}` or waits for the webhook to land.

`reference` is a UUIDv4 and MarzPay enforces uniqueness on it, which makes it
the idempotency key. A retried purchase attempt generates a *new* reference and
a new `payment_transactions` row against the same `purchase`; reusing a
reference is rejected by the provider, which is the desired behaviour.

## Webhook

`POST /v1/webhooks/marzpay`, public, no session.

```
1. read body, verify signature against MARZPAY_WEBHOOK_SECRET
2. INSERT payment_webhook_events (signature_valid = result)   ← always
3. if !signature_valid → 200, stop.   (200, not 401: never let an
   attacker distinguish a rejected forgery from an accepted one, and never
   make the provider retry a forgery)
4. enqueue a River job, return 200 immediately
5. job: match on collection.provider_transaction_id, else reference
        completed → mark txn completed, purchase paid, paid_at = now()
        failed    → mark txn failed, purchase failed
        unknown   → leave unprocessed, alert
```

Handled asynchronously because the provider's retry policy is not ours to
depend on — a slow entitlement write must not cause a timeout that triggers
duplicate callbacks.

Processing is idempotent. Callbacks arrive more than once; a duplicate
`collection.completed` must not create a second entitlement. Guard with:

```sql
UPDATE purchases SET status = 'paid', paid_at = now()
WHERE id = $1 AND status = 'pending';
```

Zero rows updated means it was already processed. That, plus the
`purchases_one_paid` partial unique index, makes double-grant structurally
impossible rather than merely unlikely.

The callback fires only on final status (`collection.completed` /
`collection.failed`), and per the docs `provider_reference` is absent from
callbacks — match on `provider_transaction_id`, falling back to `reference`.

## Reconciliation

Webhooks get lost. `reconcile_payments` runs every 15 minutes:

```sql
SELECT * FROM payment_transactions
WHERE status IN ('initiated','processing','pending')
  AND created_at < now() - interval '3 minutes'
  AND created_at > now() - interval '7 days';
```

For each, `GET /collect-money/{uuid}` and apply the same state transition the
webhook would have. Transactions still non-final after 24 hours are marked
`expired` and their purchase `failed`.

This job — not the webhook — is what actually guarantees users get what they
paid for. Treat the webhook as an optimisation that makes the common case fast.

## Entitlement

The paywall is a SQL boundary. `getSlip` must not return tips to a user who has
not paid, and must not return them *and hide them*.

```sql
SELECT t.*
FROM tips t
JOIN slips s ON s.id = t.slip_id
WHERE t.slip_id = $1
  AND (
        s.status = 'settled'                    -- settled slips are public
     OR EXISTS (SELECT 1 FROM purchases p
                WHERE p.slip_id = s.id
                  AND p.user_id = $2
                  AND p.status  = 'paid')
      )
ORDER BY t.position;
```

An unpaid viewer gets zero rows from the database. There is no filtering step in
Go that could be forgotten, and no serialisation path where the tips exist in
memory next to a boolean.

Settled slips are deliberately public — non-negotiable 7. Making an analyst's
losing slips visible is what turns their record into evidence.

## Refunds

Two cases:

- **Every tip voided** (all matches postponed or cancelled): the slip goes
  `void`, and every paid purchase is refunded automatically.
- **Admin-initiated**, for a mis-published slip.

Both set `purchases.status = 'refunded'`, write an `audit_log` entry, and
enqueue a MarzPay disbursement. Refunds do not delete the purchase row or the
slip; the history stays.

A refunded purchase revokes access — the entitlement query checks
`status = 'paid'`, so nothing further is needed.

Partial refunds are not supported. A slip is one indivisible product.

## Testing

- The MarzPay client is behind `pay.PaymentProvider`. Tests use a fake; there is
  no code path where a test can reach the live API.
- The webhook handler is tested with: valid, invalid signature, duplicate
  delivery, unknown transaction, and out-of-order (`failed` after `completed`).
  The last must not downgrade a paid purchase.
- Reconciliation is tested against a fake that "loses" the webhook entirely —
  the purchase must still complete.
