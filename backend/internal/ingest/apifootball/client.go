// Package apifootball is the API-Football (api-sports.io) v3 client.
//
// Free plan: 100 requests/day, resetting at 00:00 UTC, across ~1,200 leagues
// including the Ugandan Premier League, CAF competitions and the Asian
// leagues. It carries everything football-data.org does not.
package apifootball

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest"
)

const BaseURL = "https://v3.football.api-sports.io"

type Client struct {
	BaseURL string
	Key     string
	fetcher *ingest.Fetcher
}

func New(key string, archive ingest.Archiver) *Client {
	c := &Client{BaseURL: BaseURL, Key: key}
	c.fetcher = &ingest.Fetcher{
		Provider: ingest.ProviderAPIFootball,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Archive:  archive,
		Header: func(r *http.Request) {
			r.Header.Set("x-apisports-key", key)
		},
	}
	return c
}

func (c *Client) Name() string { return ingest.ProviderAPIFootball }

type leaguesResponse struct {
	Response []struct {
		League struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"league"`
		Country struct {
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"country"`
		Seasons []struct {
			Year    int  `json:"year"`
			Current bool `json:"current"`
		} `json:"seasons"`
	} `json:"response"`
}

func (c *Client) Competitions(ctx context.Context) ([]ingest.RawCompetition, error) {
	body, err := c.fetcher.Get(ctx, "leagues", c.BaseURL+"/leagues?current=true")
	if err != nil {
		return nil, err
	}
	var decoded leaguesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("api-football: decode leagues: %w", err)
	}

	out := make([]ingest.RawCompetition, 0, len(decoded.Response))
	for _, l := range decoded.Response {
		season := 0
		for _, s := range l.Seasons {
			if s.Current {
				season = s.Year
			}
		}
		out = append(out, ingest.RawCompetition{
			ProviderID:  fmt.Sprint(l.League.ID),
			Name:        l.League.Name,
			Country:     l.Country.Name,
			CountryCode: l.Country.Code,
			Season:      season,
		})
	}
	return out, nil
}

type teamsResponse struct {
	Response []struct {
		Team struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"team"`
	} `json:"response"`
}

func (c *Client) Teams(ctx context.Context, competition string, season int) ([]ingest.RawTeam, error) {
	url := fmt.Sprintf("%s/teams?league=%s&season=%d", c.BaseURL, competition, season)
	body, err := c.fetcher.Get(ctx, "teams", url)
	if err != nil {
		return nil, err
	}
	var decoded teamsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("api-football: decode teams: %w", err)
	}

	out := make([]ingest.RawTeam, 0, len(decoded.Response))
	for _, t := range decoded.Response {
		out = append(out, ingest.RawTeam{
			ProviderID: fmt.Sprint(t.Team.ID),
			Name:       t.Team.Name,
			ShortName:  t.Team.Code,
		})
	}
	return out, nil
}

type fixturesResponse struct {
	Response []struct {
		Fixture struct {
			ID        int    `json:"id"`
			Timestamp int64  `json:"timestamp"`
			Date      string `json:"date"`
			Status    struct {
				Short string `json:"short"`
			} `json:"status"`
		} `json:"fixture"`
		League struct {
			ID     int    `json:"id"`
			Season int    `json:"season"`
			Round  string `json:"round"`
		} `json:"league"`
		Teams struct {
			Home struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"home"`
			Away struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"away"`
		} `json:"teams"`
		// Score reports the periods separately. Only fulltime is used: it is
		// the 90-minute figure, while extratime and penalty are not.
		Score struct {
			Halftime struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"halftime"`
			Fulltime struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"fulltime"`
			Extratime struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"extratime"`
			Penalty struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"penalty"`
		} `json:"score"`
	} `json:"response"`
}

func (c *Client) Fixtures(ctx context.Context, competition string, from, to time.Time) ([]ingest.RawFixture, error) {
	return c.fixtures(ctx, "fixtures", competition, from, to)
}

func (c *Client) Results(ctx context.Context, competition string, from, to time.Time) ([]ingest.RawFixture, error) {
	return c.fixtures(ctx, "fixtures-results", competition, from, to)
}

func (c *Client) fixtures(ctx context.Context, endpoint, competition string, from, to time.Time) ([]ingest.RawFixture, error) {
	url := fmt.Sprintf("%s/fixtures?league=%s&season=%d&from=%s&to=%s&timezone=UTC",
		c.BaseURL, competition, seasonFor(from),
		from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))

	body, err := c.fetcher.Get(ctx, endpoint, url)
	if err != nil {
		return nil, err
	}
	var decoded fixturesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("api-football: decode fixtures: %w", err)
	}

	out := make([]ingest.RawFixture, 0, len(decoded.Response))
	for _, f := range decoded.Response {
		status, err := mapStatus(f.Fixture.Status.Short)
		if err != nil {
			return nil, fmt.Errorf("api-football: fixture %d: %w", f.Fixture.ID, err)
		}

		// The 90-minute score only. A tie level after 90 that was decided in
		// extra time or on penalties settles as a draw, and taking anything
		// else here corrupts every goals market in knockout rounds.
		homeScore, awayScore := f.Score.Fulltime.Home, f.Score.Fulltime.Away
		if status != domain.StatusFinished || homeScore == nil || awayScore == nil {
			if status == domain.StatusFinished {
				status = domain.StatusScheduled
			}
			homeScore, awayScore = nil, nil
		}

		out = append(out, ingest.RawFixture{
			ProviderID:     fmt.Sprint(f.Fixture.ID),
			CompetitionID:  competition,
			HomeName:       f.Teams.Home.Name,
			AwayName:       f.Teams.Away.Name,
			HomeProviderID: providerID(f.Teams.Home.ID),
			AwayProviderID: providerID(f.Teams.Away.ID),
			KickoffAt:      time.Unix(f.Fixture.Timestamp, 0).UTC(),
			Status:         status,
			HomeScore:      homeScore,
			AwayScore:      awayScore,
			Round:          parseRound(f.League.Round),
			Season:         f.League.Season,
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

// seasonFor picks the season year a date falls in. Most leagues run
// August–May, so a date before July belongs to the season that started the
// previous year.
func seasonFor(t time.Time) int {
	t = t.UTC()
	if t.Month() < time.July {
		return t.Year() - 1
	}
	return t.Year()
}

// parseRound extracts the matchday from strings like "Regular Season - 12".
func parseRound(round string) *int {
	var n int
	if _, err := fmt.Sscanf(round, "Regular Season - %d", &n); err == nil && n > 0 {
		return &n
	}
	return nil
}

// mapStatus translates API-Football's short codes. Anything unrecognised is an
// error rather than a silent 'scheduled'.
func mapStatus(short string) (domain.MatchStatus, error) {
	switch short {
	case "TBD", "NS":
		return domain.StatusScheduled, nil
	case "1H", "HT", "2H", "ET", "BT", "P", "LIVE", "INT":
		return domain.StatusInPlay, nil
	case "FT", "AET", "PEN", "WO":
		return domain.StatusFinished, nil
	case "PST":
		return domain.StatusPostponed, nil
	case "CANC":
		return domain.StatusCancelled, nil
	case "ABD", "SUSP", "AWD":
		return domain.StatusAbandoned, nil
	default:
		return "", fmt.Errorf("%w: %q", ingest.ErrUnknownStatus, short)
	}
}
