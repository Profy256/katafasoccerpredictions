// Package model turns a fixture and a league's history into published
// predictions.
//
// The Engine interface exists so the numerics can be replaced — by a separate
// service, or a different model entirely — without the API noticing. The Go
// implementation below is the only one that ships today.
package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/poisson"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/strength"
)

// Fixture is the match being predicted.
type Fixture struct {
	MatchID    uuid.UUID
	LeagueID   uuid.UUID
	HomeTeamID uuid.UUID
	AwayTeamID uuid.UUID
	KickoffAt  time.Time
}

// Output is everything one run produces: a pick per market, plus the snapshot
// of what the model saw when it decided.
type Output struct {
	Predictions []domain.Prediction
	Reasoning   domain.MatchReasoning
}

type Engine interface {
	// Predict must use only matches that kicked off before f.KickoffAt.
	Predict(f Fixture, history []strength.PlayedMatch) (Output, error)
}

// HeadToHeadLimit is how many previous meetings the detail page shows.
const HeadToHeadLimit = 5

// RecentFormLimit is how many results the form strip renders.
const RecentFormLimit = 5

// TopScorelineCount is how many exact scorelines the detail page shows.
const TopScorelineCount = 6

// PoissonEngine is the shipped implementation.
type PoissonEngine struct {
	// ModelVersion is stamped onto every prediction it produces. The accuracy
	// dashboard reports per version, and a model upgrade adds predictions
	// rather than rewriting the ones already published.
	ModelVersion string

	// Now is injectable so tests can produce predictions for fixtures at a
	// fixed point in time. Defaults to time.Now.
	Now func() time.Time
}

func NewPoissonEngine(modelVersion string) *PoissonEngine {
	return &PoissonEngine{ModelVersion: modelVersion, Now: time.Now}
}

// ErrLeakage is returned when history contains a match that had not kicked off
// before the fixture being predicted.
//
// A prediction for match M may use only results from matches that kicked off
// before M. A backtest that violates this is self-grading and worthless, and
// in production it would mean a pick was made with information the model could
// not have had.
var ErrLeakage = fmt.Errorf("model: history leaks results from at or after the target kickoff")

// AssertWalkForward fails if any match in history kicked off at or after the
// target kickoff. Called on every Predict, not only in backtests: the check is
// cheap and the failure it catches is invisible.
func AssertWalkForward(history []strength.PlayedMatch, kickoff time.Time) error {
	for _, m := range history {
		if !m.KickoffAt.Before(kickoff) {
			return fmt.Errorf("%w: match %s kicked off at %s, target at %s",
				ErrLeakage, m.MatchID, m.KickoffAt.UTC().Format(time.RFC3339),
				kickoff.UTC().Format(time.RFC3339))
		}
	}
	return nil
}

// Predict runs the full pipeline: league baselines → venue-split strengths →
// expected goals → one scoreline matrix → a pick per market.
//
// history must be the league's finished matches, oldest first, all of them
// kicked off before f.KickoffAt.
func (e *PoissonEngine) Predict(f Fixture, history []strength.PlayedMatch) (Output, error) {
	if e.ModelVersion == "" {
		return Output{}, fmt.Errorf("model: ModelVersion is required")
	}
	if err := AssertWalkForward(history, f.KickoffAt); err != nil {
		return Output{}, err
	}

	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	createdAt := now().UTC()

	// A prediction created at or after kickoff is not a prediction. The
	// database enforces this too; catching it here turns a failed INSERT into
	// a clear error at the point the mistake was made.
	if !createdAt.Before(f.KickoffAt) {
		return Output{}, fmt.Errorf(
			"model: refusing to predict match %s at %s, kickoff was %s",
			f.MatchID, createdAt.Format(time.RFC3339), f.KickoffAt.UTC().Format(time.RFC3339))
	}

	baselines := strength.ComputeBaselines(history)
	home := strength.ComputeTeamStrengths(f.HomeTeamID, history, baselines)
	away := strength.ComputeTeamStrengths(f.AwayTeamID, history, baselines)
	xg := strength.Expected(home, away, baselines)

	matrix := poisson.Build(xg.Home, xg.Away)

	predictions := make([]domain.Prediction, 0, len(domain.MarketCodes))
	for _, market := range domain.MarketCodes {
		distribution, err := matrix.Distribution(market)
		if err != nil {
			return Output{}, err
		}
		best := poisson.PickBest(distribution)

		predictions = append(predictions, domain.Prediction{
			MatchID:    f.MatchID,
			MarketCode: market,
			// The published pick is the most likely outcome, and the
			// confidence figure is the model's own probability for it — never
			// a separately fabricated number.
			PredictionValue: best.Value,
			ConfidencePct:   best.Probability * 100,
			Distribution:    distribution,
			ModelVersion:    e.ModelVersion,
			CreatedAt:       createdAt,
		})
	}

	reasoning := domain.MatchReasoning{
		MatchID: f.MatchID,
		XGHome:  xg.Home,
		XGAway:  xg.Away,
		HomeForm: strength.FormSummaryFor(home, domain.VenueHome,
			strength.RecentForm(f.HomeTeamID, history, RecentFormLimit)),
		AwayForm: strength.FormSummaryFor(away, domain.VenueAway,
			strength.RecentForm(f.AwayTeamID, history, RecentFormLimit)),
		HeadToHead:    headToHead(f, history, HeadToHeadLimit),
		TopScorelines: matrix.TopScorelines(TopScorelineCount),
		ModelVersion:  e.ModelVersion,
		// Sample size is published because it is the honest disclosure: a pick
		// drawn from eight matches must not look like one drawn from two
		// hundred.
		SampleSize: domain.SampleSize{Home: home.TotalGames, Away: away.TotalGames},
	}

	return Output{Predictions: predictions, Reasoning: reasoning}, nil
}

// headToHead returns previous meetings between the two sides, most recent
// first, regardless of which of them was at home.
func headToHead(f Fixture, history []strength.PlayedMatch, limit int) []domain.HeadToHeadMatch {
	out := make([]domain.HeadToHeadMatch, 0, limit)
	for i := len(history) - 1; i >= 0 && len(out) < limit; i-- {
		m := history[i]
		sameFixture := (m.HomeTeamID == f.HomeTeamID && m.AwayTeamID == f.AwayTeamID) ||
			(m.HomeTeamID == f.AwayTeamID && m.AwayTeamID == f.HomeTeamID)
		if !sameFixture {
			continue
		}
		out = append(out, domain.HeadToHeadMatch{
			MatchID:    m.MatchID,
			KickoffAt:  m.KickoffAt,
			HomeTeamID: m.HomeTeamID,
			AwayTeamID: m.AwayTeamID,
			HomeScore:  m.HomeScore,
			AwayScore:  m.AwayScore,
		})
	}
	return out
}

// MinHistoryPerTeam is the sample-size floor for publication.
//
// A league whose teams have less history than this does not get published
// tips: it runs in shadow mode, with predictions generated and settled
// internally, until there is enough evidence for the strength estimates to
// mean anything. Skipping this is the tempting way to launch with more leagues
// on the page, and the accuracy dashboard would expose it publicly and
// permanently.
const MinHistoryPerTeam = 40

// PublishableFrom reports whether both sides have enough history for the pick
// to be published rather than merely recorded.
func PublishableFrom(out Output) bool {
	return out.Reasoning.SampleSize.Home >= MinHistoryPerTeam &&
		out.Reasoning.SampleSize.Away >= MinHistoryPerTeam
}
