package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// predictionsForMatches loads predictions for a set of matches, applying the
// market and confidence filters in SQL.
func (db *DB) predictionsForMatches(
	ctx context.Context,
	matchIDs []uuid.UUID,
	markets []domain.MarketCode,
	minConfidence float64,
) (map[uuid.UUID][]domain.Prediction, error) {
	args := []any{matchIDs}
	where := []string{"p.match_id = ANY($1)"}

	if len(markets) > 0 {
		codes := make([]string, len(markets))
		for i, m := range markets {
			codes[i] = string(m)
		}
		args = append(args, codes)
		where = append(where, fmt.Sprintf("p.market_code = ANY($%d)", len(args)))
	}
	if minConfidence > 0 {
		args = append(args, minConfidence)
		where = append(where, fmt.Sprintf("p.confidence_pct >= $%d", len(args)))
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT p.id, p.match_id, p.market_code, p.prediction_value,
		       p.confidence_pct::float8, p.distribution, p.model_version, p.created_at
		FROM predictions p
		JOIN market_types mt ON mt.code = p.market_code
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY p.match_id, mt.sort_order, p.created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("query predictions: %w", err)
	}

	predictions, err := scanPredictions(rows)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID][]domain.Prediction, len(matchIDs))
	for _, p := range predictions {
		out[p.MatchID] = append(out[p.MatchID], p)
	}
	return out, nil
}

