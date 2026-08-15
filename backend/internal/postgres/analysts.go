package postgres

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// An analyst's record is computed over every settled tip, with no exclusions.
// Void legs are excluded from the *rollup* because a void has no outcome to
// report — that is not the same as dropping a loss, which would be.

// AnalystRecords builds every active analyst's record, best hit rate first.
func (db *DB) AnalystRecords(ctx context.Context) ([]domain.AnalystRecord, error) {
	analysts, err := db.Analysts(ctx)
	if err != nil {
		return nil, err
	}
	if len(analysts) == 0 {
		return []domain.AnalystRecord{}, nil
	}

	packages, err := db.Packages(ctx)
	if err != nil {
		return nil, err
	}
	packageName := make(map[domain.PackageCode]string, len(packages))
	for _, p := range packages {
		packageName[p.Code] = p.Name
	}

	// Read the settled tips directly rather than the rollup: the record needs
	// per-tip odds for the flat-stake return, and the analyst set is small.
	rows, err := db.Pool.Query(ctx, `
		SELECT t.analyst_id, s.package_code, r.was_correct, r.settled_at, t.odds::float8
		FROM tip_results r
		JOIN tips  t ON t.id = r.tip_id
		JOIN slips s ON s.id = t.slip_id
		WHERE r.actual_outcome <> $1`, domain.VoidOutcome)
	if err != nil {
		return nil, fmt.Errorf("query analyst tips: %w", err)
	}
	defer rows.Close()

	type tally struct {
		total, correct int
		profitUnits    float64
		oddsSum        float64
	}
	overall := map[uuid.UUID]*tally{}
	last30 := map[uuid.UUID]*tally{}
	byPackage := map[uuid.UUID]map[domain.PackageCode]*tally{}

	get := func(m map[uuid.UUID]*tally, id uuid.UUID) *tally {
		if m[id] == nil {
			m[id] = &tally{}
		}
		return m[id]
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -30)

	for rows.Next() {
		var analystID uuid.UUID
		var pkg domain.PackageCode
		var wasCorrect bool
		var settledAt time.Time
		var odds float64

		if err := rows.Scan(&analystID, &pkg, &wasCorrect, &settledAt, &odds); err != nil {
			return nil, fmt.Errorf("scan analyst tip: %w", err)
		}

		o := get(overall, analystID)
		o.total++
		o.oddsSum += odds
		if wasCorrect {
			o.correct++
			// Flat 1-unit stakes: a winner returns its odds less the stake, a
			// loser -1.
			o.profitUnits += odds - 1
		} else {
			o.profitUnits--
		}

		if settledAt.After(cutoff) {
			r := get(last30, analystID)
			r.total++
			if wasCorrect {
				r.correct++
			}
		}

		if byPackage[analystID] == nil {
			byPackage[analystID] = map[domain.PackageCode]*tally{}
		}
		if byPackage[analystID][pkg] == nil {
			byPackage[analystID][pkg] = &tally{}
		}
		p := byPackage[analystID][pkg]
		p.total++
		if wasCorrect {
			p.correct++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]domain.AnalystRecord, 0, len(analysts))
	for _, a := range analysts {
		o := overall[a.ID]
		if o == nil {
			o = &tally{}
		}
		r := last30[a.ID]
		if r == nil {
			r = &tally{}
		}

		record := domain.AnalystRecord{
			Analyst:     a,
			Overall:     domain.NewAccuracyBucket("overall", "All tips", o.total, o.correct),
			Last30Days:  domain.NewAccuracyBucket("last30", "Last 30 days", r.total, r.correct),
			ByPackage:   []domain.AccuracyBucket{},
			ProfitUnits: round2(o.profitUnits),
		}
		if o.total > 0 {
			record.ROI = o.profitUnits / float64(o.total)
			record.AverageOdds = o.oddsSum / float64(o.total)
		}

		for _, code := range domain.PackageCodes {
			if t := byPackage[a.ID][code]; t != nil && t.total > 0 {
				record.ByPackage = append(record.ByPackage,
					domain.NewAccuracyBucket(string(code), packageName[code], t.total, t.correct))
			}
		}

		slips, _, err := db.Slips(ctx, SlipQuery{AnalystID: &a.ID, Limit: 6})
		if err != nil {
			return nil, err
		}
		record.RecentSlips = slips
		out = append(out, record)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Overall.HitRate > out[j].Overall.HitRate
	})
	return out, nil
}

// AnalystRecord is one analyst by slug.
func (db *DB) AnalystRecord(ctx context.Context, slug string) (domain.AnalystRecord, error) {
	all, err := db.AnalystRecords(ctx)
	if err != nil {
		return domain.AnalystRecord{}, err
	}
	for _, r := range all {
		if r.Analyst.Slug == slug {
			return r, nil
		}
	}
	return domain.AnalystRecord{}, domain.ErrNotFound
}

// round2 matches the frontend's Math.round(profitUnits * 100) / 100, so the
// figure shown does not change at cutover.
func round2(v float64) float64 {
	rounded := float64(int64(v*100 + copySign(0.5, v)))
	return rounded / 100
}

func copySign(magnitude, sign float64) float64 {
	if sign < 0 {
		return -magnitude
	}
	return magnitude
}
