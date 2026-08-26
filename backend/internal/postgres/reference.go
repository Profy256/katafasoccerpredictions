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

// Leagues returns every league the public site may show.
//
// Gated on is_published, matching Feed. Without that the two disagreed, and
// the disagreement was load-bearing: the frontend builds its footer crawl
// spine and its sitemap from this list, so a shadow-mode league got a public
// landing page announcing "a published prediction on every upcoming fixture"
// over a feed that returns nothing for it. A thin page linked from every page
// on the site is a worse outcome than no page.
//
// This is the same rule as the paywall — the boundary is the query, not a
// filter applied after fetching.
func (db *DB) Leagues(ctx context.Context) ([]domain.League, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, short_name, country, country_code, tier, region, slug, is_active
		FROM leagues
		WHERE is_active AND is_published
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

/* ------------------------------------------------------------------ *
 * The publication gate
 * ------------------------------------------------------------------ */

// LeagueReadiness is one league's publication status alongside the evidence
// for whether it deserves it.
//
// Upcoming/Ready are counted with the *same* predicate the publish job uses
// (FreeTipCandidates), so the readiness figure is not a second opinion about
// eligibility — it is a dry run of the real one. A league can be published and
// still contribute nothing, which is exactly the state this exists to make
// visible: without it, "no tips today" and "this league is in shadow mode"
// look identical from outside.
type LeagueReadiness struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Country     string
	Region      string
	IsActive    bool
	IsPublished bool
	// Upcoming scheduled fixtures, and how many clear the sample-size floor.
	Upcoming int
	Ready    int
	// Teams in the league, and how many have less finished history than the
	// floor. TeamsShort > 0 is why Ready is low.
	Teams      int
	TeamsShort int
	// Fewest finished matches held by any team in the league.
	MinPlayed int
}

// LeagueReadinessAll reports every league against the publication gate.
//
// minSample is model.MinHistoryPerTeam, passed in rather than imported so this
// package keeps knowing nothing about the model.
func (db *DB) LeagueReadinessAll(ctx context.Context, minSample int) ([]LeagueReadiness, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH played AS (
		    SELECT t.id AS team_id, t.league_id, count(m.id) AS matches
		    FROM teams t
		    LEFT JOIN matches m
		           ON (m.home_team_id = t.id OR m.away_team_id = t.id)
		          AND m.status = 'finished'
		    GROUP BY t.id, t.league_id
		),
		team_stats AS (
		    SELECT league_id,
		           count(*)                                   AS teams,
		           count(*) FILTER (WHERE matches < $1)        AS teams_short,
		           coalesce(min(matches), 0)                   AS min_played
		    FROM played
		    GROUP BY league_id
		),
		fixture_stats AS (
		    SELECT m.league_id,
		           count(*) AS upcoming,
		           count(*) FILTER (
		               WHERE mr.sample_home >= $1 AND mr.sample_away >= $1
		           ) AS ready
		    FROM matches m
		    LEFT JOIN match_reasoning mr ON mr.match_id = m.id
		    WHERE m.status = 'scheduled' AND m.kickoff_at > now()
		    GROUP BY m.league_id
		)
		SELECT l.id, l.slug, l.name, l.country, l.region, l.is_active, l.is_published,
		       coalesce(f.upcoming, 0), coalesce(f.ready, 0),
		       coalesce(s.teams, 0), coalesce(s.teams_short, 0), coalesce(s.min_played, 0)
		FROM leagues l
		LEFT JOIN team_stats    s ON s.league_id = l.id
		LEFT JOIN fixture_stats f ON f.league_id = l.id
		ORDER BY l.is_published DESC, coalesce(f.ready, 0) DESC, l.tier, l.country, l.name`,
		minSample)
	if err != nil {
		return nil, fmt.Errorf("query league readiness: %w", err)
	}
	defer rows.Close()

	var out []LeagueReadiness
	for rows.Next() {
		var r LeagueReadiness
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Country, &r.Region,
			&r.IsActive, &r.IsPublished, &r.Upcoming, &r.Ready,
			&r.Teams, &r.TeamsShort, &r.MinPlayed); err != nil {
			return nil, fmt.Errorf("scan league readiness: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LeagueReadinessBySlug reports one league. Returns ErrNotFound when the slug
// is unknown, so a typo'd slug is an error rather than a silent no-op.
func (db *DB) LeagueReadinessBySlug(ctx context.Context, slug string, minSample int) (LeagueReadiness, error) {
	all, err := db.LeagueReadinessAll(ctx, minSample)
	if err != nil {
		return LeagueReadiness{}, err
	}
	for _, r := range all {
		if r.Slug == slug {
			return r, nil
		}
	}
	return LeagueReadiness{}, domain.ErrNotFound
}

// SetLeaguePublished flips the publication gate and returns whether the value
// actually changed.
//
// Takes a Querier so the flip and its audit_log entry commit together: a
// league that went live with no record of who cleared it is precisely the gap
// this command exists to close.
func (db *DB) SetLeaguePublished(ctx context.Context, q Querier, id uuid.UUID, published bool) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE leagues SET is_published = $2
		WHERE id = $1 AND is_published IS DISTINCT FROM $2`, id, published)
	if err != nil {
		return false, fmt.Errorf("set league published: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
