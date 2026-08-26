package tips

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Fixed kickoffs on two UTC days. Deliberately late in the day: a 23:30 UTC
// kickoff is the case a local-time day boundary would file under tomorrow.
var (
	day1     = time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	day2     = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	kickoff1 = time.Date(2026, 8, 15, 14, 0, 0, 0, time.UTC)
	kickoff2 = time.Date(2026, 8, 15, 23, 30, 0, 0, time.UTC)
	kickoff3 = time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
)

// candidate builds one candidate with a deterministic id, so failures name a
// pick rather than a random UUID.
func candidate(t *testing.T, name string, match uuid.UUID, market domain.MarketCode, conf float64, kickoff time.Time) Candidate {
	t.Helper()
	return Candidate{
		Prediction: domain.Prediction{
			ID:              namedUUID(name),
			MatchID:         match,
			MarketCode:      market,
			PredictionValue: domain.OutcomeHome,
			ConfidencePct:   conf,
			ModelVersion:    "poisson-1.2.0",
		},
		KickoffAt: kickoff,
	}
}

func namedUUID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}

func TestOddsFromProbability(t *testing.T) {
	// Values checked against oddsFromProbability in src/data/tipsters.ts:
	// max(1.02, round((0.96 / p) * 100) / 100), with p <= 0.01 short-circuited.
	cases := []struct {
		probability float64
		want        string
	}{
		{0.0, "50"},
		{0.005, "50"},
		{0.01, "50"},     // boundary is <=, so 0.01 takes the short circuit
		{0.011, "87.27"}, // just past it, the formula applies
		{0.25, "3.84"},
		{0.5, "1.92"},
		{0.75, "1.28"},
		{0.768, "1.25"}, // exactly the publishable floor
		{0.8, "1.2"},
		{0.96, "1.02"}, // the margin cancels to evens, so the clamp applies
		{1.0, "1.02"},  // clamped, never a price below evens
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("p=%g", c.probability), func(t *testing.T) {
			got := OddsFromProbability(c.probability)
			want := decimal.RequireFromString(c.want)
			if !got.Equal(want) {
				t.Fatalf("OddsFromProbability(%g) = %s, want %s", c.probability, got, want)
			}
		})
	}
}

// The floor is the reason the shortlist is not four Double Chance 1X at 1.05.
func TestSelectAppliesOddsFloor(t *testing.T) {
	match1, match2 := namedUUID("m1"), namedUUID("m2")

	// 78% → 1.23, below the 1.25 floor. 76% → 1.26, above it.
	below := candidate(t, "below", match1, domain.MarketOneXTwo, 78, kickoff1)
	above := candidate(t, "above", match2, domain.MarketOneXTwo, 76, kickoff1)

	day := Select([]Candidate{below, above}, 4)

	tips := marketTips(t, day, domain.MarketOneXTwo)
	if len(tips) != 1 {
		t.Fatalf("got %d tips, want 1", len(tips))
	}
	if tips[0].PredictionID != above.Prediction.ID {
		t.Fatalf("published the sub-floor pick: %s", tips[0].PredictionID)
	}
	if tips[0].Odds.LessThan(MinPublishableOdds) {
		t.Fatalf("published odds %s below the floor %s", tips[0].Odds, MinPublishableOdds)
	}
}

// A pick that is exactly on the floor publishes: free_tips CHECKs odds >= 1.25,
// so an off-by-one here and the INSERT disagree.
func TestSelectPublishesOddsExactlyAtFloor(t *testing.T) {
	c := candidate(t, "onFloor", namedUUID("m1"), domain.MarketOneXTwo, 76.8, kickoff1)
	if got := OddsFromProbability(0.768); !got.Equal(MinPublishableOdds) {
		t.Fatalf("fixture no longer sits on the floor: %s", got)
	}

	tips := marketTips(t, Select([]Candidate{c}, 4), domain.MarketOneXTwo)
	if len(tips) != 1 {
		t.Fatalf("got %d tips, want the on-floor pick published", len(tips))
	}
}

