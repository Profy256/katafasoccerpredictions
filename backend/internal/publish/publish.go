// Package publish freezes the daily free shortlist.
//
// Selection itself lives in internal/tips and is pure. This package is the
// part that makes the result a fact: it runs the selection once and writes it
// to free_tip_days / free_tips, after which the API only ever reads those
// rows.
package publish

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/tips"
)

type Service struct {
	DB           *postgres.DB
	Log          *slog.Logger
	ModelVersion string
	// PerMarket is how many tips each market publishes.
	PerMarket int
}

// PublishNextDay selects and freezes the next matchday's shortlist.
//
// Idempotent: a day already published is left exactly as it was. Republishing
// would be the one thing this whole design exists to prevent — the frozen list
// is what "we went 4 from 5 yesterday" refers to, and a second selection over
// changed data would quietly replace it.
func (s *Service) PublishNextDay(ctx context.Context) (tips.Day, error) {
	candidates, err := s.DB.FreeTipCandidates(ctx, s.DB.Pool, s.ModelVersion, model.MinHistoryPerTeam)
	if err != nil {
		return tips.Day{}, err
	}

	day := tips.Select(candidates, s.PerMarket)
	if day.IsEmpty() {
		return day, nil
	}

	published, err := s.DB.FreeTipsDayPublished(ctx, s.DB.Pool, day.Day)
	if err != nil {
		return tips.Day{}, err
	}
	if published {
		s.Log.Info("free shortlist already published, leaving it alone",
			"day", day.Day.Format("2006-01-02"))
		return tips.Day{}, nil
	}

	if err := s.DB.InTx(ctx, func(tx pgx.Tx) error {
		return s.DB.PublishFreeTips(ctx, tx, day, s.ModelVersion)
	}); err != nil {
		return tips.Day{}, err
	}

	// A widened window is not an error, but it is worth seeing: one starved
	// midweek day is normal, a run of them means fixtures or history have
	// stopped arriving.
	if day.CoversDays > 1 {
		s.Log.Info("matchday too thin to fill a shortlist, window extended",
			"day", day.Day.Format("2006-01-02"),
			"covers_days", day.CoversDays,
			"tips", day.TotalTips,
			"floor", tips.MinShortlistSize)
	}
	return day, nil
}
