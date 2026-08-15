package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Reference data: leagues, teams, markets, packages, analysts. All of it is
// small, slow-changing, and cacheable for an hour at the HTTP layer.

// Leagues returns every active league, ordered for display.
func (db *DB) Leagues(ctx context.Context) ([]domain.League, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, short_name, country, country_code, tier, region, slug, is_active
		FROM leagues
		WHERE is_active
		ORDER BY tier, country, name`)
	if err != nil {
		return nil, fmt.Errorf("query leagues: %w", err)
	}
	defer rows.Close()

	var out []domain.League
	for rows.Next() {
		var l domain.League
		if err := rows.Scan(&l.ID, &l.Name, &l.ShortName, &l.Country, &l.CountryCode,
			&l.Tier, &l.Region, &l.Slug, &l.IsActive); err != nil {
			return nil, fmt.Errorf("scan league: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// PublishedLeagueIDs are the leagues cleared to publish tips.
//
// A league with too little history runs in shadow mode: predictions are
// generated and settled internally, nothing is published, until the
// calibration chart says the model is sane there.
func (db *DB) PublishedLeagueIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM leagues WHERE is_active AND is_published`)
	if err != nil {
		return nil, fmt.Errorf("query published leagues: %w", err)
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (db *DB) Teams(ctx context.Context) ([]domain.Team, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, league_id, slug, name, short_name FROM teams ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query teams: %w", err)
	}
	defer rows.Close()

	var out []domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.LeagueID, &t.Slug, &t.Name, &t.ShortName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarketTypes returns market metadata in display order.
func (db *DB) MarketTypes(ctx context.Context) ([]domain.MarketType, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT code, display_name, short_name, tab_label, slug, outcomes
		FROM market_types ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("query market types: %w", err)
	}
	defer rows.Close()

	var out []domain.MarketType
	for rows.Next() {
		var m domain.MarketType
		var outcomes []byte
		if err := rows.Scan(&m.Code, &m.DisplayName, &m.ShortName, &m.TabLabel, &m.Slug, &outcomes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(outcomes, &m.Outcomes); err != nil {
			return nil, fmt.Errorf("decode outcomes for %s: %w", m.Code, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (db *DB) Packages(ctx context.Context) ([]domain.Package, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT code, name, tagline, description, typical_price_ugx, highlights
		FROM packages ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("query packages: %w", err)
	}
	defer rows.Close()

	var out []domain.Package
	for rows.Next() {
		var p domain.Package
		var highlights []byte
		var price int64
		if err := rows.Scan(&p.Code, &p.Name, &p.Tagline, &p.Description, &price, &highlights); err != nil {
			return nil, err
		}
		p.TypicalPriceUGX = domain.UGX(price)
		if err := json.Unmarshal(highlights, &p.Highlights); err != nil {
			return nil, fmt.Errorf("decode highlights for %s: %w", p.Code, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Analysts returns every active analyst, with the packages they have actually
// published slips in — derived rather than stored, so it cannot drift from the
// record.
func (db *DB) Analysts(ctx context.Context) ([]domain.Analyst, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.slug, a.name, a.handle, a.initials, a.bio, a.joined_at,
		       COALESCE(
		         array_agg(DISTINCT s.package_code) FILTER (WHERE s.id IS NOT NULL),
		         '{}'
		       ) AS packages
		FROM analysts a
		LEFT JOIN tips  t ON t.analyst_id = a.id
		LEFT JOIN slips s ON s.id = t.slip_id AND s.status <> 'draft'
		WHERE a.is_active
		GROUP BY a.id
		ORDER BY a.name`)
	if err != nil {
		return nil, fmt.Errorf("query analysts: %w", err)
	}
	defer rows.Close()

	var out []domain.Analyst
	for rows.Next() {
		var a domain.Analyst
		var packages []string
		if err := rows.Scan(&a.ID, &a.Slug, &a.Name, &a.Handle, &a.Initials,
			&a.Bio, &a.JoinedAt, &packages); err != nil {
			return nil, err
		}
		for _, p := range packages {
			a.Packages = append(a.Packages, domain.PackageCode(p))
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) AnalystBySlug(ctx context.Context, slug string) (domain.Analyst, error) {
	all, err := db.Analysts(ctx)
	if err != nil {
		return domain.Analyst{}, err
	}
	for _, a := range all {
		if a.Slug == slug {
			return a, nil
		}
	}
	return domain.Analyst{}, domain.ErrNotFound
}

// CoverageStats backs the landing strip counts.
type CoverageStats struct {
	Leagues           int    `json:"leagues"`
	Teams             int    `json:"teams"`
	UpcomingFixtures  int    `json:"upcomingFixtures"`
	LivePredictions   int    `json:"livePredictions"`
	GradedPredictions int    `json:"gradedPredictions"`
	ModelVersion      string `json:"modelVersion"`
}

func (db *DB) CoverageStats(ctx context.Context, modelVersion string) (CoverageStats, error) {
	var s CoverageStats
	err := db.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM leagues WHERE is_active),
		  (SELECT count(*) FROM teams),
		  (SELECT count(*) FROM matches WHERE status IN ('scheduled','in_play')),
		  -- Live predictions are those still awaiting a result. A voided pick is
		  -- neither live nor graded, so it is in neither figure.
		  (SELECT count(*) FROM predictions p
		     LEFT JOIN prediction_results r ON r.prediction_id = p.id
		     LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
		    WHERE r.prediction_id IS NULL AND v.prediction_id IS NULL),
		  (SELECT count(*) FROM prediction_results)`).
		Scan(&s.Leagues, &s.Teams, &s.UpcomingFixtures, &s.LivePredictions, &s.GradedPredictions)
	if err != nil {
		return CoverageStats{}, fmt.Errorf("coverage stats: %w", err)
	}
	s.ModelVersion = modelVersion
	return s, nil
}
