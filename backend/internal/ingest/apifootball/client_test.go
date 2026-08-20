package apifootball_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest/apifootball"
)

// As with the football-data tests: the fixtures in testdata/ are built to
// API-Football's documented v3 response shape, not captured from a live call.
// No credential has ever been used against this API. Replace them with real
// captured responses after the first live sync — provider_payloads archives
// every response before parsing so that costs nothing extra.
//
// On a 100-request *daily* budget, discovering a shape mismatch by burning
// calls is genuinely expensive, which is most of why these exist.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func serve(t *testing.T, name string) (*apifootball.Client, *[]*http.Request) {
	t.Helper()

	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, name))
	}))
	t.Cleanup(srv.Close)

	c := apifootball.New("test-key", nil)
	c.BaseURL = srv.URL
	return c, &seen
}

// The rule that corrupts knockout rounds if it is wrong: `fulltime` is the
// 90-minute figure and `extratime` / `penalty` are reported separately, so
// taking anything but fulltime inflates every goals market in a cup tie.
func TestOnlyTheNinetyMinuteScoreIsTaken(t *testing.T) {
	c, _ := serve(t, "fixtures.json")

	got, err := c.Results(context.Background(), "292", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("results: %v", err)
	}

	byID := map[string]ingest.RawFixture{}
	for _, f := range got {
		byID[f.ProviderID] = f
	}

	for _, tc := range []struct {
		name       string
		id         string
		home, away int
	}{
		{"regular league match", "1215501", 2, 1},
		// 1-1 after 90, 1-2 after extra time. goals reports 1-2; fulltime
		// reports 1-1, and 1-1 is what settles.
		{"after extra time", "1215502", 1, 1},
		// 0-0 after 90 and after extra time, won 4-3 on penalties. On 1X2 this
		// is a draw, and on Over/Under 2.5 it is under.
		{"after penalties", "1215503", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := byID[tc.id]
			if !ok {
				t.Fatalf("fixture %s missing from the parsed set", tc.id)
			}
			if f.HomeScore == nil || f.AwayScore == nil {
				t.Fatal("score is nil on a finished match")
			}
			if *f.HomeScore != tc.home || *f.AwayScore != tc.away {
				t.Errorf("score = %d-%d, want %d-%d (the 90-minute figure)",
					*f.HomeScore, *f.AwayScore, tc.home, tc.away)
			}
			if f.Status != domain.StatusFinished {
				t.Errorf("status = %q, want finished", f.Status)
			}
		})
	}
}

