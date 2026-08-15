// Package footballdata is the football-data.org v4 client.
//
// Free plan: 10 requests/minute, no daily cap, 12 competitions across Europe
// plus the Brasileirão. Better data quality than the alternative, and the
// per-minute rather than per-day limit makes bulk backfills practical.
package footballdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest"
)

const BaseURL = "https://api.football-data.org/v4"

type Client struct {
	BaseURL string
	Token   string
	fetcher *ingest.Fetcher
}

func New(token string, archive ingest.Archiver) *Client {
	c := &Client{BaseURL: BaseURL, Token: token}
	c.fetcher = &ingest.Fetcher{
		Provider: ingest.ProviderFootballData,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Archive:  archive,
		Header: func(r *http.Request) {
			r.Header.Set("X-Auth-Token", token)
		},
	}
	return c
}

func (c *Client) Name() string { return ingest.ProviderFootballData }

type competitionsResponse struct {
	Competitions []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Area struct {
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"area"`
		CurrentSeason struct {
			StartDate string `json:"startDate"`
		} `json:"currentSeason"`
	} `json:"competitions"`
}

func (c *Client) Competitions(ctx context.Context) ([]ingest.RawCompetition, error) {
	body, err := c.fetcher.Get(ctx, "competitions", c.BaseURL+"/competitions")
	if err != nil {
		return nil, err
	}
	var decoded competitionsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("football-data: decode competitions: %w", err)
	}

	out := make([]ingest.RawCompetition, 0, len(decoded.Competitions))
	for _, comp := range decoded.Competitions {
		season := 0
		if len(comp.CurrentSeason.StartDate) >= 4 {
			if t, err := time.Parse("2006-01-02", comp.CurrentSeason.StartDate); err == nil {
				season = t.Year()
			}
		}
		out = append(out, ingest.RawCompetition{
			ProviderID:  fmt.Sprint(comp.ID),
			Name:        comp.Name,
			Country:     comp.Area.Name,
			CountryCode: comp.Area.Code,
			Season:      season,
		})
	}
	return out, nil
}

type teamsResponse struct {
	Teams []struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		ShortName string `json:"shortName"`
		TLA       string `json:"tla"`
	} `json:"teams"`
}

func (c *Client) Teams(ctx context.Context, competition string, season int) ([]ingest.RawTeam, error) {
	url := fmt.Sprintf("%s/competitions/%s/teams", c.BaseURL, competition)
	if season > 0 {
		url += fmt.Sprintf("?season=%d", season)
	}
	body, err := c.fetcher.Get(ctx, "competition-teams", url)
	if err != nil {
		return nil, err
	}
	var decoded teamsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("football-data: decode teams: %w", err)
	}

	out := make([]ingest.RawTeam, 0, len(decoded.Teams))
	for _, t := range decoded.Teams {
		short := t.ShortName
		if short == "" {
			short = t.TLA
		}
		out = append(out, ingest.RawTeam{
			ProviderID: fmt.Sprint(t.ID),
			Name:       t.Name,
			ShortName:  short,
		})
	}
	return out, nil
}

type matchesResponse struct {
	Matches []struct {
		ID       int    `json:"id"`
		UTCDate  string `json:"utcDate"`
		Status   string `json:"status"`
		Matchday int    `json:"matchday"`
		Season   struct {
			StartDate string `json:"startDate"`
		} `json:"season"`
		HomeTeam struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"homeTeam"`
		AwayTeam struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"awayTeam"`
		Score struct {
			Winner   string `json:"winner"`
			Duration string `json:"duration"`
			FullTime struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"fullTime"`
			// RegularTime is present on knockout ties that went to extra time.
			// When it is, it — not fullTime — is the 90-minute score.
			RegularTime struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"regularTime"`
		} `json:"score"`
	} `json:"matches"`
}

func (c *Client) Fixtures(ctx context.Context, competition string, from, to time.Time) ([]ingest.RawFixture, error) {
	return c.matches(ctx, "competition-matches", competition, from, to, "")
}

func (c *Client) Results(ctx context.Context, competition string, from, to time.Time) ([]ingest.RawFixture, error) {
	return c.matches(ctx, "competition-results", competition, from, to, "FINISHED")
}

func (c *Client) matches(ctx context.Context, endpoint, competition string, from, to time.Time, status string) ([]ingest.RawFixture, error) {
	url := fmt.Sprintf("%s/competitions/%s/matches?dateFrom=%s&dateTo=%s",
		c.BaseURL, competition, from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))
	if status != "" {
		url += "&status=" + status
	}

	body, err := c.fetcher.Get(ctx, endpoint, url)
	if err != nil {
		return nil, err
	}
	var decoded matchesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("football-data: decode matches: %w", err)
	}

	out := make([]ingest.RawFixture, 0, len(decoded.Matches))
	for _, m := range decoded.Matches {
		kickoff, err := time.Parse(time.RFC3339, m.UTCDate)
		if err != nil {
			return nil, fmt.Errorf("football-data: match %d has unparseable date %q: %w",
				m.ID, m.UTCDate, err)
		}

		mapped, err := mapStatus(m.Status)
		if err != nil {
			return nil, fmt.Errorf("football-data: match %d: %w", m.ID, err)
		}

		// Take the 90-minute figure. On a knockout tie that went to extra
		// time, fullTime includes it and regularTime is the 90-minute score.
		homeScore, awayScore := m.Score.FullTime.Home, m.Score.FullTime.Away
		if m.Score.RegularTime.Home != nil && m.Score.RegularTime.Away != nil {
			homeScore, awayScore = m.Score.RegularTime.Home, m.Score.RegularTime.Away
		}
		// The schema ties status and score together, so a finished match
		// without one is not finished as far as this system is concerned.
		if mapped != domain.StatusFinished || homeScore == nil || awayScore == nil {
			if mapped == domain.StatusFinished {
				mapped = domain.StatusScheduled
			}
			homeScore, awayScore = nil, nil
		}

		var round *int
		if m.Matchday > 0 {
			round = &m.Matchday
		}

		season := 0
		if t, err := time.Parse("2006-01-02", m.Season.StartDate); err == nil {
			season = t.Year()
		}

		out = append(out, ingest.RawFixture{
			ProviderID:     fmt.Sprint(m.ID),
			CompetitionID:  competition,
			HomeName:       m.HomeTeam.Name,
			AwayName:       m.AwayTeam.Name,
			HomeProviderID: providerID(m.HomeTeam.ID),
			AwayProviderID: providerID(m.AwayTeam.ID),
			KickoffAt:      kickoff.UTC(),
			Status:         mapped,
			HomeScore:      homeScore,
			AwayScore:      awayScore,
			Round:          round,
			Season:         season,
		})
	}
	return out, nil
}

func providerID(id int) string {
	if id == 0 {
		return ""
	}
	return fmt.Sprint(id)
}

// mapStatus translates football-data's vocabulary.
//
// An unrecognised status is an error, never a silent fall-through to
// 'scheduled': that would resurrect abandoned matches and leave finished ones
// permanently ungraded.
func mapStatus(status string) (domain.MatchStatus, error) {
	switch status {
	case "SCHEDULED", "TIMED":
		return domain.StatusScheduled, nil
	case "IN_PLAY", "PAUSED":
		return domain.StatusInPlay, nil
	case "FINISHED", "AWARDED":
		return domain.StatusFinished, nil
	case "POSTPONED":
		return domain.StatusPostponed, nil
	case "CANCELLED", "CANCELED":
		return domain.StatusCancelled, nil
	case "SUSPENDED":
		return domain.StatusAbandoned, nil
	default:
		return "", fmt.Errorf("%w: %q", ingest.ErrUnknownStatus, status)
	}
}