// The ladder is what stops the shortlist being four near-identical bankers.
func TestSelectWalksTheConfidenceLadder(t *testing.T) {
	var candidates []Candidate
	// One fixture per pick, so the appearance cap is not what is being tested.
	// Two candidates in every band; only the stronger of each should be taken.
	confidences := []float64{76, 75.5, 70, 69, 65, 61, 55, 51, 40, 30}
	for i, conf := range confidences {
		name := fmt.Sprintf("c%d", i)
		candidates = append(candidates, candidate(t, name, namedUUID(name+"-match"), domain.MarketBTTS, conf, kickoff1))
	}

	tips := marketTips(t, Select(candidates, 4), domain.MarketBTTS)

	// perMarket = 4 takes the top four bands' best picks, and the tips are
	// published strongest first.
	want := []float64{76, 70, 65, 55}
	if len(tips) != len(want) {
		t.Fatalf("got %d tips, want %d", len(tips), len(want))
	}
	for i, conf := range want {
		if tips[i].ConfidencePct != conf {
			t.Fatalf("tip %d = %.1f%%, want %.1f%% (bands: %v)", i, tips[i].ConfidencePct, conf, tips)
		}
		if tips[i].Rank != i+1 {
			t.Fatalf("tip %d has rank %d", i, tips[i].Rank)
		}
	}
}

// With bands empty, the market still publishes a full shortlist.
func TestSelectBackfillsWhenBandsAreEmpty(t *testing.T) {
	var candidates []Candidate
	// Five picks all in the 60–68 band: one comes off the ladder, the rest must
	// come from the backfill pass, still strongest first.
	for i, conf := range []float64{67, 66, 65, 64, 63} {
		name := fmt.Sprintf("b%d", i)
		candidates = append(candidates, candidate(t, name, namedUUID(name+"-match"), domain.MarketOverUnder25, conf, kickoff1))
	}

	tips := marketTips(t, Select(candidates, 4), domain.MarketOverUnder25)

	want := []float64{67, 66, 65, 64}
	if len(tips) != len(want) {
		t.Fatalf("got %d tips, want %d", len(tips), len(want))
	}
	for i, conf := range want {
		if tips[i].ConfidencePct != conf {
			t.Fatalf("tip %d = %.1f%%, want %.1f%%", i, tips[i].ConfidencePct, conf)
		}
	}
}

// Without the cap, one low-scoring match wins every goals market and the page
// reads as four versions of the same tip.
func TestSelectCapsAppearancesPerFixture(t *testing.T) {
	hot := namedUUID("hot-match")

	var candidates []Candidate
	// The same fixture is the strongest pick in all six markets.
	for i, market := range domain.MarketCodes {
		name := fmt.Sprintf("hot-%d", i)
		candidates = append(candidates, candidate(t, name, hot, market, 70, kickoff1))
		// A weaker alternative in each market from its own fixture.
		other := fmt.Sprintf("other-%d", i)
		candidates = append(candidates, candidate(t, other, namedUUID(other+"-match"), market, 65, kickoff1))
	}

	day := Select(candidates, 4)

	appearances := 0
	for _, g := range day.Groups {
		for _, tip := range g.Tips {
			if tip.MatchID == hot {
				appearances++
			}
		}
	}
	if appearances != MaxAppearancesPerFixture {
		t.Fatalf("fixture appeared %d times, cap is %d", appearances, MaxAppearancesPerFixture)
	}
}

// MarketCodes order decides who spends the shared appearance budget first.
// Reordering the slice changes which tips publish, so the coupling is asserted
// rather than left as a comment.
func TestSelectSpendsAppearancesInMarketOrder(t *testing.T) {
	hot := namedUUID("hot-match")

	var candidates []Candidate
	for i, market := range domain.MarketCodes {
		name := fmt.Sprintf("hot-%d", i)
		candidates = append(candidates, candidate(t, name, hot, market, 70, kickoff1))
	}

	day := Select(candidates, 4)

	var got []domain.MarketCode
	for _, g := range day.Groups {
		got = append(got, g.Market)
	}
	want := domain.MarketCodes[:MaxAppearancesPerFixture]
	if len(got) != len(want) {
		t.Fatalf("published markets %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("published markets %v, want %v", got, want)
		}
	}
}

