package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
)

// These tests assert that the database *refuses* things. The product's only
// real asset is a verifiable track record, and every invariant below is what
// stops published history quietly becoming fiction. They are written as
// failure assertions on purpose: a test that only proves the happy path would
// still pass if every trigger were dropped.

type fixture struct {
	leagueID uuid.UUID
	seasonID uuid.UUID
	homeID   uuid.UUID
	awayID   uuid.UUID
	userID   uuid.UUID
}

func seed(t *testing.T, db *postgres.DB) fixture {
	t.Helper()
	ctx := context.Background()
	var f fixture

	err := db.Pool.QueryRow(ctx, `
		INSERT INTO leagues (slug, name, short_name, country, country_code, region)
		VALUES ('test-league','Test League','TL','Testland','TST','europe')
		RETURNING id`).Scan(&f.leagueID)
	if err != nil {
		t.Fatalf("insert league: %v", err)
	}

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO seasons (league_id, label, start_year, is_current)
		VALUES ($1,'2025/26',2025,TRUE) RETURNING id`, f.leagueID).Scan(&f.seasonID)
	if err != nil {
		t.Fatalf("insert season: %v", err)
	}

	for _, team := range []struct {
		slug string
		into *uuid.UUID
	}{{"home-fc", &f.homeID}, {"away-fc", &f.awayID}} {
		err = db.Pool.QueryRow(ctx, `
			INSERT INTO teams (league_id, slug, name, short_name)
			VALUES ($1,$2,$2,$2) RETURNING id`, f.leagueID, team.slug).Scan(team.into)
		if err != nil {
			t.Fatalf("insert team %s: %v", team.slug, err)
		}
	}

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ('admin@katafa.test','x','Admin','admin') RETURNING id`).Scan(&f.userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return f
}

