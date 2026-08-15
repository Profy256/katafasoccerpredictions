package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Budget accounting -------------------------------------------------------

func (db *DB) ProviderRequestsUsed(ctx context.Context, provider string) (int, error) {
	var used int
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(requests_used, 0) FROM provider_budget
		WHERE provider = $1 AND day = (now() AT TIME ZONE 'UTC')::date`, provider).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query provider budget: %w", err)
	}
	return used, nil
}

func (db *DB) IncrementProviderBudget(ctx context.Context, provider string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO provider_budget (provider, day, requests_used)
		VALUES ($1, (now() AT TIME ZONE 'UTC')::date, 1)
		ON CONFLICT (provider, day)
		DO UPDATE SET requests_used = provider_budget.requests_used + 1`, provider)
	if err != nil {
		return fmt.Errorf("increment provider budget: %w", err)
	}
	return nil
}

func (db *DB) MarkProviderThrottled(ctx context.Context, provider string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO provider_budget (provider, day, requests_used, throttled_at)
		VALUES ($1, (now() AT TIME ZONE 'UTC')::date, 0, now())
		ON CONFLICT (provider, day) DO UPDATE SET throttled_at = now()`, provider)
	if err != nil {
		return fmt.Errorf("mark provider throttled: %w", err)
	}
	return nil
}

// ArchivePayload stores a provider response before it is parsed.
//
// Every response, success or failure. When a settlement is disputed, the
// answer is the archived response that produced it; it also makes a parser
// change replayable offline without spending request budget.
func (db *DB) ArchivePayload(ctx context.Context, provider, endpoint, requestURL string, httpStatus int, body []byte) error {
	sum := sha256.Sum256(body)

	// Only valid JSON goes in the JSONB column; an error page is still worth
	// keeping, so the hash and status are recorded either way.
	var payload any
	if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
		payload = body
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO provider_payloads (provider, endpoint, request_url, http_status, body, content_hash)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		provider, endpoint, requestURL, httpStatus, payload, sum[:])
	if err != nil {
		return fmt.Errorf("archive payload: %w", err)
	}
	return nil
}

// PrunePayloads drops archives past the 180-day retention window.
func (db *DB) PrunePayloads(ctx context.Context) (int64, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM provider_payloads WHERE fetched_at < now() - interval '180 days'`)
	if err != nil {
		return 0, fmt.Errorf("prune payloads: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Source lookups ----------------------------------------------------------

// CompetitionRoute is one league's provider assignment.
type CompetitionRoute struct {
	LeagueID              uuid.UUID
	Provider              string
	ProviderCompetitionID string
	LeagueName            string
	SeasonID              uuid.UUID
}

// CompetitionRoutes lists active leagues with their primary provider.
//
// Competitions are routed per provider rather than falling back between them:
// neither provider is a substitute for the other, and coverage is assigned
// deliberately.
func (db *DB) CompetitionRoutes(ctx context.Context, provider string) ([]CompetitionRoute, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cs.league_id, cs.provider, cs.provider_competition_id, l.name, s.id
		FROM competition_sources cs
		JOIN leagues l ON l.id = cs.league_id
		JOIN seasons s ON s.league_id = l.id AND s.is_current
		WHERE cs.is_primary
		  AND l.is_active
		  AND ($1 = '' OR cs.provider = $1)
		ORDER BY l.tier, l.name`, provider)
	if err != nil {
		return nil, fmt.Errorf("query competition routes: %w", err)
	}
	defer rows.Close()

	var out []CompetitionRoute
	for rows.Next() {
		var r CompetitionRoute
		if err := rows.Scan(&r.LeagueID, &r.Provider, &r.ProviderCompetitionID,
			&r.LeagueName, &r.SeasonID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TeamIDBySource resolves a provider's team id, creating the team if it is new.
//
// Provider ids are the join key, never team names: names differ across
// providers and across seasons ("Man United" / "Manchester United FC" /
// "Manchester Utd").
func (db *DB) TeamIDBySource(ctx context.Context, q Querier, provider, providerTeamID string, leagueID uuid.UUID, name, shortName string) (uuid.UUID, bool, error) {
	var teamID uuid.UUID
	err := q.QueryRow(ctx, `
		SELECT team_id FROM team_sources WHERE provider = $1 AND provider_team_id = $2`,
		provider, providerTeamID).Scan(&teamID)
	if err == nil {
		return teamID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("lookup team source: %w", err)
	}

	// New to this provider. It may still be a team we already have from the
	// other provider, matched on a normalised name within the same league.
	normalised := NormaliseTeamName(name)
	err = q.QueryRow(ctx, `
		SELECT id FROM teams
		WHERE league_id = $1
		  AND regexp_replace(lower(name), '[^a-z0-9]', '', 'g') = $2
		LIMIT 1`, leagueID, normalised).Scan(&teamID)

	fuzzyMatched := false
	switch {
	case err == nil:
		// Matched an existing roster entry. Logged by the caller for review:
		// a silent duplicate poisons the strength model, splitting one club's
		// history across two half-strength rows.
		fuzzyMatched = true
	case errors.Is(err, pgx.ErrNoRows):
		if shortName == "" {
			shortName = name
		}
		slug := TeamSlug(name, leagueID)
		if err := q.QueryRow(ctx, `
			INSERT INTO teams (league_id, slug, name, short_name)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, leagueID, slug, name, shortName).Scan(&teamID); err != nil {
			return uuid.Nil, false, fmt.Errorf("insert team %q: %w", name, err)
		}
	default:
		return uuid.Nil, false, fmt.Errorf("match team by name: %w", err)
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO team_sources (team_id, provider, provider_team_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (provider, provider_team_id) DO NOTHING`,
		teamID, provider, providerTeamID); err != nil {
		return uuid.Nil, false, fmt.Errorf("link team source: %w", err)
	}
	return teamID, fuzzyMatched, nil
}

// UpsertMatch inserts or updates a fixture.
//
// A finished match with a score is never overwritten. Providers do re-emit
// corrected scores, and accepting a late correction after settlement would
// change history under an already-graded prediction; corrections go through
// the admin path so somebody owns the decision.
func (db *DB) UpsertMatch(
	ctx context.Context, q Querier,
	provider, providerMatchID string,
	leagueID, seasonID, homeTeamID, awayTeamID uuid.UUID,
	kickoffAt time.Time, status domain.MatchStatus,
	homeScore, awayScore, round *int,
) (matchID uuid.UUID, created bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT match_id FROM match_sources WHERE provider = $1 AND provider_match_id = $2`,
		provider, providerMatchID).Scan(&matchID)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		finishedAt := finishedTimestamp(status, kickoffAt)
		if err := q.QueryRow(ctx, `
			INSERT INTO matches (league_id, season_id, home_team_id, away_team_id,
			                     kickoff_at, status, home_score, away_score, round, finished_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			RETURNING id`,
			leagueID, seasonID, homeTeamID, awayTeamID, kickoffAt, status,
			homeScore, awayScore, round, finishedAt).Scan(&matchID); err != nil {
			return uuid.Nil, false, fmt.Errorf("insert match: %w", err)
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO match_sources (match_id, provider, provider_match_id)
			VALUES ($1,$2,$3)
			ON CONFLICT (provider, provider_match_id) DO NOTHING`,
			matchID, provider, providerMatchID); err != nil {
			return uuid.Nil, false, fmt.Errorf("link match source: %w", err)
		}
		return matchID, true, nil

	case err != nil:
		return uuid.Nil, false, fmt.Errorf("lookup match source: %w", err)
	}

	// The guard in the WHERE clause is the important part: a finished match
	// that already carries a score is left exactly as it is.
	if _, err := q.Exec(ctx, `
		UPDATE matches
		SET kickoff_at  = $2,
		    status      = $3,
		    home_score  = $4,
		    away_score  = $5,
		    round       = COALESCE($6, round),
		    finished_at = CASE WHEN $3 = 'finished' THEN COALESCE(finished_at, now()) ELSE finished_at END
		WHERE id = $1
		  AND NOT (status = 'finished' AND home_score IS NOT NULL AND away_score IS NOT NULL)`,
		matchID, kickoffAt, status, homeScore, awayScore, round); err != nil {
		return uuid.Nil, false, fmt.Errorf("update match: %w", err)
	}
	return matchID, false, nil
}

func finishedTimestamp(status domain.MatchStatus, kickoffAt time.Time) *time.Time {
	if status != domain.StatusFinished {
		return nil
	}
	// Roughly full time. Only used for ordering and operational reporting;
	// settlement reads status and score, never this.
	t := kickoffAt.Add(105 * time.Minute)
	return &t
}

// NormaliseTeamName strips everything but lowercase alphanumerics, so
// "Manchester United FC" and "manchester-united fc" collapse together.
func NormaliseTeamName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TeamSlug builds a unique, stable slug. The league id suffix keeps two clubs
// with the same name in different countries apart.
func TeamSlug(name string, leagueID uuid.UUID) string {
	base := strings.Trim(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		case r == ' ' || r == '-' || r == '_':
			return '-'
		default:
			return -1
		}
	}, name), "-")
	if base == "" {
		base = "team"
	}
	return base + "-" + leagueID.String()[:8]
}

// IngestProviderStatus backs GET /admin/ingest/status.
type IngestProviderStatus struct {
	Provider      string     `json:"provider"`
	RequestsUsed  int        `json:"requestsUsed"`
	DailyLimit    int        `json:"dailyLimit"`
	ThrottledAt   *time.Time `json:"throttledAt,omitempty"`
	LastFetchedAt *time.Time `json:"lastFetchedAt,omitempty"`
	Leagues       int        `json:"leagues"`
	// UngradedPastKickoff is the number that matters operationally: published
	// picks on matches that kicked off long ago and still have no result.
	UngradedPastKickoff int `json:"ungradedPastKickoff"`
}

func (db *DB) IngestStatus(ctx context.Context) ([]IngestProviderStatus, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT p.provider,
		       COALESCE(b.requests_used, 0),
		       b.throttled_at,
		       (SELECT max(fetched_at) FROM provider_payloads pp WHERE pp.provider = p.provider),
		       (SELECT count(*) FROM competition_sources cs
		         WHERE cs.provider = p.provider AND cs.is_primary)
		FROM (VALUES ('football-data'), ('api-football')) AS p(provider)
		LEFT JOIN provider_budget b
		       ON b.provider = p.provider AND b.day = (now() AT TIME ZONE 'UTC')::date`)
	if err != nil {
		return nil, fmt.Errorf("query ingest status: %w", err)
	}
	defer rows.Close()

	var out []IngestProviderStatus
	for rows.Next() {
		var s IngestProviderStatus
		if err := rows.Scan(&s.Provider, &s.RequestsUsed, &s.ThrottledAt,
			&s.LastFetchedAt, &s.Leagues); err != nil {
			return nil, err
		}
		if s.Provider == "api-football" {
			s.DailyLimit = 100
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var ungraded int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM predictions p
		JOIN matches m ON m.id = p.match_id
		LEFT JOIN prediction_results r ON r.prediction_id = p.id
		LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
		WHERE r.prediction_id IS NULL
		  AND v.prediction_id IS NULL
		  AND m.kickoff_at < now() - interval '6 hours'`).Scan(&ungraded); err != nil {
		return nil, fmt.Errorf("count ungraded predictions: %w", err)
	}
	for i := range out {
		out[i].UngradedPastKickoff = ungraded
	}
	return out, nil
}
