package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
)

// buyer arranges a slip, a purchase and a payment transaction, and returns the
// reference the collection was sent under.
func buyer(t *testing.T, db *postgres.DB, f fixture, price int64, paid bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var slipID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count,
		                   created_by, status, published_at)
		VALUES ('vip','Saturday Banker',$1,4.250,3,$2,'open',now())
		RETURNING id`, price, f.userID).Scan(&slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}

	var purchaseID uuid.UUID
	status, paidAt := "pending", any(nil)
	if paid {
		status, paidAt = "paid", any(time.Now().UTC())
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO purchases (user_id, slip_id, price_ugx, status, paid_at)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		f.userID, slipID, price, status, paidAt).Scan(&purchaseID); err != nil {
		t.Fatalf("insert purchase: %v", err)
	}

	reference := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO payment_transactions
		  (purchase_id, reference, status, amount_ugx, phone_number, mobile_provider, raw_request)
		VALUES ($1,$2,$3,$4,'+256771234567','mtn','{}'::jsonb)`,
		purchaseID, reference, map[bool]string{true: "completed", false: "processing"}[paid], price); err != nil {
		t.Fatalf("insert payment transaction: %v", err)
	}
	return reference
}

// The trace code is computed in Go for the description sent to MarzPay, and by
// Postgres for the column that is searched. If those two ever disagree, every
// code read off a statement stops resolving — silently, and only for payments
// made after the divergence.
func TestStoredTraceCodeMatchesTheOneSentToMarzPay(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)

	for i := 0; i < 25; i++ {
		reference := buyer(t, db, f, 5000, true)

		var stored string
		if err := db.Pool.QueryRow(context.Background(),
			`SELECT trace_code FROM payment_transactions WHERE reference = $1`,
			reference).Scan(&stored); err != nil {
			t.Fatalf("read trace_code: %v", err)
		}

		if want := pay.TraceCode(reference); stored != want {
			t.Fatalf("reference %s: Postgres stored %q, Go sends %q", reference, stored, want)
		}
	}
}

func TestPaymentByTraceCodeResolvesAStatementLine(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	reference := buyer(t, db, f, 5000, true)
	code := pay.TraceCode(reference)

	trace, err := db.PaymentByTraceCode(ctx, code)
	if err != nil {
		t.Fatalf("PaymentByTraceCode: %v", err)
	}

	if trace.AmountUGX != 5000 {
		t.Errorf("amount = %d, want 5000", int64(trace.AmountUGX))
	}
	if trace.SlipTitle != "Saturday Banker" {
		t.Errorf("slip title = %q", trace.SlipTitle)
	}
	if trace.PackageCode != domain.PackageVIP {
		t.Errorf("package = %q, want vip", trace.PackageCode)
	}
	if trace.UserEmail != "admin@katafa.test" {
		t.Errorf("buyer email = %q", trace.UserEmail)
	}
	if trace.PurchaseStatus != "paid" {
		t.Errorf("purchase status = %q, want paid", trace.PurchaseStatus)
	}

	// The code will be retyped by a human off a screen, so it has to resolve
	// however they type it.
	for _, variant := range []string{
		code, strings.ToLower(code), strings.TrimPrefix(code, "KTF-"), " " + code + " ",
	} {
		if _, err := db.PaymentByTraceCode(ctx, variant); err != nil {
			t.Errorf("trace code %q did not resolve: %v", variant, err)
		}
	}

	if _, err := db.PaymentByTraceCode(ctx, "KTF-00000000"); err == nil {
		t.Error("an unknown trace code resolved to something")
	}
}

// Revenue must count money that arrived, and must not quietly present pending
// or failed attempts as income.
func TestRevenueCountsOnlyMoneyThatArrived(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	buyer(t, db, f, 5000, true)
	buyer(t, db, f, 2000, true)
	buyer(t, db, f, 20000, false) // still in flight

	from := time.Now().UTC().AddDate(0, 0, -1)
	to := time.Now().UTC().AddDate(0, 0, 1)

	report, err := db.Revenue(ctx, from, to)
	if err != nil {
		t.Fatalf("Revenue: %v", err)
	}

	if report.GrossUGX != 7000 {
		t.Errorf("gross = %d, want 7000 (the pending 20000 is not revenue)", int64(report.GrossUGX))
	}
	if report.PaidPurchases != 2 {
		t.Errorf("paid purchases = %d, want 2", report.PaidPurchases)
	}
	if report.PendingUGX != 20000 || report.PendingPurchases != 1 {
		t.Errorf("pending = %d over %d purchases, want 20000 over 1",
			int64(report.PendingUGX), report.PendingPurchases)
	}
	if report.NetUGX != 7000 {
		t.Errorf("net = %d, want 7000", int64(report.NetUGX))
	}

	// The breakdown has to add up to the headline, or the report is worse than
	// no report: it looks authoritative and is not.
	var byPackage domain.UGX
	for _, bucket := range report.ByPackage {
		byPackage += bucket.GrossUGX
	}
	if byPackage != report.GrossUGX {
		t.Errorf("by-package total %d does not match gross %d",
			int64(byPackage), int64(report.GrossUGX))
	}
}

// A refund reduces net but must remain visible as a refund. A period with
// heavy refunds and one with few sales look identical once netted, and mean
// very different things.
func TestRefundsAreReportedSeparatelyFromSales(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	buyer(t, db, f, 5000, true)
	reference := buyer(t, db, f, 5000, true)

	var purchaseID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		SELECT purchase_id FROM payment_transactions WHERE reference = $1`,
		reference).Scan(&purchaseID); err != nil {
		t.Fatalf("find purchase: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE purchases SET status = 'refunded' WHERE id = $1`, purchaseID); err != nil {
		t.Fatalf("refund: %v", err)
	}

	report, err := db.Revenue(ctx, time.Now().UTC().AddDate(0, 0, -1), time.Now().UTC().AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Revenue: %v", err)
	}

	if report.GrossUGX != 5000 {
		t.Errorf("gross = %d, want 5000", int64(report.GrossUGX))
	}
	if report.RefundedUGX != 5000 {
		t.Errorf("refunded = %d, want 5000 reported separately", int64(report.RefundedUGX))
	}
	if report.NetUGX != 0 {
		t.Errorf("net = %d, want 0", int64(report.NetUGX))
	}
}

// The ledger is what a MarzPay statement is reconciled against, so it lists
// every attempt — a line missing because it failed is a line nobody can
// explain.
func TestPaymentLedgerIncludesFailedAttempts(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)

	buyer(t, db, f, 5000, true)
	buyer(t, db, f, 20000, false)

	rows, err := db.PaymentLedger(context.Background(),
		time.Now().UTC().AddDate(0, 0, -1), time.Now().UTC().AddDate(0, 0, 1), 100)
	if err != nil {
		t.Fatalf("PaymentLedger: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ledger has %d rows, want 2 including the unresolved one", len(rows))
	}
	for _, row := range rows {
		if row.TraceCode == "" {
			t.Error("ledger row has no trace code to match against a statement")
		}
		if row.SlipTitle == "" || row.PackageCode == "" {
			t.Error("ledger row cannot be attributed to a product")
		}
	}
}
