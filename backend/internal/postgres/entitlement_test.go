package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
)

// Non-negotiable 6: the paywall is a SQL boundary. An unpaid viewer's query
// must not return tip rows at all — never fetch-then-hide, in any layer.
//
// Every test here reads through db.Slip, the same call the API handler makes,
// and asserts on len(Tips). The load-bearing part is countTips: it proves the
// rows *are* in the table, so zero tips back is the query filtering rather
// than an empty fixture. A test that only checked len(Tips) == 0 would pass
// just as happily against a slip nobody ever wrote tips to.

// paywallFixture is an open slip with two tips, plus a buyer and a non-buyer.
type paywallFixture struct {
	fixture
	slipID     uuid.UUID
	analystID  uuid.UUID
	buyerID    uuid.UUID
	strangerID uuid.UUID
}

func seedPaywall(t *testing.T, db *postgres.DB) paywallFixture {
	t.Helper()
	ctx := context.Background()

	p := paywallFixture{fixture: seed(t, db)}

	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO analysts (slug, name, handle, initials, joined_at)
		VALUES ('nakato','Nakato','@nakato','N',now()) RETURNING id`).Scan(&p.analystID); err != nil {
		t.Fatalf("insert analyst: %v", err)
	}

	for _, u := range []struct {
		email string
		into  *uuid.UUID
	}{{"buyer@katafa.test", &p.buyerID}, {"stranger@katafa.test", &p.strangerID}} {
		if err := db.Pool.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, name, role)
			VALUES ($1,'x','Punter','user') RETURNING id`, u.email).Scan(u.into); err != nil {
			t.Fatalf("insert user %s: %v", u.email, err)
		}
	}

	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count,
		                   created_by, status, published_at)
		VALUES ('vip','Saturday Banker',5000,4.250,2,$1,'open',now()) RETURNING id`,
		p.userID).Scan(&p.slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}

	match := p.insertMatch(t, db, time.Now().Add(6*time.Hour))
	for i, tip := range []struct {
		market, selection, label string
	}{
		{"ONE_X_TWO", "HOME", "Home Win"},
		{"BTTS", "YES", "Both Teams to Score"},
	} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tips (slip_id, analyst_id, match_id, fixture_label, market_label,
			                  selection_label, market_code, selection_value, odds,
			                  kickoff_at, position)
			VALUES ($1,$2,$3,'Home FC v Away FC','Market',$4,$5,$6,1.850,
			        now() + interval '6 hours',$7)`,
			p.slipID, p.analystID, match, tip.label, tip.market, tip.selection, i+1); err != nil {
			t.Fatalf("insert tip %d: %v", i+1, err)
		}
	}

	return p
}