// A matchday that can fill a page is published exactly as it always was:
// one UTC day, nothing borrowed from tomorrow.
func TestSelectStaysOnOneMatchdayWhenItCanFillTheShortlist(t *testing.T) {
	var candidates []Candidate
	// Six fixtures on day 1, each priced in every market. Six fixtures at two
	// appearances apiece is comfortably past MinShortlistSize.
	for f := 0; f < 6; f++ {
		match := namedUUID(fmt.Sprintf("today-match-%d", f))
		for i, market := range domain.MarketCodes {
			name := fmt.Sprintf("today-%d-%d", f, i)
			candidates = append(candidates, candidate(t, name, match, market, 60+float64(f), kickoff1))
		}
	}
	tomorrow := candidate(t, "tomorrow", namedUUID("m3"), domain.MarketOneXTwo, 74, kickoff3)
	candidates = append(candidates, tomorrow)

	day := Select(candidates, 4)

	if !day.Day.Equal(day1) {
		t.Fatalf("matchday = %s, want %s", day.Day.Format(time.RFC3339), day1.Format(time.RFC3339))
	}
	if day.CoversDays != 1 {
		t.Errorf("covered %d days, want 1 — a full matchday must not reach forward", day.CoversDays)
	}
	if day.TotalTips < MinShortlistSize {
		t.Fatalf("published %d tips, expected at least the %d floor", day.TotalTips, MinShortlistSize)
	}
	for _, g := range day.Groups {
		for _, tip := range g.Tips {
			if tip.PredictionID == tomorrow.Prediction.ID {
				t.Fatal("published a fixture from the following day")
			}
		}
	}
	if day2.Equal(day1) { // guards the fixtures above, not the code
		t.Fatal("test days collapsed")
	}
}

// The bug this replaces a test for: a midweek card of one or two fixtures
// produced a two-tip page while a dozen fixtures sat 48 hours out, unpublished.
// A starved matchday now reaches forward instead.
func TestSelectExtendsWindowWhenMatchdayIsStarved(t *testing.T) {
	var candidates []Candidate
	// One lonely fixture on day 1 — two tips at the appearance cap, however
	// many markets price it.
	lonely := namedUUID("lonely-match")
	for i, market := range domain.MarketCodes {
		candidates = append(candidates,
			candidate(t, fmt.Sprintf("lonely-%d", i), lonely, market, 70, kickoff1))
	}
	// A full card on day 2.
	for f := 0; f < 6; f++ {
		match := namedUUID(fmt.Sprintf("tomorrow-match-%d", f))
		for i, market := range domain.MarketCodes {
			name := fmt.Sprintf("tomorrow-%d-%d", f, i)
			candidates = append(candidates, candidate(t, name, match, market, 60+float64(f), kickoff3))
		}
	}

	day := Select(candidates, 4)

	if !day.Day.Equal(day1) {
		t.Fatalf("matchday = %s, want the first day with fixtures (%s)",
			day.Day.Format(time.RFC3339), day1.Format(time.RFC3339))
	}
	if day.CoversDays != 2 {
		t.Errorf("covered %d days, want 2 — one starved day should reach exactly one day forward",
			day.CoversDays)
	}
	if day.TotalTips < MinShortlistSize {
		t.Fatalf("published %d tips, want at least the %d floor once the window widened",
			day.TotalTips, MinShortlistSize)
	}
	// The lonely fixture is still in: widening adds candidates, it does not
	// replace the matchday.
	found := false
	for _, g := range day.Groups {
		for _, tip := range g.Tips {
			if tip.MatchID == lonely {
				found = true
			}
		}
	}
	if !found {
		t.Error("the matchday's own fixture dropped out when the window widened")
	}
}

// Better a short shortlist than a stale one: the window stops at
// MaxWindowDays even when it still has not filled the page.
func TestSelectStopsAtMaxWindowDays(t *testing.T) {
	// One fixture per day for a week: every window is starved, so the loop
	// runs to its cap.
	var candidates []Candidate
	for d := 0; d < 7; d++ {
		match := namedUUID(fmt.Sprintf("day-%d-match", d))
		kickoff := kickoff1.AddDate(0, 0, d)
		for i, market := range domain.MarketCodes {
			candidates = append(candidates,
				candidate(t, fmt.Sprintf("d%d-%d", d, i), match, market, 70, kickoff))
		}
	}

	day := Select(candidates, 4)

	if day.CoversDays != MaxWindowDays {
		t.Fatalf("covered %d days, want the %d cap", day.CoversDays, MaxWindowDays)
	}
	if day.TotalTips >= MinShortlistSize {
		t.Fatalf("published %d tips; this fixture set cannot reach the %d floor, "+
			"so the test is no longer exercising the cap", day.TotalTips, MinShortlistSize)
	}
	// Nothing beyond the cap leaked in.
	limit := day1.AddDate(0, 0, MaxWindowDays)
	for _, g := range day.Groups {
		for _, tip := range g.Tips {
			for _, c := range candidates {
				if c.Prediction.ID == tip.PredictionID && !c.KickoffAt.Before(limit) {
					t.Fatalf("published a fixture kicking off %s, past the %s window limit",
						c.KickoffAt.Format(time.RFC3339), limit.Format(time.RFC3339))
				}
			}
		}
	}
}