// An abandoned match must never be gradable: the goals scored before it was
// called off are not a full-time result.
//
// The score is supplied inline rather than in testdata/fixtures.json because
// the realistic response for an abandoned match reports `fulltime` as null —
// which would make this pass without the clearing rule ever running. This is
// the adversarial version: a provider that *does* fill in fulltime on a match
// that never finished.
func TestAbandonedMatchCarriesNoScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":[{
			"fixture": {"id": 1215506, "timestamp": 1772982000, "status": {"short": "ABD", "elapsed": 63}},
			"league": {"id": 292, "season": 2025, "round": "Regular Season - 23"},
			"teams": {"home": {"id": 5303, "name": "Express FC"}, "away": {"id": 5305, "name": "BUL FC"}},
			"goals": {"home": 1, "away": 0},
			"score": {"halftime": {"home": 1, "away": 0}, "fulltime": {"home": 1, "away": 0},
			          "extratime": {"home": null, "away": null}, "penalty": {"home": null, "away": null}}
		}]}`))
	}))
	defer srv.Close()

	c := apifootball.New("test-key", nil)
	c.BaseURL = srv.URL

	got, err := c.Results(context.Background(), "292", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("results: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("parsed %d fixtures, want 1", len(got))
	}
	if got[0].Status != domain.StatusAbandoned {
		t.Errorf("status = %q, want abandoned", got[0].Status)
	}
	if got[0].HomeScore != nil || got[0].AwayScore != nil {
		t.Errorf("abandoned match kept a score of %d-%d; those goals are not a result",
			*got[0].HomeScore, *got[0].AwayScore)
	}
}

// The same rule read from the recorded shape: an abandoned match in the
// fixture file reports no full-time score and must stay ungradable.
func TestAbandonedFixtureInTheRecordedSetIsUngradable(t *testing.T) {
	c, _ := serve(t, "fixtures.json")

	got, err := c.Results(context.Background(), "292", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("results: %v", err)
	}
	for _, f := range got {
		if f.ProviderID != "1215506" {
			continue
		}
		if f.Status != domain.StatusAbandoned {
			t.Errorf("status = %q, want abandoned", f.Status)
		}
		if f.HomeScore != nil || f.AwayScore != nil {
			t.Error("abandoned match carried a score")
		}
		return
	}
	t.Fatal("fixture 1215506 missing from the parsed set")
}

func TestFixtureFieldsAreMapped(t *testing.T) {
	c, _ := serve(t, "fixtures.json")

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	got, err := c.Fixtures(context.Background(), "292", from, to)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("parsed %d fixtures, want 6", len(got))
	}

	first := got[0]
	if first.ProviderID != "1215501" {
		t.Errorf("provider id = %q", first.ProviderID)
	}
	if first.HomeName != "KCCA FC" || first.AwayName != "Vipers SC" {
		t.Errorf("teams = %q v %q", first.HomeName, first.AwayName)
	}
	if first.HomeProviderID != "5301" || first.AwayProviderID != "5304" {
		t.Errorf("team provider ids = %q / %q", first.HomeProviderID, first.AwayProviderID)
	}
	// Non-negotiable 11: UTC. The timestamp is the authority here rather than
	// the formatted date string, which carries an offset.
	want := time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC)
	if !first.KickoffAt.Equal(want) || first.KickoffAt.Location() != time.UTC {
		t.Errorf("kickoff = %s, want %s in UTC", first.KickoffAt, want)
	}
	if first.Round == nil || *first.Round != 22 {
		t.Errorf("round = %v, want 22 parsed out of \"Regular Season - 22\"", first.Round)
	}
	if first.Season != 2025 {
		t.Errorf("season = %d, want 2025", first.Season)
	}

	// A cup round has no matchday in it, and must not be given a fabricated
	// one — "Quarter-finals" parses to nothing.
	if got[1].Round != nil {
		t.Errorf("cup tie was given round %d", *got[1].Round)
	}
}

func TestRequestCarriesKeyAndWindow(t *testing.T) {
	c, seen := serve(t, "fixtures.json")

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	if _, err := c.Fixtures(context.Background(), "292", from, to); err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("made %d requests, want 1", len(*seen))
	}

	req := (*seen)[0]
	if req.Header.Get("x-apisports-key") != "test-key" {
		t.Errorf("x-apisports-key = %q", req.Header.Get("x-apisports-key"))
	}
	q := req.URL.Query()
	if q.Get("league") != "292" {
		t.Errorf("league = %q", q.Get("league"))
	}
	if q.Get("from") != "2026-03-01" || q.Get("to") != "2026-03-08" {
		t.Errorf("window = %s..%s", q.Get("from"), q.Get("to"))
	}
	// Asked for in UTC so the returned timestamps need no local reasoning.
	if q.Get("timezone") != "UTC" {
		t.Errorf("timezone = %q, want UTC", q.Get("timezone"))
	}
	// March belongs to the season that started the previous August.
	if q.Get("season") != "2025" {
		t.Errorf("season = %q, want 2025 for a March date", q.Get("season"))
	}
}

// A date in August belongs to the season starting that year, not the previous
// one. The boundary is where this goes wrong, so it is worth pinning.
func TestSeasonBoundary(t *testing.T) {
	for _, tc := range []struct {
		date time.Time
		want string
	}{
		{time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC), "2025"},
		{time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), "2026"},
		{time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "2025"},
		{time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), "2026"},
	} {
		t.Run(tc.date.Format("2006-01"), func(t *testing.T) {
			c, seen := serve(t, "fixtures.json")
			if _, err := c.Fixtures(context.Background(), "292", tc.date, tc.date); err != nil {
				t.Fatalf("fixtures: %v", err)
			}
			if got := (*seen)[0].URL.Query().Get("season"); got != tc.want {
				t.Errorf("season = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusCodesAreTranslated(t *testing.T) {
	c, _ := serve(t, "fixtures.json")

	got, err := c.Fixtures(context.Background(), "292", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}

	want := map[string]domain.MatchStatus{
		"1215501": domain.StatusFinished,  // FT
		"1215502": domain.StatusFinished,  // AET
		"1215503": domain.StatusFinished,  // PEN
		"1215504": domain.StatusScheduled, // NS
		"1215505": domain.StatusPostponed, // PST
		"1215506": domain.StatusAbandoned, // ABD
	}
	for _, f := range got {
		expected, ok := want[f.ProviderID]
		if !ok {
			t.Errorf("unexpected fixture %s in the parsed set", f.ProviderID)
			continue
		}
		if f.Status != expected {
			t.Errorf("fixture %s status = %q, want %q", f.ProviderID, f.Status, expected)
		}
	}
}

func TestUnknownStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":[{
			"fixture": {"id": 1, "timestamp": 1772377200, "status": {"short": "XYZ"}},
			"league": {"id": 292, "season": 2025, "round": "Regular Season - 1"},
			"teams": {"home": {"id": 1, "name": "A"}, "away": {"id": 2, "name": "B"}},
			"score": {"fulltime": {"home": null, "away": null}}
		}]}`))
	}))
	defer srv.Close()

	c := apifootball.New("test-key", nil)
	c.BaseURL = srv.URL

	_, err := c.Fixtures(context.Background(), "292", time.Now(), time.Now())
	if !errors.Is(err, ingest.ErrUnknownStatus) {
		t.Errorf("error = %v, want ErrUnknownStatus", err)
	}
}

