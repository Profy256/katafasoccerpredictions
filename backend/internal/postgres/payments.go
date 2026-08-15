package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// PurchaseIntent is the pair of rows written before the outbound call.
type PurchaseIntent struct {
	PurchaseID    uuid.UUID
	TransactionID uuid.UUID
	Reference     uuid.UUID
}

// CreatePurchaseIntent writes the purchase and its first transaction.
//
// Both rows commit *before* the gateway is called. If the process dies between
// them, a transaction row exists with status 'initiated' and reconciliation
// will find it. Calling the gateway first and recording after would produce a
// charge with no local record — the one failure mode that costs a user money
// and leaves no evidence.
func (db *DB) CreatePurchaseIntent(
	ctx context.Context, q Querier,
	userID, slipID uuid.UUID, price domain.UGX, phone string, rawRequest []byte,
) (PurchaseIntent, error) {
	var intent PurchaseIntent
	intent.Reference = uuid.New()

	err := q.QueryRow(ctx, `
		INSERT INTO purchases (user_id, slip_id, price_ugx, status)
		VALUES ($1,$2,$3,'pending')
		RETURNING id`, userID, slipID, int64(price)).Scan(&intent.PurchaseID)
	if err != nil {
		return PurchaseIntent{}, fmt.Errorf("insert purchase: %w", err)
	}

	err = q.QueryRow(ctx, `
		INSERT INTO payment_transactions
		  (purchase_id, reference, status, amount_ugx, phone_number, raw_request)
		VALUES ($1,$2,'initiated',$3,$4,$5)
		RETURNING id`,
		intent.PurchaseID, intent.Reference, int64(price), phone, rawRequest).Scan(&intent.TransactionID)
	if err != nil {
		return PurchaseIntent{}, fmt.Errorf("insert payment transaction: %w", err)
	}
	return intent, nil
}

// RecordCollectResponse stores what the gateway said about a collection.
func (db *DB) RecordCollectResponse(
	ctx context.Context, txID uuid.UUID,
	providerUUID, providerTxnID, mobileProvider, status string, raw []byte,
) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE payment_transactions
		SET provider_uuid   = COALESCE(NULLIF($2,''), provider_uuid),
		    provider_txn_id = COALESCE(NULLIF($3,''), provider_txn_id),
		    mobile_provider = COALESCE(NULLIF($4,''), mobile_provider),
		    status          = $5,
		    raw_response    = $6
		WHERE id = $1`, txID, providerUUID, providerTxnID, mobileProvider, status, raw)
	if err != nil {
		return fmt.Errorf("record collect response: %w", err)
	}
	return nil
}

// AlreadyOwns reports whether the user has a paid purchase for the slip.
func (db *DB) AlreadyOwns(ctx context.Context, q Querier, userID, slipID uuid.UUID) (bool, error) {
	var owns bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM purchases
		               WHERE user_id = $1 AND slip_id = $2 AND status = 'paid')`,
		userID, slipID).Scan(&owns)
	if err != nil {
		return false, fmt.Errorf("check ownership: %w", err)
	}
	return owns, nil
}

func (db *DB) Purchase(ctx context.Context, purchaseID, userID uuid.UUID) (domain.Purchase, error) {
	var p domain.Purchase
	var price int64
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, slip_id, price_ugx, status, created_at, paid_at
		FROM purchases WHERE id = $1 AND user_id = $2`, purchaseID, userID).
		Scan(&p.ID, &p.UserID, &p.SlipID, &price, &p.Status, &p.CreatedAt, &p.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Purchase{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Purchase{}, fmt.Errorf("query purchase: %w", err)
	}
	p.PriceUGX = domain.UGX(price)
	return p, nil
}

func (db *DB) PurchasesForUser(ctx context.Context, userID uuid.UUID) ([]domain.Purchase, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, slip_id, price_ugx, status, created_at, paid_at
		FROM purchases WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query purchases: %w", err)
	}
	defer rows.Close()

	out := []domain.Purchase{}
	for rows.Next() {
		var p domain.Purchase
		var price int64
		if err := rows.Scan(&p.ID, &p.UserID, &p.SlipID, &price, &p.Status, &p.CreatedAt, &p.PaidAt); err != nil {
			return nil, err
		}
		p.PriceUGX = domain.UGX(price)
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecordWebhookEvent appends a callback *before* any processing, whatever its
// contents. Every callback that arrives is recorded even if it is malformed,
// unsigned, or about a transaction we do not recognise: when a user says they
// were charged and got nothing, this table is the first place to look and it
// must not have gaps.
func (db *DB) RecordWebhookEvent(
	ctx context.Context, provider, eventType, providerTxnID, reference string,
	payload []byte, signatureValid bool,
) (int64, error) {
	var id int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO payment_webhook_events
		  (provider, event_type, provider_txn_id, reference, payload, signature_valid)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6)
		RETURNING id`,
		provider, eventType, providerTxnID, reference, payload, signatureValid).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("record webhook event: %w", err)
	}
	return id, nil
}