// countTips reads the tips table directly, with no entitlement clause. It is
// what makes the assertions below mean something: the rows are there, and the
// paid read path still refuses to return them.
func countTips(t *testing.T, db *postgres.DB, slipID uuid.UUID) int {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM tips WHERE slip_id = $1`, slipID).Scan(&n); err != nil {
		t.Fatalf("count tips: %v", err)
	}
	return n
}

// ROADMAP phase 6's "done when": an unpaid viewer provably receives zero tip
// rows.
func TestUnpaidViewerReceivesZeroTipRows(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	ctx := context.Background()

	if got := countTips(t, db, p.slipID); got != 2 {
		t.Fatalf("fixture is wrong: slip carries %d tips, want 2 — the assertions below would be vacuous", got)
	}

	// Every way of not having paid. The purchase statuses are enumerated
	// rather than represented by one case because each is a different row in
	// the table and only 'paid' may unlock.
	for _, tc := range []struct {
		name     string
		viewer   func() uuid.UUID
		purchase string // purchase status to arrange, "" for none
	}{
		{name: "anonymous", viewer: func() uuid.UUID { return uuid.Nil }},
		{name: "signed in, never bought", viewer: func() uuid.UUID { return p.strangerID }},
		{name: "purchase still pending", viewer: func() uuid.UUID { return p.strangerID }, purchase: "pending"},
		{name: "purchase failed", viewer: func() uuid.UUID { return p.strangerID }, purchase: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.purchase != "" {
				testdb.MustExec(t, db, `DELETE FROM purchases WHERE user_id = $1 AND slip_id = $2`,
					p.strangerID, p.slipID)
				testdb.MustExec(t, db, `
					INSERT INTO purchases (user_id, slip_id, price_ugx, status)
					VALUES ($1,$2,5000,$3)`, p.strangerID, p.slipID, tc.purchase)
			}

			slip, err := db.Slip(ctx, p.slipID, tc.viewer(), false)
			if err != nil {
				t.Fatalf("read slip: %v", err)
			}
			if len(slip.Tips) != 0 {
				t.Errorf("unpaid viewer received %d tip rows from the database", len(slip.Tips))
			}
			if slip.Unlocked {
				t.Error("slip reported itself unlocked to a viewer who has not paid")
			}
			// The metadata a buyer needs in order to decide is still served.
			if slip.TipCount != 2 || slip.PriceUGX != domain.UGX(5000) {
				t.Errorf("shop metadata was withheld along with the tips: tipCount=%d price=%d",
					slip.TipCount, int64(slip.PriceUGX))
			}
		})
	}
}

func TestPaidViewerReceivesTheTips(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	ctx := context.Background()

	testdb.MustExec(t, db, `
		INSERT INTO purchases (user_id, slip_id, price_ugx, status, paid_at)
		VALUES ($1,$2,5000,'paid',now())`, p.buyerID, p.slipID)

	slip, err := db.Slip(ctx, p.slipID, p.buyerID, false)
	if err != nil {
		t.Fatalf("read slip: %v", err)
	}
	if len(slip.Tips) != 2 {
		t.Fatalf("buyer received %d tip rows, want 2 — the paywall is refusing a paid viewer", len(slip.Tips))
	}
	if !slip.Unlocked {
		t.Error("slip reported itself locked to a viewer who paid for it")
	}

	// One buyer's entitlement must not leak to the next viewer along.
	stranger, err := db.Slip(ctx, p.slipID, p.strangerID, false)
	if err != nil {
		t.Fatalf("read slip as stranger: %v", err)
	}
	if len(stranger.Tips) != 0 {
		t.Errorf("a stranger received %d tip rows because somebody else had paid", len(stranger.Tips))
	}
}

// A refund revokes access. The purchase row stays — the history does not get
// deleted — so the entitlement has to turn on status rather than existence.
func TestRefundRevokesAccess(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	ctx := context.Background()

	testdb.MustExec(t, db, `
		INSERT INTO purchases (user_id, slip_id, price_ugx, status, paid_at)
		VALUES ($1,$2,5000,'paid',now())`, p.buyerID, p.slipID)

	before, err := db.Slip(ctx, p.slipID, p.buyerID, false)
	if err != nil {
		t.Fatalf("read slip: %v", err)
	}
	if len(before.Tips) != 2 {
		t.Fatalf("buyer could not see the tips before the refund: got %d", len(before.Tips))
	}

	testdb.MustExec(t, db, `
		UPDATE purchases SET status = 'refunded' WHERE user_id = $1 AND slip_id = $2`,
		p.buyerID, p.slipID)

	after, err := db.Slip(ctx, p.slipID, p.buyerID, false)
	if err != nil {
		t.Fatalf("read slip after refund: %v", err)
	}
	if len(after.Tips) != 0 {
		t.Errorf("refunded viewer still received %d tip rows", len(after.Tips))
	}

	// The record of the payment survives the revocation.
	var status string
	var paidAt *time.Time
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, paid_at FROM purchases WHERE user_id = $1 AND slip_id = $2`,
		p.buyerID, p.slipID).Scan(&status, &paidAt); err != nil {
		t.Fatalf("read purchase: %v", err)
	}
	if status != "refunded" {
		t.Errorf("purchase status = %q, want refunded", status)
	}
	if paidAt == nil {
		t.Error("paid_at was cleared by the refund — when the money arrived is part of the record")
	}
}

