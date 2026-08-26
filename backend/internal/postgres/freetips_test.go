package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/testdb"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/tips"
)

// The selection window reaches past the immediate matchday when a day is too
// thin to fill a page (tips.MaxWindowDays), so two consecutive runs see an
// overlapping set of still-scheduled fixtures. What stops the second run
// re-publishing the first run's picks is FreeTipCandidates excluding anything
// already in free_tips.
//
// Without it "Monday went 4 from 5, Tuesday went 4 from 5" would be one set of
// five results reported as two days of record. That is the free tier's whole
// claim, so it is asserted rather than trusted.
func TestFreeTipCandidatesExcludeAlreadyPublishedPredictions(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	const modelVersion = "test-1"
	const minSample = 40

	// seed() leaves the league in shadow mode, which the candidate query gates
	// on; without this the query returns nothing and every assertion below
	// passes for the wrong reason.
	f.publishLeague(t, db)

	match := f.insertMatch(t, db, time.Now().Add(30*time.Hour))
	predictionID := insertPricedPrediction(t, db, match, "BTTS", "YES", 65, minSample)

	// Baseline: eligible before it has ever been published.
	before, err := db.FreeTipCandidates(ctx, db.Pool, modelVersion, minSample)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if !containsPrediction(before, predictionID) {
		t.Fatalf("a fresh, in-window, well-sampled prediction was not offered as a candidate; "+
			"got %d candidates", len(before))
	}

	// Publish it as part of a day's shortlist.
	day := time.Now().UTC().Truncate(24 * time.Hour)
	err = db.PublishFreeTips(ctx, db.Pool, tips.Day{
		Day:        day,
		CoversDays: 2,
		TotalTips:  1,
		Groups: []tips.Group{{
			Market: "BTTS",
			Tips: []tips.Tip{{
				PredictionID: predictionID,
				MatchID:      match,
				MarketCode:   "BTTS",
				Odds:         mustOdds(t, "1.48"),
				Rank:         1,
			}},
		}},
	}, modelVersion)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The next run must not see it again, even though the fixture is still
	// scheduled and still inside the window.
	after, err := db.FreeTipCandidates(ctx, db.Pool, modelVersion, minSample)
	if err != nil {
		t.Fatalf("candidates after publish: %v", err)
	}
	if containsPrediction(after, predictionID) {
		t.Error("an already-published prediction was offered to a second shortlist; " +
			"it would be published on two days and counted in both records")
	}
}

// The exclusion must be narrow: it removes the published prediction, not the
// fixture. A match tipped in one market can still be tipped in another.
func TestFreeTipCandidatesKeepOtherMarketsOnAPublishedFixture(t *testing.T) {
	db := testdb.New(t)
	f := seed(t, db)
	ctx := context.Background()

	const modelVersion = "test-1"
	const minSample = 40

	f.publishLeague(t, db)

	match := f.insertMatch(t, db, time.Now().Add(30*time.Hour))
	published := insertPricedPrediction(t, db, match, "BTTS", "YES", 65, minSample)
	other := insertPricedPrediction(t, db, match, "ONE_X_TWO", "HOME", 55, minSample)

	day := time.Now().UTC().Truncate(24 * time.Hour)
	err := db.PublishFreeTips(ctx, db.Pool, tips.Day{
		Day: day, CoversDays: 1, TotalTips: 1,
		Groups: []tips.Group{{
			Market: "BTTS",
			Tips: []tips.Tip{{
				PredictionID: published, MatchID: match, MarketCode: "BTTS",
				Odds: mustOdds(t, "1.48"), Rank: 1,
			}},
		}},
	}, modelVersion)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	after, err := db.FreeTipCandidates(ctx, db.Pool, modelVersion, minSample)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if containsPrediction(after, published) {
		t.Error("published prediction still offered")
	}
	if !containsPrediction(after, other) {
		t.Error("publishing one market on a fixture removed the fixture's other markets " +
			"from selection; the appearance cap governs that, not this exclusion")
	}
}

// insertPricedPrediction adds a prediction plus the match_reasoning row the
// sample-size gate reads, so the candidate query has everything it joins on.
func insertPricedPrediction(
	t *testing.T, db *postgres.DB, match uuid.UUID,
	market, value string, confidence float64, sample int,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version)
		VALUES ($1,$2,$3,$4,'[]'::jsonb,'test-1') RETURNING id`,
		match, market, value, confidence).Scan(&id)
	if err != nil {
		t.Fatalf("insert prediction: %v", err)
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO match_reasoning (match_id, xg_home, xg_away, home_form, away_form,
		                             head_to_head, top_scorelines, model_version,
		                             sample_home, sample_away)
		VALUES ($1, 1.4, 1.1, '{}'::jsonb, '{}'::jsonb, '[]'::jsonb, '[]'::jsonb,
		        'test-1', $2, $2)
		ON CONFLICT (match_id) DO NOTHING`, match, sample)
	if err != nil {
		t.Fatalf("insert match_reasoning: %v", err)
	}
	return id
}

// publishLeague clears the seeded league's publication gate.
func (f fixture) publishLeague(t *testing.T, db *postgres.DB) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(),
		`UPDATE leagues SET is_published = TRUE WHERE id = $1`, f.leagueID)
	if err != nil {
		t.Fatalf("publish league: %v", err)
	}
}

func mustOdds(t *testing.T, value string) domain.Odds {
	t.Helper()
	odds, err := decimal.NewFromString(value)
	if err != nil {
		t.Fatalf("odds %q: %v", value, err)
	}
	return odds
}

func containsPrediction(candidates []tips.Candidate, id uuid.UUID) bool {
	for _, c := range candidates {
		if c.Prediction.ID == id {
			return true
		}
	}
	return false
}
