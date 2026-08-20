package settle_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
)

// grade_test.go proves the pure function. These prove the pass around it:
// which rows it picks up, which it voids, and — the one INGESTION.md names and
// nothing yet covered — what happens when a fixture moves earlier than the
// prediction that was written for it.

type settleFixture struct {
	db       *postgres.DB
	svc      *settle.Service
	leagueID uuid.UUID
	seasonID uuid.UUID
	homeID   uuid.UUID
	awayID   uuid.UUID
}

func newSettleFixture(t *testing.T) settleFixture {
	t.Helper()
	ctx := context.Background()

	db := testdb.New(t)
	f := settleFixture{
		db: db,
		svc: &settle.Service{
			DB:  db,
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
	}

	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO leagues (slug, name, short_name, country, country_code, region)
		VALUES ('test-league','Test League','TL','Testland','TST','europe')
		RETURNING id`).Scan(&f.leagueID); err != nil {
		t.Fatalf("insert league: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO seasons (league_id, label, start_year, is_current)
		VALUES ($1,'2025/26',2025,TRUE) RETURNING id`, f.leagueID).Scan(&f.seasonID); err != nil {
		t.Fatalf("insert season: %v", err)
	}
	for _, team := range []struct {
		slug string
		into *uuid.UUID
	}{{"home-fc", &f.homeID}, {"away-fc", &f.awayID}} {
		if err := db.Pool.QueryRow(ctx, `
			INSERT INTO teams (league_id, slug, name, short_name)
			VALUES ($1,$2,$2,$2) RETURNING id`, f.leagueID, team.slug).Scan(team.into); err != nil {
			t.Fatalf("insert team %s: %v", team.slug, err)
		}
	}
	return f
}