// WebhookEvent is a stored callback awaiting processing.
type WebhookEvent struct {
	ID             int64
	EventType      string
	ProviderTxnID  *string
	Reference      *string
	Payload        []byte
	SignatureValid bool
}

func (db *DB) WebhookEventByID(ctx context.Context, q Querier, id int64) (WebhookEvent, error) {
	var e WebhookEvent
	err := q.QueryRow(ctx, `
		SELECT id, event_type, provider_txn_id, reference, payload, signature_valid
		FROM payment_webhook_events WHERE id = $1`, id).
		Scan(&e.ID, &e.EventType, &e.ProviderTxnID, &e.Reference, &e.Payload, &e.SignatureValid)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookEvent{}, domain.ErrNotFound
	}
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("query webhook event: %w", err)
	}
	return e, nil
}

func (db *DB) MarkWebhookProcessed(ctx context.Context, q Querier, id int64, processErr string) error {
	var e *string
	if processErr != "" {
		e = &processErr
	}
	_, err := q.Exec(ctx,
		`UPDATE payment_webhook_events SET processed_at = now(), process_error = $2 WHERE id = $1`,
		id, e)
	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}
	return nil
}

// PaymentTransaction is one collection attempt.
type PaymentTransaction struct {
	ID            uuid.UUID
	PurchaseID    uuid.UUID
	Reference     uuid.UUID
	ProviderUUID  *string
	ProviderTxnID *string
	Status        string
	AmountUGX     domain.UGX
	PhoneNumber   string
	CreatedAt     time.Time
}

// FindTransaction locates a transaction by the provider's id, falling back to
// our reference.
//
// The callback carries provider_transaction_id and, per the documentation, not
// provider_reference — so the provider id is tried first and the reference is
// the fallback.
func (db *DB) FindTransaction(ctx context.Context, q Querier, providerTxnID, reference string) (PaymentTransaction, error) {
	var t PaymentTransaction
	var amount int64

	err := q.QueryRow(ctx, `
		SELECT id, purchase_id, reference, provider_uuid, provider_txn_id,
		       status, amount_ugx, phone_number, created_at
		FROM payment_transactions
		WHERE ($1 <> '' AND provider_txn_id = $1)
		   OR ($2 <> '' AND reference::text = $2)
		ORDER BY created_at DESC
		LIMIT 1`, providerTxnID, reference).
		Scan(&t.ID, &t.PurchaseID, &t.Reference, &t.ProviderUUID, &t.ProviderTxnID,
			&t.Status, &amount, &t.PhoneNumber, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentTransaction{}, domain.ErrNotFound
	}
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("find transaction: %w", err)
	}
	t.AmountUGX = domain.UGX(amount)
	return t, nil
}