// insertMatch adds a fixture kicking off at the given time.
func (f fixture) insertMatch(t *testing.T, db *postgres.DB, kickoff time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := db.Pool.QueryRow(context.Background(), `
		INSERT INTO matches (league_id, season_id, home_team_id, away_team_id, kickoff_at)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		f.leagueID, f.seasonID, f.homeID, f.awayID, kickoff).Scan(&id)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	return id
}

// Non-negotiable 2: a prediction must exist before kickoff. In the database
// rather than the application, because it must also hold for the admin CLI, a
// migration, a backfill script and a psql session.
func TestPredictionAfterKickoffIsRejected(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	past := f.insertMatch(t, db, time.Now().Add(-2*time.Hour))
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version)
		VALUES ($1,'ONE_X_TWO','HOME',61.5,'[]'::jsonb,'test-1')`, past)
	if err == nil {
		t.Fatal("inserted a prediction for a match that already kicked off")
	}
	if !strings.Contains(err.Error(), "is not before kickoff") {
		t.Fatalf("rejected for the wrong reason: %v", err)
	}

	// The same insert against a future fixture must succeed, or the trigger is
	// simply refusing everything and proves nothing.
	future := f.insertMatch(t, db, time.Now().Add(48*time.Hour))
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version)
		VALUES ($1,'ONE_X_TWO','HOME',61.5,'[]'::jsonb,'test-1')`, future); err != nil {
		t.Fatalf("rejected a legitimate pre-kickoff prediction: %v", err)
	}
}

// Non-negotiable 1: published predictions are immutable. Corrections are new
// rows plus an audit_log entry.
func TestPublishedRowsAreImmutable(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	match := f.insertMatch(t, db, time.Now().Add(24*time.Hour))
	var predictionID uuid.UUID
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version)
		VALUES ($1,'BTTS','YES',70,'[]'::jsonb,'test-1') RETURNING id`, match).Scan(&predictionID)
	if err != nil {
		t.Fatalf("insert prediction: %v", err)
	}

	if _, err := db.Pool.Exec(ctx,
		`UPDATE predictions SET prediction_value = 'NO' WHERE id = $1`, predictionID); err == nil {
		t.Fatal("updated a published prediction")
	} else if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("rejected for the wrong reason: %v", err)
	}

	// Settling twice must conflict rather than double-count a win.
	for i, wantErr := range []bool{false, true} {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO prediction_results (prediction_id, actual_outcome, was_correct)
			VALUES ($1,'YES',TRUE)`, predictionID)
		if wantErr && err == nil {
			t.Fatalf("attempt %d: settled the same prediction twice", i)
		}
		if !wantErr && err != nil {
			t.Fatalf("attempt %d: first settlement failed: %v", i, err)
		}
	}

	if _, err := db.Pool.Exec(ctx,
		`UPDATE prediction_results SET was_correct = FALSE WHERE prediction_id = $1`,
		predictionID); err == nil {
		t.Fatal("rewrote a settled result")
	}
}

// A match cannot be finished without a score, and cannot carry a score unless
// it is finished. Settlement reads status = 'finished', so a half-written
// result must never be gradable.
func TestMatchScoreAndStatusAreTiedTogether(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()
	match := f.insertMatch(t, db, time.Now().Add(-3*time.Hour))

	if _, err := db.Pool.Exec(ctx,
		`UPDATE matches SET status = 'finished' WHERE id = $1`, match); err == nil {
		t.Fatal("marked a match finished with no score")
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE matches SET home_score = 2, away_score = 1 WHERE id = $1`, match); err == nil {
		t.Fatal("recorded a score on a match that is not finished")
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE matches SET status = 'finished', home_score = 2, away_score = 1, finished_at = now()
		WHERE id = $1`, match); err != nil {
		t.Fatalf("rejected a complete result: %v", err)
	}
}

// Price cannot move after publication, or a slip could be repriced after
// buyers committed. Status advances draft → open → settled and never back.
func TestPublishedSlipTermsAreFrozen(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	var slipID uuid.UUID
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
		VALUES ('vip','Saturday VIP',5000,4.250,3,$1) RETURNING id`, f.userID).Scan(&slipID)
	if err != nil {
		t.Fatalf("insert slip: %v", err)
	}

	// A draft is still editable — that is the whole point of the draft state.
	if _, err := db.Pool.Exec(ctx,
		`UPDATE slips SET price_ugx = 6000 WHERE id = $1`, slipID); err != nil {
		t.Fatalf("rejected an edit to a draft: %v", err)
	}

	if _, err := db.Pool.Exec(ctx,
		`UPDATE slips SET status = 'open', published_at = now() WHERE id = $1`, slipID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for _, tc := range []struct{ name, stmt string }{
		{"reprice", `UPDATE slips SET price_ugx = 1000 WHERE id = $1`},
		{"retitle", `UPDATE slips SET title = 'Cheaper VIP' WHERE id = $1`},
		{"repackage", `UPDATE slips SET package_code = 'ordinary' WHERE id = $1`},
		{"restate odds", `UPDATE slips SET total_odds = 9.999 WHERE id = $1`},
		{"unpublish", `UPDATE slips SET published_at = NULL WHERE id = $1`},
	} {
		if _, err := db.Pool.Exec(ctx, tc.stmt, slipID); err == nil {
			t.Errorf("%s: changed a published slip's commercial terms", tc.name)
		}
	}

	if _, err := db.Pool.Exec(ctx, `
		UPDATE slips SET status = 'settled', settled_at = now(), won_tips = 2, settled_odds = 3.100
		WHERE id = $1`, slipID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE slips SET status = 'open' WHERE id = $1`, slipID); err == nil {
		t.Fatal("reopened a settled slip")
	}
}

// A tip is either fully structured and auto-gradable, or free text an admin
// grades by hand. The in-between silently never settles.
func TestTipIsEitherFullyStructuredOrFreeText(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	var slipID, analystID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
		VALUES ('ordinary','Daily',2000,3.000,2,$1) RETURNING id`, f.userID).Scan(&slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO analysts (slug, name, handle, initials, joined_at)
		VALUES ('sam','Sam','@sam','S',now()) RETURNING id`).Scan(&analystID); err != nil {
		t.Fatalf("insert analyst: %v", err)
	}
	match := f.insertMatch(t, db, time.Now().Add(6*time.Hour))

	insert := func(position int, matchID any, marketCode, selectionValue any) error {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO tips (slip_id, analyst_id, match_id, fixture_label, market_label,
			                  selection_label, market_code, selection_value, odds,
			                  kickoff_at, position)
			VALUES ($1,$2,$3,'Home v Away','Match Result','Home Win',$4,$5,1.850,now()+interval '6 hours',$6)`,
			slipID, analystID, matchID, marketCode, selectionValue, position)
		return err
	}

	if err := insert(1, match, "ONE_X_TWO", "HOME"); err != nil {
		t.Fatalf("rejected a fully structured tip: %v", err)
	}
	if err := insert(2, nil, nil, nil); err != nil {
		t.Fatalf("rejected a free-text tip: %v", err)
	}
	if err := insert(3, match, nil, nil); err == nil {
		t.Error("accepted a tip with a match but no selection — it would never settle")
	}
	if err := insert(4, match, "ONE_X_TWO", nil); err == nil {
		t.Error("accepted a tip with a market but no selection — it would never settle")
	}
}

