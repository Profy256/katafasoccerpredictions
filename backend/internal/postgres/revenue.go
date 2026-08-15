package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Answering "where is this money from?" and "did we receive what we think we
// received?" from the database rather than from the gateway's dashboard.
//
// Every amount here is int64 UGX. Sums stay in BIGINT and never touch a float,
// because a reconciliation report that is approximately right is worse than no
// report — it looks authoritative.

// PaymentTrace is the full attribution of one collection.
type PaymentTrace struct {
	TraceCode     string     `json:"traceCode"`
	Reference     uuid.UUID  `json:"reference"`
	ProviderUUID  *string    `json:"providerUuid,omitempty"`
	ProviderTxnID *string    `json:"providerTxnId,omitempty"`
	Status        string     `json:"status"`
	AmountUGX     domain.UGX `json:"amountUgx"`
	// PhoneNumber is the payer's number, which is the other thing support is
	// given when somebody says they paid.
	PhoneNumber    string     `json:"phoneNumber"`
	MobileProvider *string    `json:"mobileProvider,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	SettledAt      *time.Time `json:"settledAt,omitempty"`

	PurchaseID     uuid.UUID  `json:"purchaseId"`
	PurchaseStatus string     `json:"purchaseStatus"`
	PaidAt         *time.Time `json:"paidAt,omitempty"`

	SlipID      uuid.UUID          `json:"slipId"`
	SlipTitle   string             `json:"slipTitle"`
	PackageCode domain.PackageCode `json:"packageCode"`

	UserID    uuid.UUID `json:"userId"`
	UserEmail string    `json:"userEmail"`
	UserName  string    `json:"userName"`
}

// PaymentByTraceCode resolves a code read off a MarzPay statement, or quoted
// by a user from their SMS, to everything known about that payment.
//
// Accepts the code with or without the KTF- prefix, and in any case, because
// it will be retyped by a human from a screen.
func (db *DB) PaymentByTraceCode(ctx context.Context, code string) (PaymentTrace, error) {
	normalised := NormaliseTraceCode(code)

	var t PaymentTrace
	var amount int64
	err := db.Pool.QueryRow(ctx, `
		SELECT pt.trace_code, pt.reference, pt.provider_uuid, pt.provider_txn_id,
		       pt.status, pt.amount_ugx, pt.phone_number, pt.mobile_provider,
		       pt.created_at, pt.settled_at,
		       p.id, p.status, p.paid_at,
		       s.id, s.title, s.package_code,
		       u.id, u.email, u.name
		FROM payment_transactions pt
		JOIN purchases p ON p.id = pt.purchase_id
		JOIN slips     s ON s.id = p.slip_id
		JOIN users     u ON u.id = p.user_id
		WHERE pt.trace_code = $1`, normalised).
		Scan(&t.TraceCode, &t.Reference, &t.ProviderUUID, &t.ProviderTxnID,
			&t.Status, &amount, &t.PhoneNumber, &t.MobileProvider,
			&t.CreatedAt, &t.SettledAt,
			&t.PurchaseID, &t.PurchaseStatus, &t.PaidAt,
			&t.SlipID, &t.SlipTitle, &t.PackageCode,
			&t.UserID, &t.UserEmail, &t.UserName)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentTrace{}, domain.ErrNotFound
	}
	if err != nil {
		return PaymentTrace{}, fmt.Errorf("lookup payment by trace code: %w", err)
	}
	t.AmountUGX = domain.UGX(amount)
	return t, nil
}

// NormaliseTraceCode accepts "ktf-3f9a2b7c", "3F9A2B7C", or the full code with
// stray whitespace, and returns the canonical form.
func NormaliseTraceCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.TrimPrefix(code, "KTF-")
	code = strings.TrimPrefix(code, "KTF")
	return "KTF-" + code
}

// RevenueBucket is one grouping of received money.
type RevenueBucket struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Purchases int        `json:"purchases"`
	GrossUGX  domain.UGX `json:"grossUgx"`
}

// RevenueReport is where the money came from over a window.
type RevenueReport struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`

	// Gross is what buyers were charged; Refunded is what went back out; Net
	// is the difference. Refunds are reported separately rather than netted
	// silently, because a period with heavy refunds and one with few sales
	// look identical once netted and mean very different things.
	GrossUGX    domain.UGX `json:"grossUgx"`
	RefundedUGX domain.UGX `json:"refundedUgx"`
	NetUGX      domain.UGX `json:"netUgx"`

	PaidPurchases     int `json:"paidPurchases"`
	RefundedPurchases int `json:"refundedPurchases"`
	// Pending is money the gateway has not resolved either way. A number that
	// stays high means reconciliation is not keeping up.
	PendingPurchases int        `json:"pendingPurchases"`
	PendingUGX       domain.UGX `json:"pendingUgx"`
	// FailedPurchases is the drop-off: prompts sent that never completed.
	FailedPurchases int `json:"failedPurchases"`

	ByPackage  []RevenueBucket `json:"byPackage"`
	ByAnalyst  []RevenueBucket `json:"byAnalyst"`
	BySlip     []RevenueBucket `json:"bySlip"`
	ByDay      []RevenueBucket `json:"byDay"`
	ByProvider []RevenueBucket `json:"byMobileProvider"`
}

