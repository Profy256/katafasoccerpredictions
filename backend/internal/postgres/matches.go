package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/strength"
)

// FeedFilters mirrors FeedFilters in src/api/types.ts. An empty value means no
// filter; an *unknown* value is rejected at the HTTP layer with a 400 rather
// than silently ignored, because a typo'd market code returning the unfiltered
// feed is worse than an error.
type FeedFilters struct {
	LeagueIDs     []uuid.UUID
	Markets       []domain.MarketCode
	MinConfidence float64
	Region        string // "" or "all" means every region
}

// Feed returns upcoming fixtures with their published predictions.
//
// Only leagues cleared for publication appear: a league still accumulating
// history runs in shadow mode, its predictions generated and settled
// internally but never shown.
func (db *DB) Feed(ctx context.Context, f FeedFilters) ([]domain.MatchWithPredictions, error) {
	args := []any{}
	where := []string{
		"m.status IN ('scheduled','in_play')",
		"l.is_active",
		"l.is_published",
	}

	if len(f.LeagueIDs) > 0 {
		args = append(args, f.LeagueIDs)
		where = append(where, fmt.Sprintf("m.league_id = ANY($%d)", len(args)))
	}
	if f.Region != "" && f.Region != "all" {
		args = append(args, f.Region)
		where = append(where, fmt.Sprintf("l.region = $%d", len(args)))
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
		       m.kickoff_at, m.status, m.home_score, m.away_score, m.round
		FROM matches m
		JOIN leagues l ON l.id = m.league_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY m.kickoff_at, m.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query feed matches: %w", err)
	}

	matches, err := scanMatches(rows)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return []domain.MatchWithPredictions{}, nil
	}

	ids := make([]uuid.UUID, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	predictions, err := db.predictionsForMatches(ctx, ids, f.Markets, f.MinConfidence)
	if err != nil {
		return nil, err
	}

	leagues, teams, err := db.referenceMaps(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]domain.MatchWithPredictions, 0, len(matches))
	for _, m := range matches {
		picks := predictions[m.ID]
		// A fixture with nothing left to show after filtering is not a result.
		if len(picks) == 0 {
			continue
		}
		row, err := assemble(m, picks, leagues, teams)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// MatchDetail is one fixture with its predictions, any results, and the
// reasoning snapshot taken when the model decided.
func (db *DB) MatchDetail(ctx context.Context, matchID uuid.UUID) (domain.MatchDetail, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
		       m.kickoff_at, m.status, m.home_score, m.away_score, m.round
		FROM matches m WHERE m.id = $1`, matchID)
	if err != nil {
		return domain.MatchDetail{}, fmt.Errorf("query match: %w", err)
	}
	matches, err := scanMatches(rows)
	if err != nil {
		return domain.MatchDetail{}, err
	}
	if len(matches) == 0 {
		return domain.MatchDetail{}, domain.ErrNotFound
	}
	match := matches[0]

	predictions, err := db.predictionsForMatches(ctx, []uuid.UUID{matchID}, nil, 0)
	if err != nil {
		return domain.MatchDetail{}, err
	}
	leagues, teams, err := db.referenceMaps(ctx)
	if err != nil {
		return domain.MatchDetail{}, err
	}
	base, err := assemble(match, predictions[matchID], leagues, teams)
	if err != nil {
		return domain.MatchDetail{}, err
	}

	results, err := db.resultsForMatch(ctx, matchID)
	if err != nil {
		return domain.MatchDetail{}, err
	}
	base.Results = results

	reasoning, err := db.MatchReasoning(ctx, matchID)
	if err != nil {
		// The detail page exists to show *why* a pick was made. Without the
		// snapshot there is nothing to show, and recomputing it now would show
		// a different, flattering picture.
		return domain.MatchDetail{}, err
	}

	return domain.MatchDetail{MatchWithPredictions: base, Reasoning: reasoning}, nil
}

func (db *DB) MatchReasoning(ctx context.Context, matchID uuid.UUID) (domain.MatchReasoning, error) {
	var r domain.MatchReasoning
	var homeForm, awayForm, h2h, scorelines []byte

	err := db.Pool.QueryRow(ctx, `
		SELECT match_id, xg_home::float8, xg_away::float8, home_form, away_form,
		       head_to_head, top_scorelines, sample_home, sample_away, model_version
		FROM match_reasoning WHERE match_id = $1`, matchID).
		Scan(&r.MatchID, &r.XGHome, &r.XGAway, &homeForm, &awayForm,
			&h2h, &scorelines, &r.SampleSize.Home, &r.SampleSize.Away, &r.ModelVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MatchReasoning{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.MatchReasoning{}, fmt.Errorf("query reasoning: %w", err)
	}

	for _, d := range []struct {
		raw  []byte
		into any
		name string
	}{
		{homeForm, &r.HomeForm, "home_form"},
		{awayForm, &r.AwayForm, "away_form"},
		{h2h, &r.HeadToHead, "head_to_head"},
		{scorelines, &r.TopScorelines, "top_scorelines"},
	} {
		if err := json.Unmarshal(d.raw, d.into); err != nil {
			return domain.MatchReasoning{}, fmt.Errorf("decode %s: %w", d.name, err)
		}
	}
	return r, nil
}

// LeagueHistory returns a league's finished matches that kicked off strictly
// before `before`, oldest first.
//
// The `before` bound is the walk-forward rule expressed in SQL: a prediction
// for match M may use only results from matches that kicked off before M. The
// model asserts it again on the way in, because a query that is correct today
// can be edited tomorrow.
func (db *DB) LeagueHistory(ctx context.Context, leagueID uuid.UUID, before time.Time) ([]strength.PlayedMatch, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, home_team_id, away_team_id, home_score, away_score, kickoff_at
		FROM matches
		WHERE league_id = $1
		  AND status = 'finished'
		  AND kickoff_at < $2
		ORDER BY kickoff_at, id`, leagueID, before)
	if err != nil {
		return nil, fmt.Errorf("query league history: %w", err)
	}
	defer rows.Close()

	var out []strength.PlayedMatch
	for rows.Next() {
		var m strength.PlayedMatch
		if err := rows.Scan(&m.MatchID, &m.HomeTeamID, &m.AwayTeamID,
			&m.HomeScore, &m.AwayScore, &m.KickoffAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpcomingFixtures are matches kicking off within the window that have no
// prediction at this model version yet.
func (db *DB) UpcomingFixtures(ctx context.Context, within time.Duration, modelVersion string) ([]domain.Match, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
		       m.kickoff_at, m.status, m.home_score, m.away_score, m.round
		FROM matches m
		JOIN leagues l ON l.id = m.league_id
		WHERE m.status = 'scheduled'
		  AND l.is_active
		  AND m.kickoff_at > now()
		  AND m.kickoff_at <= now() + $1::interval
		  AND NOT EXISTS (
		        SELECT 1 FROM predictions p
		        WHERE p.match_id = m.id AND p.model_version = $2)
		ORDER BY m.kickoff_at, m.id`,
		fmt.Sprintf("%d seconds", int(within.Seconds())), modelVersion)
	if err != nil {
		return nil, fmt.Errorf("query upcoming fixtures: %w", err)
	}
	return scanMatches(rows)
}

// MatchByID reads one match.
func (db *DB) MatchByID(ctx context.Context, q Querier, id uuid.UUID) (domain.Match, error) {
	rows, err := q.Query(ctx, `
		SELECT id, league_id, season_id, home_team_id, away_team_id,
		       kickoff_at, status, home_score, away_score, round
		FROM matches WHERE id = $1`, id)
	if err != nil {
		return domain.Match{}, fmt.Errorf("query match: %w", err)
	}
	matches, err := scanMatches(rows)
	if err != nil {
		return domain.Match{}, err
	}
	if len(matches) == 0 {
		return domain.Match{}, domain.ErrNotFound
	}
	return matches[0], nil
}

func scanMatches(rows pgx.Rows) ([]domain.Match, error) {
	defer rows.Close()
	var out []domain.Match
	for rows.Next() {
		var m domain.Match
		if err := rows.Scan(&m.ID, &m.LeagueID, &m.SeasonID, &m.HomeTeamID, &m.AwayTeamID,
			&m.KickoffAt, &m.Status, &m.HomeScore, &m.AwayScore, &m.Round); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// referenceMaps loads leagues and teams keyed by id. Both tables are small and
// slow-changing, so one round trip each beats joining them onto every row.
func (db *DB) referenceMaps(ctx context.Context) (map[uuid.UUID]domain.League, map[uuid.UUID]domain.Team, error) {
	leagues, err := db.Leagues(ctx)
	if err != nil {
		return nil, nil, err
	}
	teams, err := db.Teams(ctx)
	if err != nil {
		return nil, nil, err
	}

	leagueByID := make(map[uuid.UUID]domain.League, len(leagues))
	for _, l := range leagues {
		leagueByID[l.ID] = l
	}
	teamByID := make(map[uuid.UUID]domain.Team, len(teams))
	for _, t := range teams {
		teamByID[t.ID] = t
	}
	return leagueByID, teamByID, nil
}

func assemble(
	m domain.Match,
	predictions []domain.Prediction,
	leagues map[uuid.UUID]domain.League,
	teams map[uuid.UUID]domain.Team,
) (domain.MatchWithPredictions, error) {
	league, ok := leagues[m.LeagueID]
	if !ok {
		return domain.MatchWithPredictions{}, fmt.Errorf("match %s references unknown league %s", m.ID, m.LeagueID)
	}
	home, ok := teams[m.HomeTeamID]
	if !ok {
		return domain.MatchWithPredictions{}, fmt.Errorf("match %s references unknown team %s", m.ID, m.HomeTeamID)
	}
	away, ok := teams[m.AwayTeamID]
	if !ok {
		return domain.MatchWithPredictions{}, fmt.Errorf("match %s references unknown team %s", m.ID, m.AwayTeamID)
	}

	picks := make([]domain.APIPrediction, 0, len(predictions))
	for _, p := range predictions {
		picks = append(picks, p.ToAPI())
	}

	return domain.MatchWithPredictions{
		Match:       m.ToAPI(),
		League:      league,
		HomeTeam:    home,
		AwayTeam:    away,
		Predictions: picks,
	}, nil
}
