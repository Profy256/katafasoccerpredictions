package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/strength"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

func testFixture(kickoff time.Time) model.Fixture {
	return model.Fixture{
		MatchID:    uuid.New(),
		LeagueID:   uuid.New(),
		HomeTeamID: teamUUID("t0"),
		AwayTeamID: teamUUID("t3"),
		KickoffAt:  kickoff,
	}
}

func engineAt(now time.Time) *model.PoissonEngine {
	e := model.NewPoissonEngine("test-1")
	e.Now = func() time.Time { return now }
	return e
}

// Walk-forward only: a prediction for match M may use only results from
// matches that kicked off before M. A backtest that violates this is
// self-grading and worthless, and in production it means a pick was made with
// information the model could not have had.
func TestPredictRefusesHistoryFromAfterKickoff(t *testing.T) {
	f := loadParity(t)
	kickoff := time.Date(2025, 3, 1, 15, 0, 0, 0, time.UTC)
	fixture := testFixture(kickoff)
	engine := engineAt(kickoff.Add(-24 * time.Hour))

	history := f.goHistory(t, len(f.History))

	// The unfiltered fixture history runs past the target kickoff, so this
	// must be refused rather than quietly producing a flattering prediction.
	if _, err := engine.Predict(fixture, history); !errors.Is(err, model.ErrLeakage) {
		t.Fatalf("predicted from leaked history: err = %v", err)
	}

	// The same call with history truncated at kickoff must succeed.
	var clean []strength.PlayedMatch
	for _, m := range history {
		if m.KickoffAt.Before(kickoff) {
			clean = append(clean, m)
		}
	}
	if len(clean) == 0 {
		t.Fatal("test fixture has no history before the target kickoff")
	}
	if _, err := engine.Predict(fixture, clean); err != nil {
		t.Fatalf("rejected walk-forward history: %v", err)
	}
}

// A match that kicks off at exactly the target kickoff is still leakage: it has
// not been played when the prediction is made.
func TestWalkForwardBoundaryIsExclusive(t *testing.T) {
	kickoff := time.Date(2025, 3, 1, 15, 0, 0, 0, time.UTC)
	history := []strength.PlayedMatch{{
		MatchID:    uuid.New(),
		HomeTeamID: teamUUID("t0"),
		AwayTeamID: teamUUID("t3"),
		KickoffAt:  kickoff,
	}}
	if err := model.AssertWalkForward(history, kickoff); !errors.Is(err, model.ErrLeakage) {
		t.Errorf("a simultaneous kickoff was accepted as history: %v", err)
	}

	history[0].KickoffAt = kickoff.Add(-time.Nanosecond)
	if err := model.AssertWalkForward(history, kickoff); err != nil {
		t.Errorf("a match that kicked off first was rejected: %v", err)
	}
}

// Non-negotiable 2, checked before the database gets a chance to: a pick
// written after the whistle is not a prediction.
func TestPredictRefusesToRunAfterKickoff(t *testing.T) {
	kickoff := time.Date(2025, 3, 1, 15, 0, 0, 0, time.UTC)
	engine := engineAt(kickoff.Add(time.Second))
	if _, err := engine.Predict(testFixture(kickoff), nil); err == nil {
		t.Fatal("produced a prediction for a match that had already started")
	}
}

// Markets are derived from one scoreline matrix precisely so they cannot
// contradict each other. This asserts the published picks stay consistent even
// with no history at all, which is the cold-start state.
func TestPredictionsAreInternallyConsistent(t *testing.T) {
	f := loadParity(t)
	kickoff := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	engine := engineAt(kickoff.Add(-time.Hour))

	for _, size := range []int{0, 1, 12, 120} {
		out, err := engine.Predict(testFixture(kickoff), f.goHistory(t, size))
		if err != nil {
			t.Fatalf("history %d: %v", size, err)
		}
		if len(out.Predictions) != len(domain.MarketCodes) {
			t.Fatalf("history %d: %d predictions, want one per market (%d)",
				size, len(out.Predictions), len(domain.MarketCodes))
		}

		byMarket := map[domain.MarketCode]domain.Prediction{}
		for _, p := range out.Predictions {
			byMarket[p.MarketCode] = p

			if !settle.ValidSelection(p.MarketCode, p.PredictionValue) {
				t.Errorf("history %d: %s picked %q, which the market cannot carry",
					size, p.MarketCode, p.PredictionValue)
			}
			if p.ConfidencePct < 0 || p.ConfidencePct > 100 {
				t.Errorf("history %d: %s confidence %v is out of range",
					size, p.MarketCode, p.ConfidencePct)
			}
			// The published confidence is the model's own probability for the
			// pick, never a separately fabricated number.
			var found bool
			for _, o := range p.Distribution {
				if o.Value == p.PredictionValue {
					found = true
					if diff := o.Probability*100 - p.ConfidencePct; diff > 1e-9 || diff < -1e-9 {
						t.Errorf("history %d: %s confidence %v does not match its own distribution %v",
							size, p.MarketCode, p.ConfidencePct, o.Probability*100)
					}
				}
			}
			if !found {
				t.Errorf("history %d: %s picked %q, absent from its own distribution",
					size, p.MarketCode, p.PredictionValue)
			}
			// The pick is the most likely outcome, so nothing may beat it.
			for _, o := range p.Distribution {
				if o.Probability > p.ConfidencePct/100+1e-12 {
					t.Errorf("history %d: %s published %q at %v%% while %q was likelier",
						size, p.MarketCode, p.PredictionValue, p.ConfidencePct, o.Value)
				}
			}
		}

		// Over 2.5 implies over 1.5: both come off the same matrix, so a
		// disagreement means the derivation is wrong.
		over := func(market domain.MarketCode) float64 {
			for _, o := range byMarket[market].Distribution {
				if o.Value == domain.OutcomeOver {
					return o.Probability
				}
			}
			t.Fatalf("history %d: %s has no OVER outcome", size, market)
			return 0
		}
		if over(domain.MarketOverUnder15) < over(domain.MarketOverUnder25) ||
			over(domain.MarketOverUnder25) < over(domain.MarketOverUnder35) {
			t.Errorf("history %d: over/under probabilities are not monotonic: %v %v %v",
				size, over(domain.MarketOverUnder15), over(domain.MarketOverUnder25),
				over(domain.MarketOverUnder35))
		}
	}
}

