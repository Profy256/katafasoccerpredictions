package footballdata_test

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
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest/footballdata"
)

// ROADMAP phase 2 asks for recorded-fixture tests. The fixtures in testdata/
// are built to football-data.org's documented v4 response shape rather than
// captured from a live call — no credential has ever been used against this
// API. They pin the parsing rules, which is most of what these tests are for;
// they cannot catch a field the documentation gets wrong.
//
// When the first live sync runs, replace these files with real captured
// responses. provider_payloads archives every response before parsing
// precisely so that can be done without spending request budget. The
// assertions below should survive that swap unchanged — if they do not, the
// shape differed and that is exactly the discovery worth making cheaply.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// serve returns a client pointed at a server that replies with one fixture,
// and the requests it received.
func serve(t *testing.T, name string) (*footballdata.Client, *[]*http.Request) {
	t.Helper()

	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, name))
	}))
	t.Cleanup(srv.Close)

	c := footballdata.New("test-token", nil)
	c.BaseURL = srv.URL
	return c, &seen
}

// The knockout rule, and the reason these tests exist at all: regularTime
// overrides fullTime on a tie that went past 90 minutes. Getting this wrong
// corrupts every goals market in every knockout round, silently, and only in
// the rounds that matter most.
func TestExtraTimeAndPenaltiesAreExcludedFromTheScore(t *testing.T) {
	c, _ := serve(t, "matches.json")

	got, err := c.Results(context.Background(), "PL", time.Now(), time.Now())
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
		// 2-2 after 90, 3-2 after extra time. The 90-minute figure is 2-2, and
		// on 1X2 that is a draw.
		{"extra time", "497232", 2, 2},
		// 1-1 after 90, won 4-3 on penalties. fullTime carries the shootout;
		// the 90-minute figure is 1-1.
		{"penalty shootout", "497233", 1, 1},
		// An ordinary league match has no regularTime and fullTime is correct.
		{"regular league match", "497231", 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := byID[tc.id]
			if !ok {
				t.Fatalf("fixture %s missing from the parsed set", tc.id)
			}
			if f.HomeScore == nil || f.AwayScore == nil {
				t.Fatalf("score is nil on a finished match")
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

// The schema ties status and score together: a finished match must carry a
// score. A provider reporting FINISHED with nulls would otherwise produce a
// row the database refuses, or worse, a gradable match with no goals.
func TestFinishedWithoutAScoreIsNotTreatedAsFinished(t *testing.T) {
	c, _ := serve(t, "matches.json")

	got, err := c.Results(context.Background(), "PL", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("results: %v", err)
	}

	var found bool
	for _, f := range got {
		if f.ProviderID != "497236" {
			continue
		}
		found = true
		if f.Status == domain.StatusFinished {
			t.Error("a FINISHED match with null scores was kept as finished")
		}
		if f.Status != domain.StatusScheduled {
			t.Errorf("status = %q, want scheduled", f.Status)
		}
		if f.HomeScore != nil || f.AwayScore != nil {
			t.Error("scores were not cleared alongside the downgraded status")
		}
	}
	if !found {
		t.Fatal("fixture 497236 missing from the parsed set")
	}
}

func TestFixtureFieldsAreMapped(t *testing.T) {
	c, seen := serve(t, "matches.json")

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	got, err := c.Fixtures(context.Background(), "PL", from, to)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("parsed %d fixtures, want 6", len(got))
	}

	first := got[0]
	if first.ProviderID != "497231" {
		t.Errorf("provider id = %q", first.ProviderID)
	}
	if first.HomeName != "Manchester City FC" || first.AwayName != "Chelsea FC" {
		t.Errorf("teams = %q v %q", first.HomeName, first.AwayName)
	}
	if first.HomeProviderID != "65" || first.AwayProviderID != "61" {
		t.Errorf("team provider ids = %q / %q — teams are created on demand from these",
			first.HomeProviderID, first.AwayProviderID)
	}
	// Non-negotiable 11: timestamps are UTC.
	want := time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC)
	if !first.KickoffAt.Equal(want) || first.KickoffAt.Location() != time.UTC {
		t.Errorf("kickoff = %s, want %s in UTC", first.KickoffAt, want)
	}
	if first.Round == nil || *first.Round != 28 {
		t.Errorf("round = %v, want 28", first.Round)
	}
	if first.Season != 2025 {
		t.Errorf("season = %d, want 2025 (derived from the season start date)", first.Season)
	}
	if first.CompetitionID != "PL" {
		t.Errorf("competition = %q, want PL", first.CompetitionID)
	}

	// A knockout tie has no matchday, and must not be given a fabricated one.
	if got[1].Round != nil {
		t.Errorf("knockout tie was given round %d", *got[1].Round)
	}

	// The request carries the auth header and the date window.
	if len(*seen) != 1 {
		t.Fatalf("made %d requests, want 1", len(*seen))
	}
	req := (*seen)[0]
	if req.Header.Get("X-Auth-Token") != "test-token" {
		t.Errorf("X-Auth-Token = %q", req.Header.Get("X-Auth-Token"))
	}
	q := req.URL.Query()
	if q.Get("dateFrom") != "2026-03-01" || q.Get("dateTo") != "2026-03-08" {
		t.Errorf("date window = %s..%s", q.Get("dateFrom"), q.Get("dateTo"))
	}
}

// Results narrows to finished matches at the provider, so the free-plan
// request budget is not spent transferring fixtures that are already known.
func TestResultsRequestsOnlyFinishedMatches(t *testing.T) {
	c, seen := serve(t, "matches.json")

	if _, err := c.Results(context.Background(), "PL", time.Now(), time.Now()); err != nil {
		t.Fatalf("results: %v", err)
	}
	if got := (*seen)[0].URL.Query().Get("status"); got != "FINISHED" {
		t.Errorf("status filter = %q, want FINISHED", got)
	}
}

func TestStatusVocabularyIsTranslated(t *testing.T) {
	c, _ := serve(t, "matches.json")

	got, err := c.Fixtures(context.Background(), "PL", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}

	want := map[string]domain.MatchStatus{
		"497231": domain.StatusFinished,
		"497234": domain.StatusScheduled, // TIMED
		"497235": domain.StatusPostponed,
	}
	for _, f := range got {
		if expected, ok := want[f.ProviderID]; ok && f.Status != expected {
			t.Errorf("fixture %s status = %q, want %q", f.ProviderID, f.Status, expected)
		}
	}
}

// An unrecognised status is an error, never a silent fall-through to
// 'scheduled' — that would resurrect abandoned matches and leave finished ones
// permanently ungraded.
func TestUnknownStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"matches":[{
			"id": 1, "utcDate": "2026-03-01T15:00:00Z", "status": "REARRANGED",
			"season": {"startDate": "2025-08-15"},
			"homeTeam": {"id": 1, "name": "A"}, "awayTeam": {"id": 2, "name": "B"},
			"score": {"fullTime": {"home": null, "away": null}}
		}]}`))
	}))
	defer srv.Close()

	c := footballdata.New("test-token", nil)
	c.BaseURL = srv.URL

	_, err := c.Fixtures(context.Background(), "PL", time.Now(), time.Now())
	if !errors.Is(err, ingest.ErrUnknownStatus) {
		t.Errorf("error = %v, want ErrUnknownStatus", err)
	}
}

func TestCompetitionsAreMapped(t *testing.T) {
	c, _ := serve(t, "competitions.json")

	got, err := c.Competitions(context.Background())
	if err != nil {
		t.Fatalf("competitions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d competitions, want 3", len(got))
	}
	if got[0].ProviderID != "2021" || got[0].Name != "Premier League" {
		t.Errorf("first competition = %+v", got[0])
	}
	if got[0].Country != "England" || got[0].CountryCode != "ENG" {
		t.Errorf("area = %q / %q", got[0].Country, got[0].CountryCode)
	}
	if got[0].Season != 2025 {
		t.Errorf("season = %d, want 2025", got[0].Season)
	}
	// A calendar-year league starting in April belongs to that year, not the
	// previous one.
	if got[2].Season != 2026 {
		t.Errorf("Brasileirão season = %d, want 2026", got[2].Season)
	}
}

func TestTeamsFallBackToTheThreeLetterAbbreviation(t *testing.T) {
	c, _ := serve(t, "teams.json")

	got, err := c.Teams(context.Background(), "PL", 2025)
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d teams, want 3", len(got))
	}
	if got[0].ShortName != "Man City" {
		t.Errorf("short name = %q, want Man City", got[0].ShortName)
	}
	// Brentford's shortName is empty in the fixture; the TLA stands in rather
	// than leaving a blank on the match card.
	if got[2].ShortName != "BRE" {
		t.Errorf("empty shortName fell back to %q, want BRE", got[2].ShortName)
	}
}

// A 4xx is not retried: it will not become a 2xx by being asked again, and on
// a 100-request daily budget a retry loop is expensive.
func TestClientErrorIsNotRetried(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Your API token is invalid."}`))
	}))
	defer srv.Close()

	c := footballdata.New("bad-token", nil)
	c.BaseURL = srv.URL

	if _, err := c.Competitions(context.Background()); err == nil {
		t.Fatal("a 403 was reported as success")
	}
	if calls != 1 {
		t.Errorf("made %d requests for a 403, want 1", calls)
	}
}

