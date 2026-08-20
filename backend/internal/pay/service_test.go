package pay_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
)

// ROADMAP phase 7's "done when": a lost webhook still results in a completed
// purchase, proven by a test that never delivers one.
//
// The webhook is an optimisation that makes the common case fast.
// Reconciliation is what actually guarantees users get what they paid for, and
// it is the path that is never exercised in normal operation — so it is the
// one most likely to be quietly broken. Every test in this file goes through
// pay.Service against real Postgres, because the guarantees being asserted
// (guarded UPDATEs, the one-paid-purchase partial index) live in the database.

type payFixture struct {
	db      *postgres.DB
	svc     *pay.Service
	fake    *pay.Fake
	userID  uuid.UUID
	adminID uuid.UUID
	slipID  uuid.UUID
	price   domain.UGX
}

const testPhone = "+256772123456"

func newPayFixture(t *testing.T) payFixture {
	t.Helper()
	ctx := context.Background()

	db := testdb.New(t)
	fake := pay.NewFake()

	f := payFixture{
		db:    db,
		fake:  fake,
		price: domain.UGX(5000),
		svc: &pay.Service{
			DB:          db,
			Provider:    fake,
			Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
			CallbackURL: "https://katafa.test/v1/webhooks/marzpay",
			Environment: "test",
		},
	}

	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ('admin@katafa.test','x','Admin','admin') RETURNING id`).Scan(&f.adminID); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ('buyer@katafa.test','x','Buyer','user') RETURNING id`).Scan(&f.userID); err != nil {
		t.Fatalf("insert buyer: %v", err)
	}

	var analystID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO analysts (slug, name, handle, initials, joined_at)
		VALUES ('okello','Okello','@okello','O',now()) RETURNING id`).Scan(&analystID); err != nil {
		t.Fatalf("insert analyst: %v", err)
	}

	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count,
		                   created_by, status, published_at)
		VALUES ('vip','Saturday Banker',$1,4.250,1,$2,'open',now()) RETURNING id`,
		int64(f.price), f.adminID).Scan(&f.slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tips (slip_id, analyst_id, fixture_label, market_label, selection_label,
		                  odds, kickoff_at, position)
		VALUES ($1,$2,'Home FC v Away FC','Match Result','Home Win',4.250,
		        now() + interval '6 hours',1)`, f.slipID, analystID); err != nil {
		t.Fatalf("insert tip: %v", err)
	}

	return f
}

// purchaseStatus reads the entitlement as the paywall sees it.
func (f payFixture) purchaseStatus(t *testing.T, purchaseID uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.db.Pool.QueryRow(context.Background(),
		`SELECT status FROM purchases WHERE id = $1`, purchaseID).Scan(&status); err != nil {
		t.Fatalf("read purchase status: %v", err)
	}
	return status
}

// tipsVisible answers the only question that matters to the buyer: can they
// see what they paid for? Read through the real entitlement query rather than
// by inspecting the purchase row, so the test fails if the two ever disagree.
func (f payFixture) tipsVisible(t *testing.T) int {
	t.Helper()
	slip, err := f.db.Slip(context.Background(), f.slipID, f.userID, false)
	if err != nil {
		t.Fatalf("read slip: %v", err)
	}
	return len(slip.Tips)
}

// age backdates a transaction so UnreconciledTransactions will pick it up.
//
// Reconciliation deliberately ignores anything younger than three minutes, to
// give the webhook a chance to arrive first. Waiting that out in a test would
// be three minutes of nothing, so the clock is moved instead of the test.
func (f payFixture) age(t *testing.T, d time.Duration) {
	t.Helper()
	testdb.MustExec(t, f.db,
		`UPDATE payment_transactions SET created_at = created_at - $1::interval`,
		d.String())
}

func (f payFixture) transactionFor(t *testing.T, purchaseID uuid.UUID) postgres.PaymentTransaction {
	t.Helper()
	var providerUUID *string
	var status string
	if err := f.db.Pool.QueryRow(context.Background(), `
		SELECT provider_uuid, status FROM payment_transactions
		WHERE purchase_id = $1 ORDER BY created_at DESC LIMIT 1`, purchaseID).
		Scan(&providerUUID, &status); err != nil {
		t.Fatalf("read transaction: %v", err)
	}
	txn := postgres.PaymentTransaction{PurchaseID: purchaseID, Status: status}
	txn.ProviderUUID = providerUUID
	return txn
}

// The headline test. No webhook is ever constructed, let alone delivered.
func TestLostWebhookStillCompletesThePurchase(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if got := f.purchaseStatus(t, result.PurchaseID); got != "pending" {
		t.Fatalf("purchase status after initiation = %q, want pending", got)
	}
	if n := f.tipsVisible(t); n != 0 {
		t.Fatalf("tips were visible before payment completed: %d rows", n)
	}

	// The payer approves the prompt on their handset. The gateway's callback
	// to us is lost — a dropped connection, a deploy, an expired certificate.
	// Nothing tells this system that the money arrived.
	txn := f.transactionFor(t, result.PurchaseID)
	if txn.ProviderUUID == nil {
		t.Fatal("no provider uuid recorded; reconciliation would have nothing to poll")
	}
	f.fake.SetStatus(*txn.ProviderUUID, pay.TxCompleted)

	f.age(t, 10*time.Minute)

	resolved, err := f.svc.Reconcile(ctx, 100)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if resolved != 1 {
		t.Errorf("reconcile resolved %d transactions, want 1", resolved)
	}

	if got := f.purchaseStatus(t, result.PurchaseID); got != "paid" {
		t.Fatalf("purchase status after reconciliation = %q, want paid — "+
			"a user paid and never received the slip", got)
	}
	if n := f.tipsVisible(t); n != 1 {
		t.Errorf("buyer sees %d tip rows after reconciliation, want 1", n)
	}

	// The grant is auditable without the webhook table having anything in it.
	var events int
	if err := f.db.Pool.QueryRow(ctx, `SELECT count(*) FROM payment_webhook_events`).Scan(&events); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	if events != 0 {
		t.Errorf("test delivered %d webhook events; it is supposed to deliver none", events)
	}

	var audits int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'purchase.paid' AND entity_id = $1`,
		result.PurchaseID).Scan(&audits); err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if audits != 1 {
		t.Errorf("audit_log holds %d purchase.paid entries, want 1", audits)
	}
}

