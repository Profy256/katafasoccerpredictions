package settle

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// SettleTips grades auto-gradable tips.
//
// A tip is auto-gradable only when match, market and selection are all
// present. Free-text tips wait for an admin decision and are never guessed at.
func (s *Service) SettleTips(ctx context.Context) (graded int, err error) {
	err = s.DB.InTx(ctx, func(tx pgx.Tx) error {
		pending, err := s.DB.TipsAwaitingSettlement(ctx, tx, BatchSize)
		if err != nil {
			return err
		}

		for _, t := range pending {
			result := domain.TipResult{TipID: t.TipID, SettledBy: domain.SettledByAuto}

			switch {
			case t.MatchStatus.IsVoid():
				// The match never produced a full-time score, so the tip has
				// no outcome. It voids, its odds come out of the accumulator,
				// and the remaining legs still stand.
				result.ActualOutcome = domain.VoidOutcome
				result.WasCorrect = false

			case t.HomeScore == nil || t.AwayScore == nil:
				// Finished without a score should be impossible — the schema
				// forbids it — so this is a bug rather than a state to absorb.
				return fmt.Errorf("tip %s: match finished with no score", t.TipID)

			default:
				outcome, won, err := GradeSelection(t.MarketCode, t.SelectionValue, *t.HomeScore, *t.AwayScore)
				if err != nil {
					return fmt.Errorf("grade tip %s: %w", t.TipID, err)
				}
				result.ActualOutcome = outcome
				result.WasCorrect = won
			}

			if err := s.DB.InsertTipResult(ctx, tx, result); err != nil {
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
		s.Log.Info("tips settled", "count", graded)
	}
	return graded, nil
}

// VoidedSlip is a slip whose every leg was called off. Its buyers are refunded.
type VoidedSlip struct {
	SlipID uuid.UUID
}

// CloseSlips settles slips where every tip has resolved.
//
// A slip wins only if every non-void tip won — standard accumulator
// convention. won_tips is stored so the frontend can render "4 of 5" on a
// losing slip without recounting.
func (s *Service) CloseSlips(ctx context.Context) (closed int, voided []VoidedSlip, err error) {
	err = s.DB.InTx(ctx, func(tx pgx.Tx) error {
		ready, err := s.DB.SlipsReadyToSettle(ctx, tx, BatchSize)
		if err != nil {
			return err
		}

		for _, slip := range ready {
			slipID := slip.SlipID

			// Every leg voided: the slip returned nothing and the purchase is
			// refunded. Not a loss — there was no outcome to lose.
			if slip.VoidCount == slip.SettledCount {
				if err := s.DB.VoidSlip(ctx, tx, slipID); err != nil {
					return err
				}
				if err := s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
					ActorType: postgres.ActorJob,
					Action:    "slip.voided",
					Entity:    "slip",
					EntityID:  &slipID,
					Reason:    "every selection was voided",
				}); err != nil {
					return err
				}
				voided = append(voided, VoidedSlip{SlipID: slipID})
				continue
			}

			settledOdds := slip.SurvivingOdds
			if settledOdds.LessThan(decimal.NewFromInt(1)) {
				settledOdds = decimal.NewFromInt(1)
			}

			if err := s.DB.CloseSlip(ctx, tx, slipID, slip.WonCount, settledOdds); err != nil {
				return err
			}
			if err := s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
				ActorType: postgres.ActorJob,
				Action:    "slip.settled",
				Entity:    "slip",
				EntityID:  &slipID,
				After: map[string]any{
					"won_tips":     slip.WonCount,
					"tip_count":    slip.TipCount,
					"void_tips":    slip.VoidCount,
					"settled_odds": settledOdds.StringFixed(3),
				},
			}); err != nil {
				return err
			}
			closed++
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	if closed > 0 || len(voided) > 0 {
		s.Log.Info("slips closed", "settled", closed, "voided", len(voided))
	}
	return closed, voided, nil
}

// AdminSettlement is a human grading a free-text tip.
type AdminSettlement struct {
	TipID         uuid.UUID
	WasCorrect    bool
	ActualOutcome string
	Reason        string
	AdminUserID   uuid.UUID
}

// SettleTipByAdmin records a human decision.
//
// Rules, all enforced here and by the schema: settled_by_user is mandatory;
// the result and the audit entry commit together; a tip whose kickoff has not
// passed cannot be settled; and nothing can be re-settled, because
// tip_results is immutable and primary-keyed on tip_id.
//
// A genuine mistake is corrected by a compensating audit entry and a public
// correction note on the slip, not by editing the result. That will feel
// obnoxious the first time someone fat-fingers a grade, and it is the rule
// that makes the analyst leaderboard mean anything.
func (s *Service) SettleTipByAdmin(ctx context.Context, in AdminSettlement) error {
	if in.AdminUserID == uuid.Nil {
		return fmt.Errorf("%w: an admin settlement must name the user making it", domain.ErrValidation)
	}
	if in.ActualOutcome == "" {
		return fmt.Errorf("%w: actualOutcome is required", domain.ErrValidation)
	}
	if in.Reason == "" {
		return fmt.Errorf("%w: reason is required", domain.ErrValidation)
	}

	return s.DB.InTx(ctx, func(tx pgx.Tx) error {
		tip, err := s.DB.TipForAdminSettlement(ctx, tx, in.TipID)
		if err != nil {
			return err
		}
		if tip.Settled {
			return fmt.Errorf("%w: tip %s is already settled", domain.ErrConflict, in.TipID)
		}
		if !tip.KickoffPast {
			return fmt.Errorf("%w: tip %s has not kicked off yet", domain.ErrConflict, in.TipID)
		}

		adminID := in.AdminUserID
		if err := s.DB.InsertTipResult(ctx, tx, domain.TipResult{
			TipID:         in.TipID,
			WasCorrect:    in.WasCorrect,
			ActualOutcome: in.ActualOutcome,
			SettledBy:     domain.SettledByAdmin,
			SettledByUser: &adminID,
		}); err != nil {
			return err
		}

		tipID := in.TipID
		return s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &adminID,
			Action:    "tip.settled_by_admin",
			Entity:    "tip",
			EntityID:  &tipID,
			Reason:    in.Reason,
			After: map[string]any{
				"was_correct":    in.WasCorrect,
				"actual_outcome": in.ActualOutcome,
				"slip_id":        tip.SlipID,
			},
		})
	})
}