// Non-negotiable 7: settled slips become public. That is what makes an
// analyst's record auditable rather than a claim, and it means the losing
// slips are visible too.
func TestSettledSlipsArePublic(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	ctx := context.Background()

	testdb.MustExec(t, db, `
		UPDATE slips SET status = 'settled', settled_at = now(), won_tips = 0, settled_odds = 4.250
		WHERE id = $1`, p.slipID)

	for _, viewer := range []struct {
		name string
		id   uuid.UUID
	}{
		{"anonymous", uuid.Nil},
		{"signed in, never bought", p.strangerID},
	} {
		t.Run(viewer.name, func(t *testing.T) {
			slip, err := db.Slip(ctx, p.slipID, viewer.id, false)
			if err != nil {
				t.Fatalf("read settled slip: %v", err)
			}
			if len(slip.Tips) != 2 {
				t.Errorf("settled slip withheld its tips from %s: got %d rows, want 2",
					viewer.name, len(slip.Tips))
			}
		})
	}
}

// insertDraftSlip adds a second, never-published slip carrying one tip.
//
// It is inserted as a draft rather than demoted from the open slip on purpose:
// the slips_guarded trigger refuses to unpublish, which is itself one of the
// invariants, so a test that reached for UPDATE here would be testing against
// a state the database will not let exist.
func insertDraftSlip(t *testing.T, db *postgres.DB, p paywallFixture) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var slipID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
		VALUES ('ordinary','Unfinished Sunday',2000,2.100,1,$1) RETURNING id`,
		p.userID).Scan(&slipID); err != nil {
		t.Fatalf("insert draft slip: %v", err)
	}

	match := p.insertMatch(t, db, time.Now().Add(30*time.Hour))
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tips (slip_id, analyst_id, match_id, fixture_label, market_label,
		                  selection_label, market_code, selection_value, odds,
		                  kickoff_at, position)
		VALUES ($1,$2,$3,'Home FC v Away FC','Market','Over 2.5','OVER_UNDER_2_5','OVER',2.100,
		        now() + interval '30 hours',1)`,
		slipID, p.analystID, match); err != nil {
		t.Fatalf("insert draft tip: %v", err)
	}
	return slipID
}

// A draft is a 404 rather than a 403, so unpublished slip ids stay
// undiscoverable by probing.
func TestDraftSlipIsInvisibleButAdminsSeeIt(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	draftID := insertDraftSlip(t, db, p)
	ctx := context.Background()

	if _, err := db.Slip(ctx, draftID, p.strangerID, false); err != domain.ErrNotFound {
		t.Errorf("draft slip returned %v, want ErrNotFound — a 403 would confirm the id exists", err)
	}
	if _, err := db.Slip(ctx, draftID, uuid.Nil, false); err != domain.ErrNotFound {
		t.Errorf("draft slip returned %v to an anonymous prober, want ErrNotFound", err)
	}

	admin, err := db.Slip(ctx, draftID, p.userID, true)
	if err != nil {
		t.Fatalf("admin could not read the draft they are authoring: %v", err)
	}
	if len(admin.Tips) != 1 {
		t.Errorf("admin received %d tip rows, want 1", len(admin.Tips))
	}
}

// Drafts must not appear in the list either, or their ids leak there instead.
func TestSlipListExcludesDrafts(t *testing.T) {
	db := testdb.New(t)
	p := seedPaywall(t, db)
	draftID := insertDraftSlip(t, db, p)
	ctx := context.Background()

	slips, _, err := db.Slips(ctx, postgres.SlipQuery{})
	if err != nil {
		t.Fatalf("list slips: %v", err)
	}
	if len(slips) != 1 {
		t.Fatalf("listed %d slips, want only the one open slip", len(slips))
	}
	if slips[0].ID == draftID {
		t.Fatal("the draft was listed and the published slip was not")
	}
	if slips[0].ID != p.slipID {
		t.Fatalf("listed slip %s, want the open slip %s", slips[0].ID, p.slipID)
	}
}