// An international break is a legitimate outcome, not a reason to write an
// empty free_tip_days row claiming a shortlist was published.
func TestSelectEmptyWhenNoCandidates(t *testing.T) {
	day := Select(nil, 4)
	if !day.IsEmpty() {
		t.Fatalf("empty input produced %d tips", day.TotalTips)
	}
	if !day.Day.IsZero() {
		t.Fatalf("empty input produced matchday %s", day.Day)
	}
	if len(day.Groups) != 0 {
		t.Fatalf("empty input produced %d groups", len(day.Groups))
	}
}

// The bug: the shared appearance budget was spent by filling each market to
// perMarket in turn, so 1X2 and Double Chance claimed two slots on every
// fixture and the three goals markets published nothing at all. Every market
// with something publishable must get a pick before any market gets a second.
func TestSelectGivesEveryMarketATipBeforeAnyMarketGetsSeconds(t *testing.T) {
	// Five fixtures, every market priced on each — deliberately fewer
	// fixture-slots (5 x 2 = 10) than six markets filling four tips each (24),
	// which is exactly the shape that starved the late markets.
	var candidates []Candidate
	for f := 0; f < 5; f++ {
		match := namedUUID(fmt.Sprintf("m-%d", f))
		for i, market := range domain.MarketCodes {
			// Confidence varies by market so the ladder has real choices and
			// the markets are not tied.
			conf := 55 + float64(f)*3 + float64(i)
			candidates = append(candidates,
				candidate(t, fmt.Sprintf("f%d-m%d", f, i), match, market, conf, kickoff1))
		}
	}

	day := Select(candidates, 4)

	got := make(map[domain.MarketCode]int, len(domain.MarketCodes))
	for _, g := range day.Groups {
		got[g.Market] = len(g.Tips)
	}
	for _, market := range domain.MarketCodes {
		if got[market] == 0 {
			t.Errorf("%s published nothing while the shortlist ran to %d tips; "+
				"it had a candidate on every fixture", market, day.TotalTips)
		}
	}

	// Nobody may take a second while somebody publishable is still on zero.
	// With every market non-empty above, the check that matters is the spread:
	// no market should be more than one ahead of the smallest.
	minTips, maxTips := -1, 0
	for _, market := range domain.MarketCodes {
		n := got[market]
		if minTips == -1 || n < minTips {
			minTips = n
		}
		if n > maxTips {
			maxTips = n
		}
	}
	if maxTips-minTips > 1 {
		t.Errorf("tips per market ranged %d..%d; round-robin should keep them within one",
			minTips, maxTips)
	}

	// The appearance cap is still the cap — fairness must not be bought by
	// letting one fixture carry the whole page.
	perFixture := make(map[uuid.UUID]int)
	for _, g := range day.Groups {
		for _, tip := range g.Tips {
			perFixture[tip.MatchID]++
		}
	}
	for match, n := range perFixture {
		if n > MaxAppearancesPerFixture {
			t.Errorf("fixture %s appeared %d times, cap is %d", match, n, MaxAppearancesPerFixture)
		}
	}
}

// Fairness must not manufacture tips: a market whose every candidate fails the
// odds floor stays empty, because the alternative is publishing a 1.05.
func TestSelectLeavesAMarketEmptyWhenNothingClearsTheFloor(t *testing.T) {
	var candidates []Candidate
	for f := 0; f < 5; f++ {
		match := namedUUID(fmt.Sprintf("m-%d", f))
		for _, market := range domain.MarketCodes {
			conf := 60.0
			// Over 3.5 is priced so high that its odds fall under the floor.
			if market == domain.MarketOverUnder35 {
				conf = 99
			}
			candidates = append(candidates,
				candidate(t, fmt.Sprintf("f%d-%s", f, market), match, market, conf, kickoff1))
		}
	}

	day := Select(candidates, 4)

	for _, g := range day.Groups {
		if g.Market == domain.MarketOverUnder35 {
			t.Fatalf("published %d %s tips below the %s odds floor",
				len(g.Tips), g.Market, MinPublishableOdds)
		}
	}
	if day.TotalTips == 0 {
		t.Fatal("the other five markets published nothing either; the fixture set is wrong")
	}
}