func (f settleFixture) insertMatch(t *testing.T, kickoff time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.db.Pool.QueryRow(context.Background(), `
		INSERT INTO matches (league_id, season_id, home_team_id, away_team_id, kickoff_at)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		f.leagueID, f.seasonID, f.homeID, f.awayID, kickoff).Scan(&id); err != nil {
		t.Fatalf("insert match: %v", err)
	}
	return id
}

func (f settleFixture) predict(t *testing.T, matchID uuid.UUID, market, value string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.db.Pool.QueryRow(context.Background(), `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version)
		VALUES ($1,$2,$3,66.0,'[]'::jsonb,'test-1') RETURNING id`,
		matchID, market, value).Scan(&id); err != nil {
		t.Fatalf("insert prediction: %v", err)
	}
	return id
}

func (f settleFixture) finish(t *testing.T, matchID uuid.UUID, home, away int) {
	t.Helper()
	testdb.MustExec(t, f.db, `
		UPDATE matches SET status = 'finished', home_score = $2, away_score = $3, finished_at = now()
		WHERE id = $1`, matchID, home, away)
}

func (f settleFixture) voidReason(t *testing.T, predictionID uuid.UUID) (string, bool) {
	t.Helper()
	var reason string
	err := f.db.Pool.QueryRow(context.Background(),
		`SELECT reason FROM prediction_voids WHERE prediction_id = $1`, predictionID).Scan(&reason)
	if err != nil {
		return "", false
	}
	return reason, true
}

func (f settleFixture) result(t *testing.T, predictionID uuid.UUID) (outcome string, correct, settled bool) {
	t.Helper()
	err := f.db.Pool.QueryRow(context.Background(),
		`SELECT actual_outcome, was_correct FROM prediction_results WHERE prediction_id = $1`,
		predictionID).Scan(&outcome, &correct)
	if err != nil {
		return "", false, false
	}
	return outcome, correct, true
}

// A settlement pass grades finished matches and leaves everything else alone.
func TestSettlePassGradesOnlyFinishedMatches(t *testing.T) {
	f := newSettleFixture(t)
	ctx := context.Background()

	finished := f.insertMatch(t, time.Now().Add(3*time.Hour))
	upcoming := f.insertMatch(t, time.Now().Add(30*time.Hour))

	won := f.predict(t, finished, "ONE_X_TWO", "HOME")
	lost := f.predict(t, finished, "BTTS", "NO")
	pending := f.predict(t, upcoming, "ONE_X_TWO", "AWAY")

	f.finish(t, finished, 2, 1)

	graded, err := f.svc.SettlePredictions(ctx)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if graded != 2 {
		t.Fatalf("graded %d predictions, want 2", graded)
	}

	if outcome, correct, ok := f.result(t, won); !ok || outcome != "HOME" || !correct {
		t.Errorf("home win graded as outcome=%q correct=%v settled=%v", outcome, correct, ok)
	}
	// Non-negotiable 5: a loss is settled and counted, not quietly dropped.
	if outcome, correct, ok := f.result(t, lost); !ok || outcome != "YES" || correct {
		t.Errorf("2-1 with BTTS NO graded as outcome=%q correct=%v settled=%v", outcome, correct, ok)
	}
	if _, _, ok := f.result(t, pending); ok {
		t.Error("graded a prediction for a match that has not been played")
	}

	// Running again must not double-grade; prediction_results' primary key
	// would raise, so a second pass returning 0 is the assertion.
	again, err := f.svc.SettlePredictions(ctx)
	if err != nil {
		t.Fatalf("second settle pass: %v", err)
	}
	if again != 0 {
		t.Errorf("second pass graded %d predictions, want 0", again)
	}
}

// The case INGESTION.md asks for and nothing covered: a fixture is moved
// *earlier*, to a kickoff that now precedes the prediction's own creation.
//
// The pick was legitimate when it was written — the trigger allowed it — but
// it is no longer true that it predates kickoff, and the trigger cannot catch
// that because it fires on the prediction, not on the reschedule. Voiding is
// the only honest option: keeping it would mean publishing a pick that the
// record cannot prove came first.
func TestRescheduleEarlierThanThePredictionVoidsIt(t *testing.T) {
	f := newSettleFixture(t)
	ctx := context.Background()

	match := f.insertMatch(t, time.Now().Add(48*time.Hour))
	prediction := f.predict(t, match, "ONE_X_TWO", "HOME")

	// Nothing is wrong yet.
	voided, err := f.svc.VoidUngradablePredictions(ctx)
	if err != nil {
		t.Fatalf("void pass before reschedule: %v", err)
	}
	if voided != 0 {
		t.Fatalf("voided %d predictions before anything moved", voided)
	}

	// The broadcaster moves the tie to a slot that has already passed, which
	// retroactively puts kickoff before the pick was written.
	testdb.MustExec(t, f.db,
		`UPDATE matches SET kickoff_at = now() - interval '1 hour' WHERE id = $1`, match)

	voided, err = f.svc.VoidUngradablePredictions(ctx)
	if err != nil {
		t.Fatalf("void pass after reschedule: %v", err)
	}
	if voided != 1 {
		t.Fatalf("voided %d predictions after the reschedule, want 1", voided)
	}

	reason, ok := f.voidReason(t, prediction)
	if !ok {
		t.Fatal("prediction was not voided")
	}
	if reason != "fixture rescheduled earlier than the prediction was made" {
		t.Errorf("void reason = %q", reason)
	}

	// A void has no outcome. It must never become a result row, because that
	// would make it either a free win or a free loss in the accuracy figure.
	if _, _, settled := f.result(t, prediction); settled {
		t.Error("a voided prediction also carries a graded result")
	}

	// The gap between made and settled must be explainable in SQL.
	var audits int
	if err := f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'prediction.voided' AND entity_id = $1`,
		prediction).Scan(&audits); err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if audits != 1 {
		t.Errorf("audit_log holds %d void entries, want 1", audits)
	}

	// The pass runs every 30 minutes forever. It must not re-void.
	voided, err = f.svc.VoidUngradablePredictions(ctx)
	if err != nil {
		t.Fatalf("third void pass: %v", err)
	}
	if voided != 0 {
		t.Errorf("re-voided %d already-voided predictions", voided)
	}
}