// Reconciliation runs every few minutes forever. It must be safe to run again
// over a transaction it already settled.
func TestReconcileIsIdempotent(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)
	f.fake.SetStatus(*txn.ProviderUUID, pay.TxCompleted)
	f.age(t, 10*time.Minute)

	for i := range 3 {
		if _, err := f.svc.Reconcile(ctx, 100); err != nil {
			t.Fatalf("reconcile pass %d: %v", i+1, err)
		}
	}

	var paid int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM purchases WHERE id = $1 AND status = 'paid'`,
		result.PurchaseID).Scan(&paid); err != nil {
		t.Fatalf("count paid: %v", err)
	}
	if paid != 1 {
		t.Errorf("%d paid purchases after three reconciliation passes, want 1", paid)
	}

	var audits int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'purchase.paid' AND entity_id = $1`,
		result.PurchaseID).Scan(&audits); err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if audits != 1 {
		t.Errorf("repeated reconciliation wrote %d purchase.paid audit entries, want 1", audits)
	}
}

// The webhook arriving *and* reconciliation running is the normal case, not an
// edge case: the job does not know the callback already landed.
func TestWebhookAndReconciliationDoNotDoubleGrant(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)
	f.fake.SetStatus(*txn.ProviderUUID, pay.TxCompleted)

	f.deliverWebhook(t, *txn.ProviderUUID, "successful", true)

	if got := f.purchaseStatus(t, result.PurchaseID); got != "paid" {
		t.Fatalf("purchase status after webhook = %q, want paid", got)
	}

	f.age(t, 10*time.Minute)
	if _, err := f.svc.Reconcile(ctx, 100); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var audits int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'purchase.paid' AND entity_id = $1`,
		result.PurchaseID).Scan(&audits); err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if audits != 1 {
		t.Errorf("webhook plus reconciliation granted %d times, want 1", audits)
	}
	if n := f.tipsVisible(t); n != 1 {
		t.Errorf("buyer sees %d tip rows, want 1", n)
	}
}

// deliverWebhook records and processes a callback the way the HTTP handler
// does, without going through HTTP. `valid` scripts whether the signature
// checks out.
func (f payFixture) deliverWebhook(t *testing.T, providerTxnID, status string, valid bool) int64 {
	t.Helper()
	ctx := context.Background()

	body, err := json.Marshal(map[string]any{
		"event_type": "collection.completed",
		"data": map[string]any{
			"provider_transaction_id": "fake-txn-" + providerTxnID[:8],
			"status":                  status,
		},
	})
	if err != nil {
		t.Fatalf("encode webhook body: %v", err)
	}

	signature := f.fake.Sign(body)
	if !valid {
		signature = "sha256=deadbeef"
	}
	signatureValid := f.fake.VerifyWebhook(body, signature)
	if signatureValid != valid {
		t.Fatalf("fixture produced signatureValid=%v, want %v", signatureValid, valid)
	}

	eventType, providerTxn, reference := pay.ParseWebhook(body)
	eventID, err := f.db.RecordWebhookEvent(ctx, "marzpay", eventType, providerTxn, reference, body, signatureValid)
	if err != nil {
		t.Fatalf("record webhook: %v", err)
	}
	if signatureValid {
		if err := f.svc.ProcessWebhookEvent(ctx, eventID); err != nil {
			t.Fatalf("process webhook: %v", err)
		}
	}
	return eventID
}

// Callbacks do arrive more than once. A duplicate must not create a second
// entitlement, and must not error.
func TestDuplicateWebhookGrantsOnce(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)

	for range 3 {
		f.deliverWebhook(t, *txn.ProviderUUID, "successful", true)
	}

	var audits int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'purchase.paid' AND entity_id = $1`,
		result.PurchaseID).Scan(&audits); err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if audits != 1 {
		t.Errorf("three identical callbacks granted %d times, want 1", audits)
	}
}