// Revenue reports received money over a window, broken down by what produced
// it.
//
// The window is applied to paid_at, not created_at: a purchase started on
// Monday and paid on Tuesday is Tuesday's money, which is what a statement
// will agree with.
func (db *DB) Revenue(ctx context.Context, from, to time.Time) (RevenueReport, error) {
	report := RevenueReport{From: from, To: to}

	var gross, refunded, pending int64
	err := db.Pool.QueryRow(ctx, `
		SELECT
		  COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'paid'), 0),
		  COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'refunded'), 0),
		  COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'pending'), 0),
		  count(*) FILTER (WHERE p.status = 'paid'),
		  count(*) FILTER (WHERE p.status = 'refunded'),
		  count(*) FILTER (WHERE p.status = 'pending'),
		  count(*) FILTER (WHERE p.status = 'failed')
		FROM purchases p
		WHERE COALESCE(p.paid_at, p.created_at) >= $1
		  AND COALESCE(p.paid_at, p.created_at) < $2`, from, to).
		Scan(&gross, &refunded, &pending,
			&report.PaidPurchases, &report.RefundedPurchases,
			&report.PendingPurchases, &report.FailedPurchases)
	if err != nil {
		return RevenueReport{}, fmt.Errorf("revenue totals: %w", err)
	}
	report.GrossUGX = domain.UGX(gross)
	report.RefundedUGX = domain.UGX(refunded)
	report.NetUGX = domain.UGX(gross - refunded)
	report.PendingUGX = domain.UGX(pending)

	// Only 'paid' rows appear in the breakdowns. A pending purchase is not
	// revenue and must never be presented as though it were.
	buckets := []struct {
		into  *[]RevenueBucket
		query string
	}{
		{&report.ByPackage, `
			SELECT s.package_code, pk.name, count(*), COALESCE(sum(p.price_ugx), 0)
			FROM purchases p
			JOIN slips    s  ON s.id = p.slip_id
			JOIN packages pk ON pk.code = s.package_code
			WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
			GROUP BY s.package_code, pk.name, pk.sort_order
			ORDER BY pk.sort_order`},

		{&report.ByAnalyst, `
			SELECT a.id::text, a.name, count(DISTINCT p.id), COALESCE(sum(p.price_ugx), 0)
			FROM purchases p
			JOIN slips         s  ON s.id = p.slip_id
			JOIN slip_analysts sa ON sa.slip_id = s.id
			JOIN analysts      a  ON a.id = sa.analyst_id
			WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
			GROUP BY a.id, a.name
			ORDER BY sum(p.price_ugx) DESC`},

		{&report.BySlip, `
			SELECT s.id::text, s.title, count(*), COALESCE(sum(p.price_ugx), 0)
			FROM purchases p
			JOIN slips s ON s.id = p.slip_id
			WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
			GROUP BY s.id, s.title
			ORDER BY sum(p.price_ugx) DESC
			LIMIT 50`},

		{&report.ByDay, `
			SELECT to_char(date_trunc('day', p.paid_at), 'YYYY-MM-DD'),
			       to_char(date_trunc('day', p.paid_at), 'YYYY-MM-DD'),
			       count(*), COALESCE(sum(p.price_ugx), 0)
			FROM purchases p
			WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
			GROUP BY 1
			ORDER BY 1`},

		{&report.ByProvider, `
			SELECT COALESCE(pt.mobile_provider, 'unknown'),
			       COALESCE(pt.mobile_provider, 'unknown'),
			       count(DISTINCT p.id), COALESCE(sum(p.price_ugx), 0)
			FROM purchases p
			JOIN payment_transactions pt ON pt.purchase_id = p.id AND pt.status = 'completed'
			WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
			GROUP BY 1
			ORDER BY sum(p.price_ugx) DESC`},
	}

	for _, b := range buckets {
		rows, err := db.Pool.Query(ctx, b.query, from, to)
		if err != nil {
			return RevenueReport{}, fmt.Errorf("revenue breakdown: %w", err)
		}
		list := []RevenueBucket{}
		for rows.Next() {
			var bucket RevenueBucket
			var amount int64
			if err := rows.Scan(&bucket.Key, &bucket.Label, &bucket.Purchases, &amount); err != nil {
				rows.Close()
				return RevenueReport{}, fmt.Errorf("scan revenue bucket: %w", err)
			}
			bucket.GrossUGX = domain.UGX(amount)
			list = append(list, bucket)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return RevenueReport{}, err
		}
		*b.into = list
	}

	return report, nil
}

