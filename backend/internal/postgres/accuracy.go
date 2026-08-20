package postgres

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// The accuracy dashboard is served from accuracy_rollup, not from a live
// aggregate over prediction_results. Computing a hit rate over every settled
// prediction on each page view is the one query guaranteed to get slower every
// day the product succeeds.
//
// Nothing here filters the underlying set. The API may window a *query* over
// the rollup for the timeline chart, but every settled prediction is in it —
// no exclusions, no "excluding postponed", no cherry-picked windows.

// confidenceBands mirror CONFIDENCE_BANDS in src/api/client.ts, indexed by the
// width_bucket(confidence_pct, 50, 90, 4) value the rollup stores.
var confidenceBands = []struct {
	Bucket int
	Key    string
	Label  string
}{
	{0, "lt50", "Under 50%"},
	{1, "50-60", "50–60%"},
	{2, "60-70", "60–70%"},
	{3, "70-80", "70–80%"},
	{4, "80-90", "80–90%"},
	{5, "gte90", "90%+"},
}

// RefreshRollups rebuilds the materialised views after a settlement batch.
//
// CONCURRENTLY, so the accuracy page stays readable during the refresh. It
// requires the unique indexes the migration creates, and it cannot run inside
// a transaction — hence the pool rather than a Querier.
func (db *DB) RefreshRollups(ctx context.Context) error {
	for _, view := range []string{"accuracy_rollup", "analyst_rollup"} {
		if _, err := db.Pool.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY "+view); err != nil {
			return fmt.Errorf("refresh %s: %w", view, err)
		}
	}
	return nil
}

// AccuracySummary is the public dashboard payload.
func (db *DB) AccuracySummary(ctx context.Context, modelVersion string) (domain.AccuracySummary, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT model_version, market_code, league_id, confidence_band, settled_day, total, correct
		FROM accuracy_rollup
		ORDER BY settled_day`)
	if err != nil {
		return domain.AccuracySummary{}, fmt.Errorf("query accuracy rollup: %w", err)
	}
	defer rows.Close()

	type key struct {
		total   int
		correct int
	}
	var overall key
	byMarket := map[domain.MarketCode]key{}
	byLeague := map[uuid.UUID]key{}
	byBand := map[int]key{}
	byDay := map[string]key{}

	add := func(k key, total, correct int) key {
		k.total += total
		k.correct += correct
		return k
	}

	for rows.Next() {
		var version string
		var market domain.MarketCode
		var leagueID uuid.UUID
		var band int
		var day time.Time
		var total, correct int

		if err := rows.Scan(&version, &market, &leagueID, &band, &day, &total, &correct); err != nil {
			return domain.AccuracySummary{}, fmt.Errorf("scan rollup: %w", err)
		}

		overall = add(overall, total, correct)
		byMarket[market] = add(byMarket[market], total, correct)
		byLeague[leagueID] = add(byLeague[leagueID], total, correct)
		byBand[band] = add(byBand[band], total, correct)
		byDay[day.Format(time.DateOnly)] = add(byDay[day.Format(time.DateOnly)], total, correct)
	}
	if err := rows.Err(); err != nil {
		return domain.AccuracySummary{}, err
	}

	markets, err := db.MarketTypes(ctx)
	if err != nil {
		return domain.AccuracySummary{}, err
	}
	leagues, err := db.Leagues(ctx)
	if err != nil {
		return domain.AccuracySummary{}, err
	}

	// Every list starts empty rather than nil so it serialises as [] and not
	// null. The frontend types all four as arrays and calls .map on them; a
	// null here is a crash on the accuracy page, and it would only appear
	// before the first settlement — that is, on a brand new deployment.
	summary := domain.AccuracySummary{
		Overall:          domain.NewAccuracyBucket("overall", "All predictions", overall.total, overall.correct),
		ModelVersion:     modelVersion,
		ByMarket:         []domain.AccuracyBucket{},
		ByLeague:         []domain.AccuracyBucket{},
		ByConfidenceBand: []domain.AccuracyBucket{},
		Timeline:         []domain.AccuracyPoint{},
	}

	// Empty buckets are dropped, matching the frontend's `.filter(b => b.total > 0)`:
	// a market with no settled picks is absent rather than shown at 0%.
	for _, m := range markets {
		if k, ok := byMarket[m.Code]; ok && k.total > 0 {
			summary.ByMarket = append(summary.ByMarket,
				domain.NewAccuracyBucket(string(m.Code), m.ShortName, k.total, k.correct))
		}
	}
	for _, l := range leagues {
		if k, ok := byLeague[l.ID]; ok && k.total > 0 {
			summary.ByLeague = append(summary.ByLeague,
				domain.NewAccuracyBucket(l.ID.String(), l.Name, k.total, k.correct))
		}
	}
	// The calibration bands are the ones worth watching weekly: if the 70–80%
	// band does not hit near 70%, the published confidence figures are
	// misleading users and the model needs recalibrating.
	for _, band := range confidenceBands {
		if k, ok := byBand[band.Bucket]; ok && k.total > 0 {
			summary.ByConfidenceBand = append(summary.ByConfidenceBand,
				domain.NewAccuracyBucket(band.Key, band.Label, k.total, k.correct))
		}
	}

	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)

	runningTotal, runningCorrect := 0, 0
	for _, day := range days {
		k := byDay[day]
		runningTotal += k.total
		runningCorrect += k.correct
		point := domain.AccuracyPoint{Date: day, Settled: k.total}
		if runningTotal > 0 {
			point.CumulativeHitRate = float64(runningCorrect) / float64(runningTotal)
		}
		if k.total > 0 {
			point.DailyHitRate = float64(k.correct) / float64(k.total)
		}
		summary.Timeline = append(summary.Timeline, point)
	}

	// The window the graded set covers, at full timestamp precision — the
	// rollup only carries days.
	var first, last *time.Time
	if err := db.Pool.QueryRow(ctx,
		`SELECT min(settled_at), max(settled_at) FROM prediction_results`).Scan(&first, &last); err != nil {
		return domain.AccuracySummary{}, fmt.Errorf("query settlement window: %w", err)
	}
	if first != nil {
		summary.FirstSettledAt = first.UTC().Format(time.RFC3339)
	}
	if last != nil {
		summary.LastSettledAt = last.UTC().Format(time.RFC3339)
	}

	return summary, nil
}