// A fixture moved *later* is the ordinary case and changes nothing: the pick
// still predates kickoff.
func TestRescheduleLaterLeavesThePredictionAlone(t *testing.T) {
	f := newSettleFixture(t)
	ctx := context.Background()

	match := f.insertMatch(t, time.Now().Add(6*time.Hour))
	prediction := f.predict(t, match, "BTTS", "YES")

	testdb.MustExec(t, f.db,
		`UPDATE matches SET kickoff_at = now() + interval '10 days' WHERE id = $1`, match)

	voided, err := f.svc.VoidUngradablePredictions(ctx)
	if err != nil {
		t.Fatalf("void pass: %v", err)
	}
	if voided != 0 {
		t.Errorf("voided %d predictions for a fixture that merely moved later", voided)
	}
	if _, ok := f.voidReason(t, prediction); ok {
		t.Error("a postponement to a later date voided a still-valid prediction")
	}
}

// Matches that will never produce a full-time score are voided, and settlement
// stops re-examining them.
func TestCancelledAndAbandonedMatchesAreVoided(t *testing.T) {
	f := newSettleFixture(t)
	ctx := context.Background()

	for _, status := range []string{"cancelled", "abandoned"} {
		t.Run(status, func(t *testing.T) {
			match := f.insertMatch(t, time.Now().Add(4*time.Hour))
			prediction := f.predict(t, match, "ONE_X_TWO", "HOME")

			testdb.MustExec(t, f.db, `UPDATE matches SET status = $2 WHERE id = $1`, match, status)

			if _, err := f.svc.VoidUngradablePredictions(ctx); err != nil {
				t.Fatalf("void pass: %v", err)
			}
			reason, ok := f.voidReason(t, prediction)
			if !ok {
				t.Fatalf("prediction on a %s match was not voided", status)
			}
			if reason != "match "+status {
				t.Errorf("void reason = %q, want %q", reason, "match "+status)
			}
			if _, _, settled := f.result(t, prediction); settled {
				t.Error("a voided prediction also carries a graded result")
			}
		})
	}
}

// Non-negotiable 5: accuracy is computed over every settled prediction, no
// exclusions — and voiding is not a way to exclude a loss. A void has no
// outcome; a loss has one and is counted.
func TestAccuracyCountsEveryLossAndNoVoids(t *testing.T) {
	f := newSettleFixture(t)
	ctx := context.Background()

	played := f.insertMatch(t, time.Now().Add(2*time.Hour))
	f.predict(t, played, "ONE_X_TWO", "HOME") // wins on 2-1
	f.predict(t, played, "BTTS", "NO")        // loses on 2-1
	f.finish(t, played, 2, 1)

	cancelled := f.insertMatch(t, time.Now().Add(2*time.Hour))
	f.predict(t, cancelled, "ONE_X_TWO", "AWAY")
	testdb.MustExec(t, f.db, `UPDATE matches SET status = 'cancelled' WHERE id = $1`, cancelled)

	if _, err := f.svc.SettlePredictions(ctx); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := f.svc.VoidUngradablePredictions(ctx); err != nil {
		t.Fatalf("void: %v", err)
	}
	if err := f.db.RefreshRollups(ctx); err != nil {
		t.Fatalf("refresh rollups: %v", err)
	}

	summary, err := f.db.AccuracySummary(ctx, "")
	if err != nil {
		t.Fatalf("accuracy summary: %v", err)
	}
	if summary.Overall.Total != 2 {
		t.Errorf("accuracy counted %d settled predictions, want 2 (one win, one loss, void excluded)",
			summary.Overall.Total)
	}
	if summary.Overall.Correct != 1 {
		t.Errorf("accuracy counted %d correct, want 1", summary.Overall.Correct)
	}
	if summary.Overall.HitRate != 0.5 {
		t.Errorf("hit rate = %v, want 0.5", summary.Overall.HitRate)
	}
}
