// Package predict runs the model over upcoming fixtures and stores the
// results.
//
// It is the only writer of predictions and match_reasoning. Both are written
// in one transaction per fixture, so a match can never end up with published
// picks and no record of what the model saw when it made them.
package predict

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

type Service struct {
	DB     *postgres.DB
	Engine model.Engine
	Log    *slog.Logger
	// ModelVersion is stamped onto every prediction.
	ModelVersion string
}

// Horizon is how far ahead the model prices fixtures.
//
// This is the single number that decides how much football the site can show.
// Predictions are what make a fixture visible at all — the feed drops any
// fixture with none attached — so a horizon shorter than the fixture sync
// leaves everything past it ingested and invisible: no feed row, no team-page
// entry, no league-page entry, no match URL in the sitemap, and nothing for
// the free shortlist to select from. At the old 48 hours a midweek day showed
// a single fixture across the entire site.
//
// It is not simply maximised, because predictions are immutable and
// UpcomingFixtures skips a fixture already priced at this model version: a
// pick made seven days out keeps that pick, made on form seven days stale,
// and it goes into the permanent accuracy record that way. Seven days is the
// point where the coming weekend is always visible from midweek without
// pricing a fortnight of football on old form.
//
// Walk-forward is unaffected either way — the model only ever reads results
// from before the fixture it is pricing, whenever it runs.
//
// It must stay at least tips.MaxWindowDays long, or the shortlist's window can
// reach days that have nothing priced on them. TestHorizonCoversShortlistWindow
// pins that.
const Horizon = 7 * 24 * time.Hour

type Stats struct {
	Fixtures    int
	Predictions int
	Skipped     int
}

// GenerateUpcoming predicts every fixture kicking off within the window that
// has no prediction at the current model version.
func (s *Service) GenerateUpcoming(ctx context.Context, within time.Duration) (Stats, error) {
	var stats Stats

	fixtures, err := s.DB.UpcomingFixtures(ctx, within, s.ModelVersion)
	if err != nil {
		return stats, err
	}

	for _, match := range fixtures {
		// Walk-forward: only results from matches that kicked off before this
		// one. The query bounds it and the engine asserts it again, because a
		// query that is correct today can be edited tomorrow.
		history, err := s.DB.LeagueHistory(ctx, match.LeagueID, match.KickoffAt)
		if err != nil {
			return stats, err
		}

		out, err := s.Engine.Predict(model.Fixture{
			MatchID:    match.ID,
			LeagueID:   match.LeagueID,
			HomeTeamID: match.HomeTeamID,
			AwayTeamID: match.AwayTeamID,
			KickoffAt:  match.KickoffAt,
		}, history)
		if err != nil {
			if errors.Is(err, model.ErrLeakage) {
				// A real bug, not a data condition: fail loudly rather than
				// publishing a pick made with information the model could not
				// have had.
				return stats, err
			}
			// Most likely the fixture kicked off between the query and now.
			s.Log.Warn("skipping fixture", "match_id", match.ID, "err", err)
			stats.Skipped++
			continue
		}

		written, err := s.store(ctx, out)
		if err != nil {
			return stats, err
		}
		stats.Fixtures++
		stats.Predictions += written
	}

	return stats, nil
}

// store writes a fixture's predictions and its reasoning snapshot together.
func (s *Service) store(ctx context.Context, out model.Output) (int, error) {
	written := 0
	err := s.DB.InTx(ctx, func(tx pgx.Tx) error {
		for _, p := range out.Predictions {
			id, err := s.DB.InsertPrediction(ctx, tx, p)
			if err != nil {
				return err
			}
			// uuid.Nil means the row already existed at this model version.
			// Reruns are normal and idempotent.
			if id != uuid.Nil {
				written++
			}
		}
		return s.DB.UpsertMatchReasoning(ctx, tx, out.Reasoning)
	})
	if err != nil {
		return 0, err
	}
	return written, nil
}