// A model upgrade adds a second predictions row for the same match and market.
// Both offered at once would let one fixture's single pick spend two
// appearances against itself.
func TestSelectTakesOnePredictionPerMatchAndMarket(t *testing.T) {
	match := namedUUID("m1")
	v1 := candidate(t, "v1", match, domain.MarketOneXTwo, 70, kickoff1)
	v2 := candidate(t, "v2", match, domain.MarketOneXTwo, 65, kickoff1)
	v2.Prediction.ModelVersion = "poisson-1.3.0"

	tips := marketTips(t, Select([]Candidate{v1, v2}, 4), domain.MarketOneXTwo)
	if len(tips) != 1 {
		t.Fatalf("got %d tips for one (match, market), want 1", len(tips))
	}
}

// A republished day must not differ from the frozen one because Postgres
// returned rows in a different order.
func TestSelectIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	var candidates []Candidate
	for i, market := range domain.MarketCodes {
		for j, conf := range []float64{70, 70, 65, 62, 55, 45} {
			name := fmt.Sprintf("m%d-c%d", i, j)
			candidates = append(candidates, candidate(t, name, namedUUID(name+"-match"), market, conf, kickoff1))
		}
	}

	want := Select(candidates, 4)

	reversed := make([]Candidate, 0, len(candidates))
	for i := len(candidates) - 1; i >= 0; i-- {
		reversed = append(reversed, candidates[i])
	}
	got := Select(reversed, 4)

	if got.TotalTips != want.TotalTips || len(got.Groups) != len(want.Groups) {
		t.Fatalf("reversed input gave %d tips in %d groups, want %d in %d",
			got.TotalTips, len(got.Groups), want.TotalTips, len(want.Groups))
	}
	for gi := range want.Groups {
		if got.Groups[gi].Market != want.Groups[gi].Market {
			t.Fatalf("group %d market = %s, want %s", gi, got.Groups[gi].Market, want.Groups[gi].Market)
		}
		for ti := range want.Groups[gi].Tips {
			a, b := want.Groups[gi].Tips[ti], got.Groups[gi].Tips[ti]
			if a.PredictionID != b.PredictionID || a.Rank != b.Rank {
				t.Fatalf("%s rank %d: got %s, want %s", a.MarketCode, a.Rank, b.PredictionID, a.PredictionID)
			}
		}
	}
}

// perMarket <= 0 is the frontend's default argument, not a request for nothing.
func TestSelectDefaultsPerMarket(t *testing.T) {
	var candidates []Candidate
	for i, conf := range []float64{76, 70, 65, 55, 45, 44} {
		name := fmt.Sprintf("d%d", i)
		candidates = append(candidates, candidate(t, name, namedUUID(name+"-match"), domain.MarketOneXTwo, conf, kickoff1))
	}

	tips := marketTips(t, Select(candidates, 0), domain.MarketOneXTwo)
	if len(tips) != DefaultPerMarket {
		t.Fatalf("got %d tips, want DefaultPerMarket = %d", len(tips), DefaultPerMarket)
	}
}

// Ranks are what free_tips (day, market_code, rank) is unique on: a gap or a
// duplicate is a failed INSERT at publish time.
func TestSelectRanksAreContiguousPerMarket(t *testing.T) {
	var candidates []Candidate
	for i, market := range domain.MarketCodes {
		for j, conf := range []float64{76, 70, 65, 55} {
			name := fmt.Sprintf("r%d-%d", i, j)
			candidates = append(candidates, candidate(t, name, namedUUID(name+"-match"), market, conf, kickoff1))
		}
	}

	for _, g := range Select(candidates, 4).Groups {
		for i, tip := range g.Tips {
			if tip.Rank != i+1 {
				t.Fatalf("%s tip %d has rank %d", g.Market, i, tip.Rank)
			}
			if i > 0 && g.Tips[i-1].ConfidencePct < tip.ConfidencePct {
				t.Fatalf("%s is not published strongest first", g.Market)
			}
		}
	}
}

func marketTips(t *testing.T, day Day, market domain.MarketCode) []Tip {
	t.Helper()
	for _, g := range day.Groups {
		if g.Market == market {
			return g.Tips
		}
	}
	return nil
}