// PaymentLedgerRow is one line of the raw payment ledger, in the shape a
// MarzPay statement is reconciled against.
type PaymentLedgerRow struct {
	TraceCode      string     `json:"traceCode"`
	ProviderTxnID  *string    `json:"providerTxnId,omitempty"`
	Status         string     `json:"status"`
	AmountUGX      domain.UGX `json:"amountUgx"`
	PhoneNumber    string     `json:"phoneNumber"`
	MobileProvider *string    `json:"mobileProvider,omitempty"`
	PackageCode    string     `json:"packageCode"`
	SlipTitle      string     `json:"slipTitle"`
	UserEmail      string     `json:"userEmail"`
	CreatedAt      time.Time  `json:"createdAt"`
	SettledAt      *time.Time `json:"settledAt,omitempty"`
}

// PaymentLedger lists every collection attempt in a window, whatever its
// outcome.
//
// Failed and pending attempts are included deliberately: a statement shows
// what the gateway did, and a line that is missing here because it did not
// succeed is a line nobody can explain.
func (db *DB) PaymentLedger(ctx context.Context, from, to time.Time, limit int) ([]PaymentLedgerRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT pt.trace_code, pt.provider_txn_id, pt.status, pt.amount_ugx,
		       pt.phone_number, pt.mobile_provider,
		       s.package_code, s.title, u.email,
		       pt.created_at, pt.settled_at
		FROM payment_transactions pt
		JOIN purchases p ON p.id = pt.purchase_id
		JOIN slips     s ON s.id = p.slip_id
		JOIN users     u ON u.id = p.user_id
		WHERE pt.created_at >= $1 AND pt.created_at < $2
		ORDER BY pt.created_at DESC
		LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("payment ledger: %w", err)
	}
	defer rows.Close()

	out := []PaymentLedgerRow{}
	for rows.Next() {
		var r PaymentLedgerRow
		var amount int64
		if err := rows.Scan(&r.TraceCode, &r.ProviderTxnID, &r.Status, &amount,
			&r.PhoneNumber, &r.MobileProvider,
			&r.PackageCode, &r.SlipTitle, &r.UserEmail,
			&r.CreatedAt, &r.SettledAt); err != nil {
			return nil, fmt.Errorf("scan ledger row: %w", err)
		}
		r.AmountUGX = domain.UGX(amount)
		out = append(out, r)
	}
	return out, rows.Err()
}