func scanPredictions(rows pgx.Rows) ([]domain.Prediction, error) {
	defer rows.Close()
	var out []domain.Prediction
	for rows.Next() {
		var p domain.Prediction
		var distribution []byte
		if err := rows.Scan(&p.ID, &p.MatchID, &p.MarketCode, &p.PredictionValue,
			&p.ConfidencePct, &distribution, &p.ModelVersion, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan prediction: %w", err)
		}
		if err := json.Unmarshal(distribution, &p.Distribution); err != nil {
			return nil, fmt.Errorf("decode distribution for %s: %w", p.ID, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (db *DB) resultsForMatch(ctx context.Context, matchID uuid.UUID) ([]domain.APIPredictionResult, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT r.prediction_id, r.actual_outcome, r.was_correct, r.settled_at
		FROM prediction_results r
		JOIN predictions p ON p.id = r.prediction_id
		WHERE p.match_id = $1
		ORDER BY r.settled_at`, matchID)
	if err != nil {
		return nil, fmt.Errorf("query results: %w", err)
	}
	defer rows.Close()

	var out []domain.APIPredictionResult
	for rows.Next() {
		var r domain.APIPredictionResult
		if err := rows.Scan(&r.PredictionID, &r.ActualOutcome, &r.WasCorrect, &r.SettledAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertPrediction writes one published pick.
//
// There is no update counterpart anywhere in this package, by design. A model
// upgrade inserts a new row under a new model_version; it never rewrites what
// was published. The database enforces both that and the before-kickoff rule.
func (db *DB) InsertPrediction(ctx context.Context, q Querier, p domain.Prediction) (uuid.UUID, error) {
	distribution, err := json.Marshal(p.Distribution)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encode distribution: %w", err)
	}

	var id uuid.UUID
	err = q.QueryRow(ctx, `
		INSERT INTO predictions (match_id, market_code, prediction_value,
		                         confidence_pct, distribution, model_version, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (match_id, market_code, model_version) DO NOTHING
		RETURNING id`,
		p.MatchID, p.MarketCode, p.PredictionValue, p.ConfidencePct,
		distribution, p.ModelVersion, p.CreatedAt).Scan(&id)
	if err == pgx.ErrNoRows {
		// Already predicted at this model version. Not an error: the
		// generation job is idempotent and reruns are normal.
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert prediction: %w", err)
	}
	return id, nil
}

// UpsertMatchReasoning writes the snapshot of what the model saw.
//
// ON CONFLICT DO NOTHING rather than DO UPDATE: the snapshot belongs to the
// run that produced the published picks, and overwriting it with a later run's
// view is exactly the flattering rewrite the table exists to prevent.
func (db *DB) UpsertMatchReasoning(ctx context.Context, q Querier, r domain.MatchReasoning) error {
	homeForm, err := json.Marshal(r.HomeForm)
	if err != nil {
		return fmt.Errorf("encode home form: %w", err)
	}
	awayForm, err := json.Marshal(r.AwayForm)
	if err != nil {
		return fmt.Errorf("encode away form: %w", err)
	}
	h2h, err := json.Marshal(r.HeadToHead)
	if err != nil {
		return fmt.Errorf("encode head to head: %w", err)
	}
	scorelines, err := json.Marshal(r.TopScorelines)
	if err != nil {
		return fmt.Errorf("encode scorelines: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO match_reasoning (match_id, xg_home, xg_away, home_form, away_form,
		                             head_to_head, top_scorelines, sample_home, sample_away,
		                             model_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (match_id) DO NOTHING`,
		r.MatchID, r.XGHome, r.XGAway, homeForm, awayForm, h2h, scorelines,
		r.SampleSize.Home, r.SampleSize.Away, r.ModelVersion)
	if err != nil {
		return fmt.Errorf("insert reasoning: %w", err)
	}
	return nil
}

// PendingSettlement is a graded-ready prediction: its match has finished and
// it has no result yet.
type PendingSettlement struct {
	PredictionID    uuid.UUID
	MatchID         uuid.UUID
	MarketCode      domain.MarketCode
	PredictionValue string
	HomeScore       int
	AwayScore       int
}

// PredictionsAwaitingSettlement locks a batch of gradable predictions.
//
// SKIP LOCKED so two workers running the same cron minute divide the work
// rather than contending, and FOR UPDATE OF p so the lock is on the
// predictions rows only.
func (db *DB) PredictionsAwaitingSettlement(ctx context.Context, q Querier, limit int) ([]PendingSettlement, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.match_id, p.market_code, p.prediction_value, m.home_score, m.away_score
		FROM predictions p
		JOIN matches m ON m.id = p.match_id
		LEFT JOIN prediction_results r ON r.prediction_id = p.id
		LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
		WHERE m.status = 'finished'
		  AND r.prediction_id IS NULL
		  AND v.prediction_id IS NULL
		FOR UPDATE OF p SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending settlement: %w", err)
	}
	defer rows.Close()

	var out []PendingSettlement
	for rows.Next() {
		var p PendingSettlement
		if err := rows.Scan(&p.PredictionID, &p.MatchID, &p.MarketCode,
			&p.PredictionValue, &p.HomeScore, &p.AwayScore); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InsertPredictionResult records a graded pick.
//
// ON CONFLICT DO NOTHING is the idempotency guarantee: two workers racing
// produce one row, not two, and never a double-counted win.
func (db *DB) InsertPredictionResult(ctx context.Context, q Querier, r domain.PredictionResult) error {
	_, err := q.Exec(ctx, `
		INSERT INTO prediction_results (prediction_id, actual_outcome, was_correct)
		VALUES ($1,$2,$3)
		ON CONFLICT (prediction_id) DO NOTHING`,
		r.PredictionID, r.ActualOutcome, r.WasCorrect)
	if err != nil {
		return fmt.Errorf("insert prediction result: %w", err)
	}
	return nil
}

// VoidablePrediction is an unsettled pick whose match will never produce a
// full-time score.
type VoidablePrediction struct {
	PredictionID uuid.UUID
	MatchID      uuid.UUID
	Status       domain.MatchStatus
}

// PredictionsAwaitingVoid finds unsettled predictions on cancelled or
// abandoned matches.
//
// Postponed is deliberately absent. A postponement is handled by *waiting*:
// the provider reschedules the same fixture id, ingestion moves kickoff_at,
// and the prediction — made before the original kickoff, therefore before the
// new one — settles when the match is eventually played. Re-predicting a
// rescheduled fixture would be a post-hoc pick with fresher information.
func (db *DB) PredictionsAwaitingVoid(ctx context.Context, q Querier, limit int) ([]VoidablePrediction, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.match_id, m.status
		FROM predictions p
		JOIN matches m ON m.id = p.match_id
		LEFT JOIN prediction_results r ON r.prediction_id = p.id
		LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
		WHERE m.status IN ('cancelled','abandoned')
		  AND r.prediction_id IS NULL
		  AND v.prediction_id IS NULL
		FOR UPDATE OF p SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query voidable predictions: %w", err)
	}
	defer rows.Close()

	var out []VoidablePrediction
	for rows.Next() {
		var v VoidablePrediction
		if err := rows.Scan(&v.PredictionID, &v.MatchID, &v.Status); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VoidPrediction records that a pick can never be graded. A void has no
// outcome to report; it is not a loss, and it is not an excluded loss either.
func (db *DB) VoidPrediction(ctx context.Context, q Querier, predictionID uuid.UUID, reason string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO prediction_voids (prediction_id, reason)
		VALUES ($1,$2)
		ON CONFLICT (prediction_id) DO NOTHING`, predictionID, reason)
	if err != nil {
		return fmt.Errorf("void prediction: %w", err)
	}
	return nil
}

// PredictionsInvalidatedByReschedule finds predictions whose match moved
// *earlier*, to a kickoff that now precedes the prediction's own creation.
//
// The trigger's invariant is broken retroactively in that case: the pick was
// legitimate when made, but it is no longer true that it predates kickoff.
// Voiding it is the only honest option — it must never be papered over.
func (db *DB) PredictionsInvalidatedByReschedule(ctx context.Context, q Querier) ([]VoidablePrediction, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.match_id, m.status
		FROM predictions p
		JOIN matches m ON m.id = p.match_id
		LEFT JOIN prediction_results r ON r.prediction_id = p.id
		LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
		WHERE p.created_at >= m.kickoff_at
		  AND r.prediction_id IS NULL
		  AND v.prediction_id IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query invalidated predictions: %w", err)
	}
	defer rows.Close()

	var out []VoidablePrediction
	for rows.Next() {
		var v VoidablePrediction
		if err := rows.Scan(&v.PredictionID, &v.MatchID, &v.Status); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LedgerFilters narrows the settled-predictions ledger.
type LedgerFilters struct {
	LeagueIDs []uuid.UUID
	Markets   []domain.MarketCode
	Outcome   string // "", "all", "hit" or "miss"
	Limit     int
	Cursor    *Cursor
}

// Cursor is opaque cursor pagination over (settled_at, prediction_id).
//
// Offset pagination over a table that gains rows daily shows duplicates across
// pages: rows inserted between two requests shift everything down.
type Cursor struct {
	SettledAt time.Time
	ID        uuid.UUID
}

// SettledPredictions is the unfiltered graded ledger — every settled pick,
// wins and losses alike. The outcome filter narrows what a *reader* asked for;
// it never narrows what the accuracy figures are computed over.
func (db *DB) SettledPredictions(ctx context.Context, f LedgerFilters) ([]domain.SettledPrediction, *Cursor, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	args := []any{}
	where := []string{"TRUE"}

	if len(f.LeagueIDs) > 0 {
		args = append(args, f.LeagueIDs)
		where = append(where, fmt.Sprintf("m.league_id = ANY($%d)", len(args)))
	}
	if len(f.Markets) > 0 {
		codes := make([]string, len(f.Markets))
		for i, m := range f.Markets {
			codes[i] = string(m)
		}
		args = append(args, codes)
		where = append(where, fmt.Sprintf("p.market_code = ANY($%d)", len(args)))
	}
	switch f.Outcome {
	case "hit":
		where = append(where, "r.was_correct")
	case "miss":
		where = append(where, "NOT r.was_correct")
	}
	if f.Cursor != nil {
		args = append(args, f.Cursor.SettledAt, f.Cursor.ID)
		where = append(where, fmt.Sprintf("(r.settled_at, p.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit+1)

	rows, err := db.Pool.Query(ctx, `
		SELECT p.id, p.match_id, p.market_code, p.prediction_value,
		       p.confidence_pct::float8, p.distribution, p.model_version, p.created_at,
		       r.actual_outcome, r.was_correct, r.settled_at,
		       m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
		       m.kickoff_at, m.status, m.home_score, m.away_score, m.round
		FROM prediction_results r
		JOIN predictions p ON p.id = r.prediction_id
		JOIN matches     m ON m.id = p.match_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY r.settled_at DESC, p.id DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query settled predictions: %w", err)
	}
	defer rows.Close()

	leagues, teams, err := db.referenceMaps(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]domain.SettledPrediction, 0, limit)
	var next *Cursor
	for rows.Next() {
		var p domain.Prediction
		var distribution []byte
		var res domain.PredictionResult
		var m domain.Match

		if err := rows.Scan(&p.ID, &p.MatchID, &p.MarketCode, &p.PredictionValue,
			&p.ConfidencePct, &distribution, &p.ModelVersion, &p.CreatedAt,
			&res.ActualOutcome, &res.WasCorrect, &res.SettledAt,
			&m.ID, &m.LeagueID, &m.SeasonID, &m.HomeTeamID, &m.AwayTeamID,
			&m.KickoffAt, &m.Status, &m.HomeScore, &m.AwayScore, &m.Round); err != nil {
			return nil, nil, fmt.Errorf("scan settled prediction: %w", err)
		}
		if err := json.Unmarshal(distribution, &p.Distribution); err != nil {
			return nil, nil, fmt.Errorf("decode distribution: %w", err)
		}
		res.PredictionID = p.ID

		// The extra row proves there is another page without a second query.
		if len(out) == limit {
			next = &Cursor{SettledAt: res.SettledAt, ID: p.ID}
			break
		}

		out = append(out, domain.SettledPrediction{
			Prediction: p.ToAPI(),
			Result:     res.ToAPI(),
			Match:      m.ToAPI(),
			League:     leagues[m.LeagueID],
			HomeTeam:   teams[m.HomeTeamID],
			AwayTeam:   teams[m.AwayTeamID],
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// The cursor points at the last row *returned*, not the lookahead row.
	if next != nil && len(out) > 0 {
		last := out[len(out)-1]
		next = &Cursor{SettledAt: last.Result.SettledAt, ID: last.Prediction.ID}
	}
	return out, next, nil
}