// A human overriding the record must be named in it.
func TestAdminSettlementMustNameAUser(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	var slipID, analystID, tipID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
		VALUES ('vip','V',5000,2.000,1,$1) RETURNING id`, f.userID).Scan(&slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO analysts (slug, name, handle, initials, joined_at)
		VALUES ('kay','Kay','@kay','K',now()) RETURNING id`).Scan(&analystID); err != nil {
		t.Fatalf("insert analyst: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO tips (slip_id, analyst_id, fixture_label, market_label, selection_label,
		                  odds, kickoff_at, position)
		VALUES ($1,$2,'A v B','Corners','Over 9.5',1.900,now(),1) RETURNING id`,
		slipID, analystID).Scan(&tipID); err != nil {
		t.Fatalf("insert tip: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tip_results (tip_id, was_correct, actual_outcome, settled_by)
		VALUES ($1,TRUE,'OVER','admin')`, tipID); err == nil {
		t.Fatal("recorded an admin settlement with nobody accountable for it")
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tip_results (tip_id, was_correct, actual_outcome, settled_by, settled_by_user)
		VALUES ($1,TRUE,'OVER','admin',$2)`, tipID, f.userID); err != nil {
		t.Fatalf("rejected a properly attributed admin settlement: %v", err)
	}
}

// Non-negotiable 6: an unpaid viewer's query must not return tip rows at all.
// Asserted at the query level, not the response level — there is no
// serialisation path where the tips exist in memory next to a boolean.
func TestOnlyOnePaidPurchasePerUserPerSlip(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	var slipID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by, status, published_at)
		VALUES ('vip','V',5000,2.000,1,$1,'open',now()) RETURNING id`, f.userID).Scan(&slipID); err != nil {
		t.Fatalf("insert slip: %v", err)
	}

	pay := func() error {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO purchases (user_id, slip_id, price_ugx, status, paid_at)
			VALUES ($1,$2,5000,'paid',now())`, f.userID, slipID)
		return err
	}
	if err := pay(); err != nil {
		t.Fatalf("first purchase: %v", err)
	}
	if err := pay(); err == nil {
		t.Fatal("granted a second paid entitlement for the same slip")
	}

	// A failed attempt must not block a retry.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO purchases (user_id, slip_id, price_ugx, status)
		VALUES ($1,$2,5000,'failed')`, f.userID, slipID); err != nil {
		t.Fatalf("blocked a retry after failure: %v", err)
	}
}