func TestCompetitionsTakeTheCurrentSeason(t *testing.T) {
	c, _ := serve(t, "leagues.json")

	got, err := c.Competitions(context.Background())
	if err != nil {
		t.Fatalf("competitions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("parsed %d competitions, want 2", len(got))
	}
	if got[0].ProviderID != "292" || got[0].Name != "Uganda Premier League" {
		t.Errorf("first competition = %+v", got[0])
	}
	if got[0].Country != "Uganda" || got[0].CountryCode != "UG" {
		t.Errorf("country = %q / %q", got[0].Country, got[0].CountryCode)
	}
	// Two seasons are listed; only the one flagged current counts.
	if got[0].Season != 2025 {
		t.Errorf("season = %d, want 2025 — the season flagged current", got[0].Season)
	}
	// A continental competition has a null country code, which must not become
	// the string "null".
	if got[1].CountryCode != "" {
		t.Errorf("null country code parsed as %q", got[1].CountryCode)
	}
}

func TestTeamsAreMapped(t *testing.T) {
	c, _ := serve(t, "teams.json")

	got, err := c.Teams(context.Background(), "292", 2025)
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d teams, want 3", len(got))
	}
	if got[0].ProviderID != "5301" || got[0].Name != "KCCA FC" || got[0].ShortName != "KCC" {
		t.Errorf("first team = %+v", got[0])
	}
	// A null code must not become the string "null" on a match card.
	if got[2].ShortName != "" {
		t.Errorf("null team code parsed as %q", got[2].ShortName)
	}
}

// API-Football answers 200 with an `errors` object rather than an HTTP error
// when the key is wrong or the plan does not cover a league. The response then
// carries no fixtures, which must read as "nothing to ingest" rather than
// crashing the job — the daily sync covers many leagues and one being out of
// plan must not stop the rest.
func TestPlanErrorYieldsNoFixturesRatherThanACrash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"get":"fixtures","parameters":{},
			"errors":{"plan":"Your plan does not have access to this league."},
			"results":0,"paging":{"current":1,"total":1},"response":[]}`))
	}))
	defer srv.Close()

	c := apifootball.New("test-key", nil)
	c.BaseURL = srv.URL

	got, err := c.Fixtures(context.Background(), "999", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("an out-of-plan league failed the whole sync: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parsed %d fixtures from an error response", len(got))
	}
}

func TestRateLimitSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := apifootball.New("test-key", nil)
	c.BaseURL = srv.URL

	if _, err := c.Competitions(context.Background()); !errors.Is(err, ingest.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}
