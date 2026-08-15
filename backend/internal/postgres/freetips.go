package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/tips"
)

// Everything here reads free_tips. Nothing re-derives the shortlist.
//
// The selection ran once, at publish time, and was frozen. Re-deriving it on
// read would make "yesterday's tips went 4 from 5" a claim about a list that no
// longer exists — once matches finish they stop being scheduled, so the live
// selection cannot reproduce them, and a model rerun would produce a different
// list if it could.

// PublishFreeTips freezes a day's shortlist.
//
// The free_tip_days row and its free_tips rows are written in one transaction,
// so a crash cannot leave a day that claims ten tips and carries four.
func (db *DB) PublishFreeTips(ctx context.Context, q Querier, day tips.Day, modelVersion string) error {
	if day.IsEmpty() {
		return fmt.Errorf("refusing to publish an empty shortlist for %s",
			day.Day.Format(time.DateOnly))
	}

	_, err := q.Exec(ctx, `
		INSERT INTO free_tip_days (day, model_version, total_tips)
		VALUES ($1,$2,$3)
		ON CONFLICT (day) DO NOTHING`,
		day.Day, modelVersion, day.TotalTips)
	if err != nil {
		return fmt.Errorf("insert free tip day: %w", err)
	}

	for _, group := range day.Groups {
		for _, tip := range group.Tips {
			_, err := q.Exec(ctx, `
				INSERT INTO free_tips (day, market_code, prediction_id, odds, rank)
				VALUES ($1,$2,$3,$4::numeric,$5)
				ON CONFLICT (day, market_code, rank) DO NOTHING`,
				day.Day, tip.MarketCode, tip.PredictionID, tip.Odds.String(), tip.Rank)
			if err != nil {
				return fmt.Errorf("insert free tip %s: %w", tip.PredictionID, err)
			}
		}
	}
	return nil
}

