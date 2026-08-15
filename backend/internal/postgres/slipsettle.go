package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// PendingTip is an auto-gradable tip whose match has resolved one way or
// another.
type PendingTip struct {
	TipID          uuid.UUID
	SlipID         uuid.UUID
	MarketCode     domain.MarketCode
	SelectionValue string
	MatchStatus    domain.MatchStatus
	HomeScore      *int
	AwayScore      *int
}

// TipsAwaitingSettlement finds auto-gradable tips on open slips whose match
// has finished, been cancelled, or been abandoned.
//
// Free-text tips are absent by construction: they have no match_id, and they
// wait for an admin decision rather than being guessed at.
func (db *DB) TipsAwaitingSettlement(ctx context.Context, q Querier, limit int) ([]PendingTip, error) {
	rows, err := q.Query(ctx, `
		SELECT t.id, t.slip_id, t.market_code, t.selection_value,
		       m.status, m.home_score, m.away_score
		FROM tips t
		JOIN slips   s ON s.id = t.slip_id
		JOIN matches m ON m.id = t.match_id
		LEFT JOIN tip_results r ON r.tip_id = t.id
		WHERE s.status = 'open'
		  AND r.tip_id IS NULL
		  AND t.match_id IS NOT NULL
		  AND t.market_code IS NOT NULL
		  AND t.selection_value IS NOT NULL
		  AND m.status IN ('finished','cancelled','abandoned')
		FOR UPDATE OF t SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query tips awaiting settlement: %w", err)
	}
	defer rows.Close()

	var out []PendingTip
	for rows.Next() {
		var t PendingTip
		if err := rows.Scan(&t.TipID, &t.SlipID, &t.MarketCode, &t.SelectionValue,
			&t.MatchStatus, &t.HomeScore, &t.AwayScore); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// InsertTipResult grades one tip. ON CONFLICT DO NOTHING keeps a race from
// double-counting, and tip_results is immutable so nothing can rewrite it.
func (db *DB) InsertTipResult(ctx context.Context, q Querier, r domain.TipResult) error {
	_, err := q.Exec(ctx, `
		INSERT INTO tip_results (tip_id, was_correct, actual_outcome, settled_by, settled_by_user)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tip_id) DO NOTHING`,
		r.TipID, r.WasCorrect, r.ActualOutcome, r.SettledBy, r.SettledByUser)
	if err != nil {
		return fmt.Errorf("insert tip result: %w", err)
	}
	return nil
}

// TipForAdminSettlement is a free-text tip a human is about to grade.
type TipForAdminSettlement struct {
	TipID       uuid.UUID
	SlipID      uuid.UUID
	KickoffPast bool
	Settled     bool
	SlipStatus  domain.SlipStatus
}

func (db *DB) TipForAdminSettlement(ctx context.Context, q Querier, tipID uuid.UUID) (TipForAdminSettlement, error) {
	var t TipForAdminSettlement
	err := q.QueryRow(ctx, `
		SELECT t.id, t.slip_id, t.kickoff_at <= now(),
		       EXISTS (SELECT 1 FROM tip_results r WHERE r.tip_id = t.id),
		       s.status
		FROM tips t
		JOIN slips s ON s.id = t.slip_id
		WHERE t.id = $1`, tipID).
		Scan(&t.TipID, &t.SlipID, &t.KickoffPast, &t.Settled, &t.SlipStatus)
	if err != nil {
		if isNoRows(err) {
			return TipForAdminSettlement{}, domain.ErrNotFound
		}
		return TipForAdminSettlement{}, fmt.Errorf("query tip: %w", err)
	}
	return t, nil
}

// SlipSettlementState is everything needed to decide whether a slip can close.
type SlipSettlementState struct {
	SlipID       uuid.UUID
	TipCount     int
	SettledCount int
	WonCount     int
	VoidCount    int
	// SurvivingOdds is the product of the odds of every tip that was not
	// voided. Void legs are removed from the accumulator — the standard
	// bookmaker rule, and the only one fair to a buyer whose match was called
	// off.
	SurvivingOdds decimal.Decimal
}

// SlipsReadyToSettle finds open slips where every tip has resolved.
func (db *DB) SlipsReadyToSettle(ctx context.Context, q Querier, limit int) ([]SlipSettlementState, error) {
	rows, err := q.Query(ctx, `
		SELECT s.id,
		       s.tip_count,
		       count(r.tip_id)                                          AS settled,
		       count(*) FILTER (WHERE r.was_correct)                    AS won,
		       count(*) FILTER (WHERE r.actual_outcome = $2)            AS voided,
		       COALESCE(
		         (SELECT COALESCE(exp(sum(ln(t2.odds))), 1)
		            FROM tips t2
		            JOIN tip_results r2 ON r2.tip_id = t2.id
		           WHERE t2.slip_id = s.id AND r2.actual_outcome <> $2),
		         1)::text                                               AS surviving_odds
		FROM slips s
		JOIN tips t        ON t.slip_id = s.id
		LEFT JOIN tip_results r ON r.tip_id = t.id
		WHERE s.status = 'open'
		GROUP BY s.id
		HAVING count(t.id) = count(r.tip_id)
		LIMIT $1`, limit, domain.VoidOutcome)
	if err != nil {
		return nil, fmt.Errorf("query slips ready to settle: %w", err)
	}
	defer rows.Close()

	var out []SlipSettlementState
	for rows.Next() {
		var s SlipSettlementState
		var odds string
		if err := rows.Scan(&s.SlipID, &s.TipCount, &s.SettledCount,
			&s.WonCount, &s.VoidCount, &odds); err != nil {
			return nil, err
		}
		parsed, err := decimal.NewFromString(odds)
		if err != nil {
			return nil, fmt.Errorf("parse surviving odds %q: %w", odds, err)
		}
		s.SurvivingOdds = parsed
		out = append(out, s)
	}
	return out, rows.Err()
}

// CloseSlip settles a slip.
//
// settled_odds is written rather than total_odds being mutated: the published
// price is frozen at publication and must keep showing what buyers were
// advertised. Both figures are displayed.
func (db *DB) CloseSlip(ctx context.Context, q Querier, slipID uuid.UUID, wonTips int, settledOdds decimal.Decimal) error {
	_, err := q.Exec(ctx, `
		UPDATE slips
		SET status = 'settled', settled_at = now(), won_tips = $2, settled_odds = $3::numeric
		WHERE id = $1 AND status = 'open'`,
		slipID, wonTips, settledOdds.StringFixed(3))
	if err != nil {
		return fmt.Errorf("close slip: %w", err)
	}
	return nil
}

// VoidSlip marks a slip whose every leg was called off. Its purchases are
// refunded; the slip and the purchases both stay on the record.
func (db *DB) VoidSlip(ctx context.Context, q Querier, slipID uuid.UUID) error {
	_, err := q.Exec(ctx, `
		UPDATE slips SET status = 'void', won_tips = 0
		WHERE id = $1 AND status = 'open'`, slipID)
	if err != nil {
		return fmt.Errorf("void slip: %w", err)
	}
	return nil
}

func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