// A 429 with no Retry-After surfaces as ErrRateLimited so the caller can halve
// its local limiter for the rest of the day rather than keep hammering.
func TestRateLimitSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := footballdata.New("test-token", nil)
	c.BaseURL = srv.URL

	if _, err := c.Competitions(context.Background()); !errors.Is(err, ingest.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

// Every response is archived before it is parsed, whatever the status. That
// archive is what makes a parser bug replayable without spending budget — the
// single most useful property when the first live sync surprises you.
func TestResponsesAreArchivedBeforeParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"matches":[{"id":1,"utcDate":"nonsense","status":"FINISHED",
			"season":{"startDate":"2025-08-15"},
			"homeTeam":{"id":1,"name":"A"},"awayTeam":{"id":2,"name":"B"},
			"score":{"fullTime":{"home":1,"away":0}}}]}`))
	}))
	defer srv.Close()

	archive := &recordingArchive{}
	c := footballdata.New("test-token", archive)
	c.BaseURL = srv.URL

	// The parse fails on the unparseable date...
	if _, err := c.Fixtures(context.Background(), "PL", time.Now(), time.Now()); err == nil {
		t.Fatal("an unparseable date was accepted")
	}
	// ...and the body is still on disk to debug it with.
	if len(archive.payloads) != 1 {
		t.Fatalf("archived %d payloads, want 1", len(archive.payloads))
	}
	if archive.payloads[0].status != http.StatusOK {
		t.Errorf("archived status = %d", archive.payloads[0].status)
	}
	if len(archive.payloads[0].body) == 0 {
		t.Error("archived an empty body")
	}
}

type archivedPayload struct {
	provider, endpoint, url string
	status                  int
	body                    []byte
}

type recordingArchive struct{ payloads []archivedPayload }

func (a *recordingArchive) ArchivePayload(ctx context.Context, provider, endpoint, requestURL string, httpStatus int, body []byte) error {
	a.payloads = append(a.payloads, archivedPayload{provider, endpoint, requestURL, httpStatus, body})
	return nil
}
