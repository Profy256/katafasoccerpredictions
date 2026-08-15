package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Public reads. Each serves one function in src/api/client.ts and returns the
// type in src/api/types.ts verbatim, so the frontend swap is a change of
// transport only.

func (s *Server) handleLeagues(w http.ResponseWriter, r *http.Request) {
	leagues, err := s.DB.Leagues(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(leagues), render.CacheReference)
}

func (s *Server) handleMarkets(w http.ResponseWriter, r *http.Request) {
	markets, err := s.DB.MarketTypes(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(markets), render.CacheReference)
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	filters, err := parseFeedFilters(r)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	feed, err := s.DB.Feed(r.Context(), filters)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(feed), render.CacheFeed)
}

func (s *Server) handleMatchDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"), "match id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	detail, err := s.DB.MatchDetail(r.Context(), id)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, detail, render.CacheFeed)
}

func (s *Server) handleAccuracy(w http.ResponseWriter, r *http.Request) {
	summary, err := s.DB.AccuracySummary(r.Context(), s.Config.ModelVersion)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, summary, render.CacheAccuracy)
}

func (s *Server) handleSettledPredictions(w http.ResponseWriter, r *http.Request) {
	filters, err := parseLedgerFilters(r)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	rows, next, err := s.DB.SettledPredictions(r.Context(), filters)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	render.JSON(w, r, struct {
		Items      []domain.SettledPrediction `json:"items"`
		NextCursor string                     `json:"nextCursor,omitempty"`
	}{Items: orEmpty(rows), NextCursor: encodeCursor(next)}, render.CacheAccuracy)
}

func (s *Server) handleFreeTips(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var day time.Time
	if raw := r.URL.Query().Get("date"); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			render.Error(w, r, fmt.Errorf("%w: date must be YYYY-MM-DD", domain.ErrValidation), s.Log)
			return
		}
		day = parsed
	} else {
		// Omitted means the current published day. The latest frozen row is
		// the answer; there is no live selection to fall back on.
		latest, err := s.DB.LatestFreeTipsDay(ctx)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				render.JSON(w, r, domain.FreeTipsDay{
					Date: "", Groups: []domain.FreeTipGroup{}, TotalTips: 0,
				}, render.CacheFreeTips)
				return
			}
			render.Error(w, r, err, s.Log)
			return
		}
		day = latest
	}

	tipsDay, err := s.DB.FreeTips(ctx, day)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	if tipsDay.Groups == nil {
		tipsDay.Groups = []domain.FreeTipGroup{}
	}
	render.JSON(w, r, tipsDay, render.CacheFreeTips)
}

func (s *Server) handleFreeTipsHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	to := time.Now().UTC().Truncate(24 * time.Hour)
	from := to.AddDate(0, 0, -13)

	if raw := query.Get("from"); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			render.Error(w, r, fmt.Errorf("%w: from must be YYYY-MM-DD", domain.ErrValidation), s.Log)
			return
		}
		from = parsed
	}
	if raw := query.Get("to"); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			render.Error(w, r, fmt.Errorf("%w: to must be YYYY-MM-DD", domain.ErrValidation), s.Log)
			return
		}
		to = parsed
	}
	if to.Before(from) {
		render.Error(w, r, fmt.Errorf("%w: to is before from", domain.ErrValidation), s.Log)
		return
	}

	days, err := s.DB.FreeTipsHistory(r.Context(), from, to)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, struct {
		Days []domain.FreeTipsHistoryDay `json:"days"`
	}{Days: orEmpty(days)}, render.CacheReference)
}

func (s *Server) handlePackages(w http.ResponseWriter, r *http.Request) {
	packages, err := s.DB.Packages(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(packages), render.CacheReference)
}

func (s *Server) handleAnalysts(w http.ResponseWriter, r *http.Request) {
	analysts, err := s.DB.Analysts(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(analysts), render.CacheReference)
}

func (s *Server) handleAnalystLeaderboard(w http.ResponseWriter, r *http.Request) {
	records, err := s.DB.AnalystRecords(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, orEmpty(records), render.CacheAccuracy)
}

func (s *Server) handleAnalystRecord(w http.ResponseWriter, r *http.Request) {
	record, err := s.DB.AnalystRecord(r.Context(), r.PathValue("slug"))
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, record, render.CacheAccuracy)
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	stats, err := s.DB.CoverageStats(r.Context(), s.Config.ModelVersion)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, stats, render.CacheStats)
}

/* ---------------------------------------------------------------- *
 * Query parameter parsing
 *
 * Multi-value parameters are comma-separated, not repeated keys, and an
 * unknown value is a 400 rather than a silent skip: a typo'd market code
 * returning the unfiltered feed is worse than an error.
 * ---------------------------------------------------------------- */

func parseFeedFilters(r *http.Request) (postgres.FeedFilters, error) {
	query := r.URL.Query()
	var f postgres.FeedFilters

	leagueIDs, err := parseUUIDList(query.Get("leagues"), "league id")
	if err != nil {
		return f, err
	}
	f.LeagueIDs = leagueIDs

	markets, err := parseMarketList(query.Get("markets"))
	if err != nil {
		return f, err
	}
	f.Markets = markets

	if raw := query.Get("min_confidence"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 || value > 100 {
			return f, fmt.Errorf("%w: min_confidence must be between 0 and 100", domain.ErrValidation)
		}
		f.MinConfidence = value
	}

	if raw := query.Get("region"); raw != "" && raw != "all" {
		region, err := domain.ParseRegion(raw)
		if err != nil {
			return f, fmt.Errorf("%w: %s", domain.ErrValidation, err)
		}
		f.Region = string(region)
	}
	return f, nil
}

func parseLedgerFilters(r *http.Request) (postgres.LedgerFilters, error) {
	query := r.URL.Query()
	var f postgres.LedgerFilters

	leagueIDs, err := parseUUIDList(query.Get("leagues"), "league id")
	if err != nil {
		return f, err
	}
	f.LeagueIDs = leagueIDs

	markets, err := parseMarketList(query.Get("markets"))
	if err != nil {
		return f, err
	}
	f.Markets = markets

	switch outcome := query.Get("outcome"); outcome {
	case "", "all", "hit", "miss":
		f.Outcome = outcome
	default:
		return f, fmt.Errorf("%w: outcome must be all, hit or miss", domain.ErrValidation)
	}

	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return f, fmt.Errorf("%w: limit must be a positive integer", domain.ErrValidation)
		}
		f.Limit = limit
	}

	if raw := query.Get("cursor"); raw != "" {
		cursor, err := decodeCursor(raw)
		if err != nil {
			return f, err
		}
		f.Cursor = cursor
	}
	return f, nil
}

func parseMarketList(raw string) ([]domain.MarketCode, error) {
	if raw == "" {
		return nil, nil
	}
	var out []domain.MarketCode
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		market, err := domain.ParseMarketCode(part)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", domain.ErrValidation, err)
		}
		out = append(out, market)
	}
	return out, nil
}

func parseUUIDList(raw, label string) ([]uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	var out []uuid.UUID
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := parseUUID(part, label)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func parseUUID(raw, label string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %q is not a valid %s", domain.ErrValidation, raw, label)
	}
	return id, nil
}

// orEmpty makes a nil slice serialise as [] rather than null. The frontend
// types every list as an array, and `null.map` is a crash.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