// Sample size is reported honestly, and it is what the publication gate reads.
func TestSampleSizeGatesPublication(t *testing.T) {
	f := loadParity(t)
	kickoff := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	engine := engineAt(kickoff.Add(-time.Hour))

	cold, err := engine.Predict(testFixture(kickoff), nil)
	if err != nil {
		t.Fatalf("cold start: %v", err)
	}
	if cold.Reasoning.SampleSize.Home != 0 || cold.Reasoning.SampleSize.Away != 0 {
		t.Errorf("cold start reported sample size %+v, want zeroes", cold.Reasoning.SampleSize)
	}
	if model.PublishableFrom(cold) {
		t.Error("a model with no history at all was cleared for publication")
	}

	warm, err := engine.Predict(testFixture(kickoff), f.goHistory(t, len(f.History)))
	if err != nil {
		t.Fatalf("full history: %v", err)
	}
	if warm.Reasoning.SampleSize.Home < model.MinHistoryPerTeam {
		t.Skipf("fixture history gives only %d matches for the home side; "+
			"publication gate needs %d", warm.Reasoning.SampleSize.Home, model.MinHistoryPerTeam)
	}
	if !model.PublishableFrom(warm) {
		t.Error("a model with full history was withheld from publication")
	}
}

// The detail page shows what the model saw at the time it decided, so the
// snapshot must be populated from the same run that produced the picks.
func TestReasoningSnapshotIsPopulated(t *testing.T) {
	f := loadParity(t)
	kickoff := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	engine := engineAt(kickoff.Add(-time.Hour))

	fixture := testFixture(kickoff)
	out, err := engine.Predict(fixture, f.goHistory(t, len(f.History)))
	if err != nil {
		t.Fatalf("predict: %v", err)
	}

	r := out.Reasoning
	if r.MatchID != fixture.MatchID {
		t.Errorf("reasoning is for match %s, fixture was %s", r.MatchID, fixture.MatchID)
	}
	if r.XGHome < strength.MinXG || r.XGHome > strength.MaxXG ||
		r.XGAway < strength.MinXG || r.XGAway > strength.MaxXG {
		t.Errorf("xg %v/%v outside the clamped range", r.XGHome, r.XGAway)
	}
	if r.HomeForm.Venue != domain.VenueHome || r.AwayForm.Venue != domain.VenueAway {
		t.Error("form summaries are not venue-split as the model uses them")
	}
	if r.HomeForm.TeamID != fixture.HomeTeamID || r.AwayForm.TeamID != fixture.AwayTeamID {
		t.Error("form summaries are attached to the wrong teams")
	}
	if len(r.TopScorelines) != model.TopScorelineCount {
		t.Errorf("%d top scorelines, want %d", len(r.TopScorelines), model.TopScorelineCount)
	}
	for i := 1; i < len(r.TopScorelines); i++ {
		if r.TopScorelines[i].Probability > r.TopScorelines[i-1].Probability {
			t.Error("top scorelines are not in descending probability order")
			break
		}
	}
	// Head-to-head must contain only meetings between these two sides.
	for _, h := range r.HeadToHead {
		sameFixture := (h.HomeTeamID == fixture.HomeTeamID && h.AwayTeamID == fixture.AwayTeamID) ||
			(h.HomeTeamID == fixture.AwayTeamID && h.AwayTeamID == fixture.HomeTeamID)
		if !sameFixture {
			t.Errorf("head-to-head includes %s v %s, neither pairing of the fixture",
				h.HomeTeamID, h.AwayTeamID)
		}
		if !h.KickoffAt.Before(kickoff) {
			t.Error("head-to-head includes a match from after the target kickoff")
		}
	}
	if r.ModelVersion != "test-1" {
		t.Errorf("reasoning stamped %q, want the engine's model version", r.ModelVersion)
	}
}