// A forged callback is recorded and never acted on. Recording it is the point:
// a flood of forgeries is something you want to be able to see afterwards.
func TestInvalidSignatureIsRecordedButNeverActedOn(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)

	eventID := f.deliverWebhook(t, *txn.ProviderUUID, "successful", false)

	// The handler records but does not process an invalid signature. Processing
	// it explicitly must still refuse — the check is in the service, not only
	// in the handler.
	if err := f.svc.ProcessWebhookEvent(ctx, eventID); err != nil {
		t.Fatalf("processing a forged event should be a no-op, not an error: %v", err)
	}

	if got := f.purchaseStatus(t, result.PurchaseID); got != "pending" {
		t.Errorf("a forged callback moved the purchase to %q", got)
	}
	if n := f.tipsVisible(t); n != 0 {
		t.Errorf("a forged callback unlocked %d tip rows", n)
	}

	var recorded bool
	var processError *string
	if err := f.db.Pool.QueryRow(ctx, `
		SELECT signature_valid, process_error FROM payment_webhook_events WHERE id = $1`,
		eventID).Scan(&recorded, &processError); err != nil {
		t.Fatalf("read webhook event: %v", err)
	}
	if recorded {
		t.Error("forged callback was recorded as validly signed")
	}
	if processError == nil || *processError != "signature invalid" {
		t.Errorf("process_error = %v, want \"signature invalid\"", processError)
	}
}

// Callbacks can arrive out of order. A late 'failed' after a 'completed' must
// not revoke a slip the user paid for.
func TestLateFailureDoesNotRevokeAPaidPurchase(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)

	f.deliverWebhook(t, *txn.ProviderUUID, "successful", true)
	if got := f.purchaseStatus(t, result.PurchaseID); got != "paid" {
		t.Fatalf("purchase status = %q, want paid", got)
	}

	f.deliverWebhook(t, *txn.ProviderUUID, "failed", true)

	if got := f.purchaseStatus(t, result.PurchaseID); got != "paid" {
		t.Errorf("a late failure callback downgraded a paid purchase to %q", got)
	}
	if n := f.tipsVisible(t); n != 1 {
		t.Errorf("buyer sees %d tip rows after a late failure callback, want 1", n)
	}
}