// CompletePurchase marks a transaction completed and grants the entitlement.
//
// The purchase UPDATE is guarded on status = 'pending': zero rows updated
// means it was already processed. That, plus the purchases_one_paid partial
// unique index, makes a double grant structurally impossible rather than
// merely unlikely — callbacks do arrive more than once.
func (db *DB) CompletePurchase(ctx context.Context, q Querier, txID, purchaseID uuid.UUID) (granted bool, err error) {
	if _, err := q.Exec(ctx, `
		UPDATE payment_transactions
		SET status = 'completed', settled_at = now()
		WHERE id = $1 AND status <> 'completed'`, txID); err != nil {
		return false, fmt.Errorf("complete transaction: %w", err)
	}

	tag, err := q.Exec(ctx, `
		UPDATE purchases SET status = 'paid', paid_at = now()
		WHERE id = $1 AND status = 'pending'`, purchaseID)
	if err != nil {
		return false, fmt.Errorf("mark purchase paid: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// FailPurchase records a failed collection.
//
// It refuses to downgrade a purchase that is already paid. Callbacks can
// arrive out of order, and a late 'failed' after a 'completed' must not revoke
// a slip the user paid for.
func (db *DB) FailPurchase(ctx context.Context, q Querier, txID, purchaseID uuid.UUID, status string) error {
	if _, err := q.Exec(ctx, `
		UPDATE payment_transactions SET status = $2, settled_at = now()
		WHERE id = $1 AND status NOT IN ('completed')`, txID, status); err != nil {
		return fmt.Errorf("fail transaction: %w", err)
	}
	if _, err := q.Exec(ctx, `
		UPDATE purchases SET status = 'failed'
		WHERE id = $1 AND status = 'pending'`, purchaseID); err != nil {
		return fmt.Errorf("fail purchase: %w", err)
	}
	return nil
}

// UnreconciledTransactions are non-final collections old enough that a webhook
// should already have arrived.
//
// This job, not the webhook, is what actually guarantees users get what they
// paid for. The webhook is an optimisation that makes the common case fast.
func (db *DB) UnreconciledTransactions(ctx context.Context, limit int) ([]PaymentTransaction, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, purchase_id, reference, provider_uuid, provider_txn_id,
		       status, amount_ugx, phone_number, created_at
		FROM payment_transactions
		WHERE status IN ('initiated','processing','pending')
		  AND created_at < now() - interval '3 minutes'
		  AND created_at > now() - interval '7 days'
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query unreconciled transactions: %w", err)
	}
	defer rows.Close()

	var out []PaymentTransaction
	for rows.Next() {
		var t PaymentTransaction
		var amount int64
		if err := rows.Scan(&t.ID, &t.PurchaseID, &t.Reference, &t.ProviderUUID, &t.ProviderTxnID,
			&t.Status, &amount, &t.PhoneNumber, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.AmountUGX = domain.UGX(amount)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ExpireStaleTransactions marks transactions that never reached a final state.
func (db *DB) ExpireStaleTransactions(ctx context.Context) (int64, error) {
	tag, err := db.Pool.Exec(ctx, `
		WITH stale AS (
		    UPDATE payment_transactions
		    SET status = 'expired', settled_at = now()
		    WHERE status IN ('initiated','processing','pending')
		      AND created_at < now() - interval '24 hours'
		    RETURNING purchase_id
		)
		UPDATE purchases SET status = 'failed'
		WHERE id IN (SELECT purchase_id FROM stale) AND status = 'pending'`)
	if err != nil {
		return 0, fmt.Errorf("expire stale transactions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RefundPurchase marks a purchase refunded and records the refund.
//
// A refunded purchase revokes access on its own: the entitlement query checks
// status = 'paid', so nothing further is needed. The purchase row and the slip
// both stay — the history does not get deleted.
func (db *DB) RefundPurchase(ctx context.Context, q Querier, purchaseID uuid.UUID, amount domain.UGX, reason string) error {
	if _, err := q.Exec(ctx, `
		INSERT INTO refunds (purchase_id, amount_ugx, reason)
		VALUES ($1,$2,$3)
		ON CONFLICT (purchase_id) DO NOTHING`, purchaseID, int64(amount), reason); err != nil {
		return fmt.Errorf("insert refund: %w", err)
	}
	if _, err := q.Exec(ctx, `
		UPDATE purchases SET status = 'refunded' WHERE id = $1 AND status = 'paid'`,
		purchaseID); err != nil {
		return fmt.Errorf("mark purchase refunded: %w", err)
	}
	return nil
}

// PaidPurchasesForSlip lists entitlements to refund when a slip voids.
func (db *DB) PaidPurchasesForSlip(ctx context.Context, q Querier, slipID uuid.UUID) ([]domain.Purchase, error) {
	rows, err := q.Query(ctx, `
		SELECT id, user_id, slip_id, price_ugx, status, created_at, paid_at
		FROM purchases WHERE slip_id = $1 AND status = 'paid'`, slipID)
	if err != nil {
		return nil, fmt.Errorf("query paid purchases: %w", err)
	}
	defer rows.Close()

	var out []domain.Purchase
	for rows.Next() {
		var p domain.Purchase
		var price int64
		if err := rows.Scan(&p.ID, &p.UserID, &p.SlipID, &price, &p.Status, &p.CreatedAt, &p.PaidAt); err != nil {
			return nil, err
		}
		p.PriceUGX = domain.UGX(price)
		out = append(out, p)
	}
	return out, rows.Err()
}