// FreeTipsDayPublished reports whether a day has already been frozen. The
// publish job checks this rather than relying on ON CONFLICT, so a rerun logs
// "already published" instead of silently doing nothing.
func (db *DB) FreeTipsDayPublished(ctx context.Context, q Querier, day time.Time) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM free_tip_days WHERE day = $1)`, day).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check published day: %w", err)
	}
	return exists, nil
}

// LatestFreeTipsDay is the most recently published shortlist date.
func (db *DB) LatestFreeTipsDay(ctx context.Context) (time.Time, error) {
	var day time.Time
	err := db.Pool.QueryRow(ctx, `SELECT day FROM free_tip_days ORDER BY day DESC LIMIT 1`).Scan(&day)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, domain.ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest free tips day: %w", err)
	}
	return day, nil
}

// FreeTips returns one published day, grouped by market in display order.
func (db *DB) FreeTips(ctx context.Context, day time.Time) (domain.FreeTipsDay, error) {
	var totalTips int
	err := db.Pool.QueryRow(ctx,
		`SELECT total_tips FROM free_tip_days WHERE day = $1`, day).Scan(&totalTips)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FreeTipsDay{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FreeTipsDay{}, fmt.Errorf("query free tip day: %w", err)
	}

	groups, _, _, err := db.freeTipGroups(ctx, day, day)
	if err != nil {
		return domain.FreeTipsDay{}, err
	}

	return domain.FreeTipsDay{
		Date:      day.Format(time.DateOnly),
		Groups:    groups[day.Format(time.DateOnly)],
		TotalTips: totalTips,
	}, nil
}

// FreeTipsHistory serves "yesterday's free tips went 4 from 5" by joining the
// frozen shortlist to prediction_results.
func (db *DB) FreeTipsHistory(ctx context.Context, from, to time.Time) ([]domain.FreeTipsHistoryDay, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT day, total_tips FROM free_tip_days
		WHERE day BETWEEN $1 AND $2
		ORDER BY day DESC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query free tip days: %w", err)
	}
	defer rows.Close()

	type dayRow struct {
		day   time.Time
		total int
	}
	var days []dayRow
	for rows.Next() {
		var d dayRow
		if err := rows.Scan(&d.day, &d.total); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(days) == 0 {
		return []domain.FreeTipsHistoryDay{}, nil
	}

	groups, settled, correct, err := db.freeTipGroups(ctx, from, to)
	if err != nil {
		return nil, err
	}

	out := make([]domain.FreeTipsHistoryDay, 0, len(days))
	for _, d := range days {
		key := d.day.Format(time.DateOnly)
		entry := domain.FreeTipsHistoryDay{
			Date:      key,
			TotalTips: d.total,
			Settled:   settled[key],
			Correct:   correct[key],
			Groups:    groups[key],
		}
		if entry.Settled > 0 {
			entry.HitRate = float64(entry.Correct) / float64(entry.Settled)
		}
		out = append(out, entry)
	}
	return out, nil
}

// freeTipGroups loads the frozen tips for a date range, with results attached
// where the pick has been graded.
func (db *DB) freeTipGroups(ctx context.Context, from, to time.Time) (
	groups map[string][]domain.FreeTipGroup,
	settled map[string]int,
	correct map[string]int,
	err error,
) {
	rows, err := db.Pool.Query(ctx, `
		SELECT ft.day, ft.market_code, ft.odds::float8, ft.rank,
		       p.id, p.match_id, p.market_code, p.prediction_value,
		       p.confidence_pct::float8, p.distribution, p.model_version, p.created_at,
		       m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
		       m.kickoff_at, m.status, m.home_score, m.away_score, m.round,
		       r.actual_outcome, r.was_correct, r.settled_at
		FROM free_tips ft
		JOIN predictions p ON p.id = ft.prediction_id
		JOIN matches     m ON m.id = p.match_id
		JOIN market_types mt ON mt.code = ft.market_code
		LEFT JOIN prediction_results r ON r.prediction_id = p.id
		WHERE ft.day BETWEEN $1 AND $2
		ORDER BY ft.day DESC, mt.sort_order, ft.rank`, from, to)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query free tips: %w", err)
	}
	defer rows.Close()

	leagues, teams, err := db.referenceMaps(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	groups = map[string][]domain.FreeTipGroup{}
	settled = map[string]int{}
	correct = map[string]int{}

	for rows.Next() {
		var day time.Time
		var groupMarket domain.MarketCode
		var odds float64
		var rank int
		var p domain.Prediction
		var distribution []byte
		var m domain.Match
		var actualOutcome *string
		var wasCorrect *bool
		var settledAt *time.Time

		if err := rows.Scan(&day, &groupMarket, &odds, &rank,
			&p.ID, &p.MatchID, &p.MarketCode, &p.PredictionValue,
			&p.ConfidencePct, &distribution, &p.ModelVersion, &p.CreatedAt,
			&m.ID, &m.LeagueID, &m.SeasonID, &m.HomeTeamID, &m.AwayTeamID,
			&m.KickoffAt, &m.Status, &m.HomeScore, &m.AwayScore, &m.Round,
			&actualOutcome, &wasCorrect, &settledAt); err != nil {
			return nil, nil, nil, fmt.Errorf("scan free tip: %w", err)
		}
		if err := json.Unmarshal(distribution, &p.Distribution); err != nil {
			return nil, nil, nil, fmt.Errorf("decode distribution: %w", err)
		}

		key := day.Format(time.DateOnly)
		tip := domain.FreeTip{
			Match:      m.ToAPI(),
			League:     leagues[m.LeagueID],
			HomeTeam:   teams[m.HomeTeamID],
			AwayTeam:   teams[m.AwayTeamID],
			Prediction: p.ToAPI(),
			Odds:       odds,
		}
		if wasCorrect != nil && actualOutcome != nil && settledAt != nil {
			tip.Result = &domain.APIPredictionResult{
				PredictionID:  p.ID,
				ActualOutcome: *actualOutcome,
				WasCorrect:    *wasCorrect,
				SettledAt:     settledAt.UTC(),
			}
			settled[key]++
			if *wasCorrect {
				correct[key]++
			}
		}

		// Rows arrive ordered by market, so the last group is the right one
		// until the market changes.
		list := groups[key]
		if len(list) == 0 || list[len(list)-1].Market != groupMarket {
			list = append(list, domain.FreeTipGroup{Market: groupMarket})
		}
		list[len(list)-1].Tips = append(list[len(list)-1].Tips, tip)
		groups[key] = list
	}
	return groups, settled, correct, rows.Err()
}

// FreeTipCandidates loads the predictions eligible for a shortlist.
//
// Eligibility is decided here, not in tips.Select, which is pure: scheduled
// fixtures only (never in_play — a "tip" on a match already underway is not a
// tip), from published leagues only, at one model version, and only where the
// reasoning snapshot shows enough history for the pick to be publishable.
func (db *DB) FreeTipCandidates(ctx context.Context, q Querier, modelVersion string, minSample int) ([]tips.Candidate, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.match_id, p.market_code, p.prediction_value,
		       p.confidence_pct::float8, p.distribution, p.model_version, p.created_at,
		       m.kickoff_at
		FROM predictions p
		JOIN matches  m ON m.id = p.match_id
		JOIN leagues  l ON l.id = m.league_id
		JOIN match_reasoning mr ON mr.match_id = m.id
		WHERE m.status = 'scheduled'
		  AND m.kickoff_at > now()
		  AND l.is_active
		  AND l.is_published
		  AND p.model_version = $1
		  AND mr.sample_home >= $2
		  AND mr.sample_away >= $2
		ORDER BY m.kickoff_at, p.id`, modelVersion, minSample)
	if err != nil {
		return nil, fmt.Errorf("query free tip candidates: %w", err)
	}
	defer rows.Close()

	var out []tips.Candidate
	for rows.Next() {
		var p domain.Prediction
		var distribution []byte
		var kickoff time.Time
		if err := rows.Scan(&p.ID, &p.MatchID, &p.MarketCode, &p.PredictionValue,
			&p.ConfidencePct, &distribution, &p.ModelVersion, &p.CreatedAt, &kickoff); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		if err := json.Unmarshal(distribution, &p.Distribution); err != nil {
			return nil, fmt.Errorf("decode distribution: %w", err)
		}
		out = append(out, tips.Candidate{Prediction: p, KickoffAt: kickoff})
	}
	return out, rows.Err()
}