// If the outbound call never lands, the transaction row still exists and is
// recoverable. This is the failure mode that would otherwise charge a user
// with no local record — the ordering in CreatePurchaseIntent exists for it.
func TestFailedCollectLeavesARecoverableRecord(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	f.fake.FailCollect = errors.New("connection reset by peer")

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("a failed outbound call must not fail the purchase: %v", err)
	}

	var status string
	var providerUUID *string
	if err := f.db.Pool.QueryRow(ctx, `
		SELECT status, provider_uuid FROM payment_transactions WHERE purchase_id = $1`,
		result.PurchaseID).Scan(&status, &providerUUID); err != nil {
		t.Fatalf("read transaction: %v", err)
	}
	if status != "initiated" {
		t.Errorf("transaction status = %q, want initiated", status)
	}
	if providerUUID != nil {
		t.Errorf("provider uuid = %v, want none — the gateway was never reached", *providerUUID)
	}

	// Nothing to poll, so reconciliation leaves it alone rather than guessing.
	f.age(t, 10*time.Minute)
	resolved, err := f.svc.Reconcile(ctx, 100)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if resolved != 0 {
		t.Errorf("reconcile claimed to resolve %d transactions it could not poll", resolved)
	}
	if got := f.purchaseStatus(t, result.PurchaseID); got != "pending" {
		t.Errorf("purchase status = %q, want pending", got)
	}

	// After 24 hours it is closed out rather than left pending forever.
	f.age(t, 25*time.Hour)
	if _, err := f.svc.Reconcile(ctx, 100); err != nil {
		t.Fatalf("reconcile after expiry window: %v", err)
	}
	if got := f.purchaseStatus(t, result.PurchaseID); got != "failed" {
		t.Errorf("stale purchase status = %q, want failed", got)
	}
	if n := f.tipsVisible(t); n != 0 {
		t.Errorf("an expired purchase unlocked %d tip rows", n)
	}
}

// A refund revokes the entitlement without deleting the history.
func TestRefundRevokesTheEntitlement(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)
	f.deliverWebhook(t, *txn.ProviderUUID, "successful", true)
	if n := f.tipsVisible(t); n != 1 {
		t.Fatalf("buyer sees %d tip rows before the refund, want 1", n)
	}

	if err := f.svc.RefundSlip(ctx, f.slipID, "every leg voided"); err != nil {
		t.Fatalf("refund slip: %v", err)
	}

	if got := f.purchaseStatus(t, result.PurchaseID); got != "refunded" {
		t.Errorf("purchase status = %q, want refunded", got)
	}
	if n := f.tipsVisible(t); n != 0 {
		t.Errorf("a refunded buyer still sees %d tip rows", n)
	}

	var amount int64
	var reason string
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT amount_ugx, reason FROM refunds WHERE purchase_id = $1`,
		result.PurchaseID).Scan(&amount, &reason); err != nil {
		t.Fatalf("read refund: %v", err)
	}
	if amount != int64(f.price) {
		t.Errorf("refunded %d shillings, want %d", amount, int64(f.price))
	}

	// Refunding twice must not write a second refund row: a slip is one
	// indivisible product and partial refunds do not exist.
	if err := f.svc.RefundSlip(ctx, f.slipID, "retry"); err != nil {
		t.Fatalf("second refund pass: %v", err)
	}
	var refunds int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM refunds WHERE purchase_id = $1`, result.PurchaseID).Scan(&refunds); err != nil {
		t.Fatalf("count refunds: %v", err)
	}
	if refunds != 1 {
		t.Errorf("%d refund rows for one purchase, want 1", refunds)
	}
}

// Buying the same slip twice is refused before any money moves.
func TestSecondPurchaseOfTheSameSlipIsRefused(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	txn := f.transactionFor(t, result.PurchaseID)
	f.deliverWebhook(t, *txn.ProviderUUID, "successful", true)

	collectsBefore := len(f.fake.Collected)
	_, err = f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second purchase returned %v, want ErrConflict", err)
	}
	if len(f.fake.Collected) != collectsBefore {
		t.Error("a refused purchase still prompted the payer for money")
	}
}

// The trace code returned to the caller must match what Postgres generated, or
// support cannot find a payment from the code the user reads off their SMS.
func TestReturnedTraceCodeMatchesTheStoredOne(t *testing.T) {
	f := newPayFixture(t)
	ctx := context.Background()

	result, err := f.svc.Purchase(ctx, f.userID, f.slipID, testPhone)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}

	var stored string
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT trace_code FROM payment_transactions WHERE purchase_id = $1`,
		result.PurchaseID).Scan(&stored); err != nil {
		t.Fatalf("read trace code: %v", err)
	}
	if stored != result.TraceCode {
		t.Errorf("Postgres generated %q but the API returned %q", stored, result.TraceCode)
	}
}
