package settle

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Service runs settlement passes. It holds no grading logic of its own: Grade
// is the pure function above, and this only reads rows, calls it, and writes
// the outcome.
type Service struct {
	DB  *postgres.DB
	Log *slog.Logger
}

// BatchSize caps one settlement pass. Large enough that a matchday clears in a
// few passes, small enough that the transaction stays short.
const BatchSize = 500

// SettlePredictions grades every prediction whose match has finished.
//
// The whole batch runs in one transaction, so a crash mid-batch replays
// cleanly rather than leaving half a matchday graded.
func (s *Service) SettlePredictions(ctx context.Context) (graded int, err error) {
	err = s.DB.InTx(ctx, func(tx pgx.Tx) error {
		pending, err := s.DB.PredictionsAwaitingSettlement(ctx, tx, BatchSize)
		if err != nil {
			return err
		}

		for _, p := range pending {
			outcome, err := Grade(p.MarketCode, p.HomeScore, p.AwayScore)
			if err != nil {
				// An ungradable market is a bug, not a data condition. Fail
				// the batch rather than silently skipping it forever.
				return fmt.Errorf("grade prediction %s: %w", p.PredictionID, err)
			}

			if err := s.DB.InsertPredictionResult(ctx, tx, domain.PredictionResult{
				PredictionID:  p.PredictionID,
				ActualOutcome: outcome,
				WasCorrect:    Matches(p.MarketCode, p.PredictionValue, outcome),
			}); err != nil {
				return err
			}
			graded++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	if graded > 0 {
		s.Log.Info("predictions settled", "count", graded)
	}
	return graded, nil
}

// VoidUngradablePredictions voids picks on matches that will never produce a
// full-time score, and picks retroactively invalidated by a reschedule.
//
// Every void is written to audit_log with its reason, so the gap between
// "predictions made" and "predictions settled" is always explainable.
func (s *Service) VoidUngradablePredictions(ctx context.Context) (voided int, err error) {
	err = s.DB.InTx(ctx, func(tx pgx.Tx) error {
		cancelled, err := s.DB.PredictionsAwaitingVoid(ctx, tx, BatchSize)
		if err != nil {
			return err
		}
		for _, v := range cancelled {
			reason := fmt.Sprintf("match %s", v.Status)
			if err := s.voidOne(ctx, tx, v, reason); err != nil {
				return err
			}
			voided++
		}

		// A fixture moved *earlier*, to a kickoff that now precedes the
		// prediction's own creation. The pick was legitimate when made, but it
		// is no longer true that it predates kickoff. This is rare and must
		// never be papered over.
		invalidated, err := s.DB.PredictionsInvalidatedByReschedule(ctx, tx)
		if err != nil {
			return err
		}
		for _, v := range invalidated {
			const reason = "fixture rescheduled earlier than the prediction was made"
			if err := s.voidOne(ctx, tx, v, reason); err != nil {
				return err
			}
			s.Log.Warn("prediction invalidated by reschedule",
				"prediction_id", v.PredictionID, "match_id", v.MatchID)
			voided++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if voided > 0 {
		s.Log.Info("predictions voided", "count", voided)
	}
	return voided, nil
}

func (s *Service) voidOne(ctx context.Context, tx pgx.Tx, v postgres.VoidablePrediction, reason string) error {
	if err := s.DB.VoidPrediction(ctx, tx, v.PredictionID, reason); err != nil {
		return err
	}
	predictionID := v.PredictionID
	return s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
		ActorType: postgres.ActorJob,
		Action:    "prediction.voided",
		Entity:    "prediction",
		EntityID:  &predictionID,
		Reason:    reason,
		After:     map[string]any{"match_id": v.MatchID, "match_status": v.Status},
	})
}
