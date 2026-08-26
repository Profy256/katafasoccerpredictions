// Package tips selects the daily free shortlist.
//
// This is a port of getFreeTips in src/api/client.ts, with one deliberate
// difference: the frontend re-derives the shortlist live from current data on
// every read, and the backend must not. Selection happens once, in
// publish_free_tips at 05:00, and the result is frozen into free_tip_days /
// free_tips. The API only ever reads those rows.
//
// Computed live, the shortlist is a function of *current* data: once matches
// finish they stop being scheduled, so yesterday's shortlist could not be
// reconstructed — and if it could, a model rerun would reconstruct a different
// one. "We went 4 from 5 yesterday" requires a row saying what yesterday's
// five were, written before those matches kicked off. See
// docs/backend/SETTLEMENT.md § Why the free shortlist is frozen.
//
// Select is therefore pure: it takes candidates and returns the selection, with
// no database and no clock in scope. Freezing it is the caller's job.
package tips

import (
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

/*
 * The three constants below are ported verbatim from src/api/client.ts,
 * comments included. They are not tuning knobs: each one exists because of a
 * specific way the shortlist reads badly without it, and the reason is only
 * recorded here. When the frontend's copy is deleted at cutover, this becomes
 * the only record of why the numbers are what they are.
 */

// MinPublishableOdds is the floor below which a selection is not worth
// publishing as a tip.
//
// Ranking purely on model confidence surfaces things like Double Chance 1X at
// 1.05 — technically the most likely outcome on the card, and useless to
// anyone. The floor keeps the shortlist to selections that actually pay.
//
// free_tips.odds carries a CHECK matching this value, so a change here without
// a migration fails loudly at INSERT rather than quietly publishing 1.05s.
var MinPublishableOdds = decimal.RequireFromString("1.25")

// MaxAppearancesPerFixture caps how many times one fixture can carry the
// shortlist. Without it a single low-scoring match wins every goals market and
// the page reads as four versions of the same tip.
const MaxAppearancesPerFixture = 2

// ConfidenceBand is one rung of ConfidenceLadder. Membership is [Min, Max):
// half-open, so the bands partition 0–101 without a pick falling in two.
type ConfidenceBand struct {
	Min float64
	Max float64
}

// ConfidenceLadder is the confidence bands the shortlist is drawn from, safest
// first.
//
// Odds here are derived from the model's own probability, so confidence and
// price move in lockstep — ranking a market purely by confidence returns four
// near-identical selections sitting on the odds floor. Taking the best pick out
// of each band instead gives a ladder from banker to value, which is both more
// useful to read and more honest about what the model is offering.
var ConfidenceLadder = []ConfidenceBand{
	{75, 101},
	{68, 75},
	{60, 68},
	{50, 60},
	{0, 50},
}

// DefaultPerMarket is how many tips each market publishes, matching the
// frontend's default argument to getFreeTips.
const DefaultPerMarket = 4

// MinShortlistSize is the point below which a matchday is treated as starved
// and the selection window reaches forward a day at a time.
//
// The shortlist used to cover exactly one UTC day, full stop. On a weekend
// that is right — ten fixtures easily fill six markets. Midweek it produced a
// page with two tips on it while a dozen perfectly good fixtures sat 48 hours
// out, unpublished and invisible: not a thin *model*, a thin *window*. A
// reader cannot tell those apart, and "2 tips" reads as a broken site.
//
// Ten is roughly two per market — enough that every market has something to
// show. It is a floor, not a target: a day that clears it is published exactly
// as it was before, so nothing changes on the days that were already healthy.
const MinShortlistSize = 10

// MaxWindowDays caps how far forward a starved day may reach.
//
// Three days is the practical limit of "upcoming". Past that the model is
// pricing fixtures whose team news does not exist yet, and a tip published
// four days early has four days to be invalidated by an injury the model
// cannot see. Better a short shortlist than a stale one.
const MaxWindowDays = 3

// oddsMargin is the bookmaker margin applied when turning a model probability
// into odds, from oddsFromProbability in src/data/tipsters.ts.
//
// These are derived prices, not real ones: no bookmaker odds are ingested yet
// (ROADMAP.md § Deliberately later). Honest, but not a market price, and the
// distinction matters if anyone ever compares the published odds to a book.
var oddsMargin = decimal.RequireFromString("0.96")

// The derived-price clamps, also from oddsFromProbability. A near-zero
// probability would otherwise produce an absurd number rather than a price.
var (
	maxDerivedOdds = decimal.NewFromInt(50)
	minDerivedOdds = decimal.RequireFromString("1.02")
	minProbability = 0.01
)

// OddsFromProbability turns a model probability (0..1) into decimal odds,
// mirroring oddsFromProbability in src/data/tipsters.ts.
//
// decimal, not float: these odds are stored as NUMERIC(7,3) and multiply into
// accumulator prices, where float drift across five legs is visible to users.
// The two-decimal rounding is the frontend's and is kept so the same
// probability yields the same published price on both sides of the cutover.
func OddsFromProbability(probability float64) domain.Odds {
	if probability <= minProbability {
		return maxDerivedOdds
	}
	odds := oddsMargin.Div(decimal.NewFromFloat(probability)).Round(2)
	if odds.LessThan(minDerivedOdds) {
		return minDerivedOdds
	}
	return odds
}

// Candidate is one prediction offered to the shortlist, with the kickoff of the
// match it belongs to.
//
// The caller supplies only predictions that are eligible to be published at
// all: scheduled matches (never in_play — a "tip" on a match already underway
// is not a tip), from leagues with is_published = true (INGESTION.md § Cold
// start runs the rest in shadow mode), and one model version, so a model
// upgrade does not offer two rows for the same pick.
type Candidate struct {
	Prediction domain.Prediction
	KickoffAt  time.Time
}

// Tip is one selected pick, in the shape free_tips stores.
type Tip struct {
	PredictionID  uuid.UUID
	MatchID       uuid.UUID
	MarketCode    domain.MarketCode
	ConfidencePct float64
	Odds          domain.Odds
	// Rank is 1-based within the market, in published order. It is part of the
	// (day, market_code, rank) uniqueness in free_tips: the display order is
	// frozen too, not re-sorted on read.
	Rank int
}

// Group is one market's shortlist.
type Group struct {
	Market domain.MarketCode
	Tips   []Tip
}

// Day is a whole shortlist, ready to be written as one free_tip_days row plus
// its free_tips rows.
type Day struct {
	// Day is the matchday the shortlist covers, at UTC midnight — the DATE the
	// free_tip_days primary key stores. Zero when nothing was selected.
	//
	// All job schedules are UTC (ARCHITECTURE.md § Job flow), so the day
	// boundary is UTC too. A local-time boundary would put a 23:30 UTC kickoff
	// on a different day than the job that published it.
	Day    time.Time
	Groups []Group
	// CoversDays is how many UTC days of fixtures the selection reached over,
	// starting at Day. Normally 1. More means the matchday was starved and the
	// window extended — worth logging, because a run of 3s is the model or the
	// ingestion running dry, not a quiet Tuesday.
	CoversDays int
	TotalTips  int
}

// IsEmpty reports whether there is nothing to publish. A day with no eligible
// fixtures is a legitimate outcome — an international break, or every league in
// shadow mode — and writing an empty free_tip_days row would claim a shortlist
// was published when none was.
func (d Day) IsEmpty() bool { return d.TotalTips == 0 }

// Select builds the shortlist: the strongest model picks per market for the
// next matchday. This is what the free tier publishes instead of the whole
// fixture list — the full feed still lives at /fixtures.
//
// perMarket is how many tips each market publishes; zero or less means
// DefaultPerMarket.
//
// The output is deterministic for a given candidate set: ties on confidence are
// broken by kickoff, then by prediction id, rather than by the order rows came
// back from Postgres. The frontend relies on JavaScript's stable sort over
// kickoff-ordered matches for the same effect; leaving it implicit here would
// make a republished day differ from the frozen one for no visible reason.
func Select(candidates []Candidate, perMarket int) Day {
	if perMarket <= 0 {
		perMarket = DefaultPerMarket
	}

	// The shortlist starts at the next matchday with fixtures on it.
	day, ok := nextMatchday(candidates)
	if !ok {
		return Day{}
	}

	// One day is the normal case and the first thing tried, so a healthy
	// matchday is selected exactly as it always was. A starved one reaches
	// forward a day at a time rather than publishing a two-tip page.
	//
	// Re-running the whole selection per width, rather than topping up a
	// previous pass, is deliberate: the confidence ladder and the per-fixture
	// appearance cap are properties of the candidate set as a whole, and a
	// widened set can legitimately produce a *different* — better — ladder
	// than the narrow one it replaces. Topping up would freeze the narrow
	// day's choices and staple the extras on the end.
	var selected Day
	for width := 1; width <= MaxWindowDays; width++ {
		selected = selectWindow(candidates, day, width, perMarket)
		if selected.TotalTips >= MinShortlistSize {
			break
		}
	}
	return selected
}

// selectWindow runs the selection over `width` UTC days starting at day.
//
// The per-fixture appearance budget is shared by all six markets, and it used
// to be spent by filling each market to perMarket in turn: 1X2 took two slots
// on every fixture, Double Chance took the rest, and the goals markets found
// nothing left. On a small card that published ten tips across three markets
// and left Over/Under 1.5, 2.5 and 3.5 completely empty — three published
// market pages with nothing on them, and three markets absent from a shortlist
// that claims to price six.
//
// So the budget is handed out in rounds instead: every market takes its first
// pick before any market takes its second. A market is only empty now if it
// genuinely has nothing to publish — no candidate clearing the odds floor —
// rather than because it sorts late in domain.MarketCodes.
//
// Within a round, MarketCodes order still decides who reaches a contested
// fixture first, so that coupling is unchanged and still asserted.
func selectWindow(candidates []Candidate, day time.Time, width, perMarket int) Day {
	byMarket := groupByMarket(candidates, day, day.AddDate(0, 0, width))

	appearances := make(map[uuid.UUID]int)

	pickers := make([]*marketPicker, 0, len(domain.MarketCodes))
	for _, market := range domain.MarketCodes {
		pickers = append(pickers, &marketPicker{
			market:     market,
			candidates: byMarket[market],
			taken:      make(map[uuid.UUID]bool),
		})
	}

	// Round-robin. perMarket rounds, one pick per market per round.
	for round := 0; round < perMarket; round++ {
		progressed := false
		for _, p := range pickers {
			if p.takeOne(appearances) {
				progressed = true
			}
		}
		// Every market is out of reachable candidates; more rounds cannot help.
		if !progressed {
			break
		}
	}

	var groups []Group
	total := 0
	for _, p := range pickers {
		if len(p.tips) == 0 {
			continue
		}
		p.finalise()
		groups = append(groups, Group{Market: p.market, Tips: p.tips})
		total += len(p.tips)
	}

	return Day{Day: day, Groups: groups, CoversDays: width, TotalTips: total}
}

// marketPicker walks one market's confidence ladder, one pick at a time.
//
// It is stateful because the rounds in selectWindow interleave the markets:
// the ladder position has to survive between a market's turns, or every round
// would hand back the same safest pick.
type marketPicker struct {
	market     domain.MarketCode
	candidates []Candidate
	tips       []Tip
	taken      map[uuid.UUID]bool
	// band is how far down ConfidenceLadder this market has walked. Once it
	// reaches the end the market backfills on confidence, which is what stops
	// a market with picks in only one band publishing a single tip.
	band int
}

// takeOne adds at most one tip, spending from the shared appearance budget.
// It reports whether it added one.
func (p *marketPicker) takeOne(appearances map[uuid.UUID]int) bool {
	claim := func(c Candidate) bool {
		used := appearances[c.Prediction.MatchID]
		if used >= MaxAppearancesPerFixture {
			return false
		}
		appearances[c.Prediction.MatchID] = used + 1
		p.taken[c.Prediction.ID] = true
		p.tips = append(p.tips, toTip(c))
		return true
	}

	// Best available pick from the next non-empty band, safest band first.
	for p.band < len(ConfidenceLadder) {
		band := ConfidenceLadder[p.band]
		p.band++
		for _, c := range p.candidates {
			conf := c.Prediction.ConfidencePct
			if p.taken[c.Prediction.ID] || conf < band.Min || conf >= band.Max {
				continue
			}
			// A candidate whose fixture is out of budget does not consume this
			// market's turn: the next one in the same band is tried instead.
			if claim(c) {
				return true
			}
		}
	}

	// Ladder exhausted — backfill on confidence.
	for _, c := range p.candidates {
		if p.taken[c.Prediction.ID] {
			continue
		}
		if claim(c) {
			return true
		}
	}
	return false
}

// finalise orders a market's tips strongest first and numbers them. Rank is
// part of the frozen record — (day, market_code, rank) is unique in free_tips
// — so display order is decided here, not on read.
func (p *marketPicker) finalise() {
	sortByConfidence(p.tips, func(t Tip) (float64, uuid.UUID) {
		return t.ConfidencePct, t.PredictionID
	})
	for i := range p.tips {
		p.tips[i].Rank = i + 1
	}
}

// groupByMarket buckets the window's candidates per market, applying the odds
// floor and ordering each bucket strongest first.
//
// The window is half-open, [from, until): a fixture kicking off at exactly
// midnight belongs to the day starting then, not the one ending.
func groupByMarket(candidates []Candidate, from, until time.Time) map[domain.MarketCode][]Candidate {
	out := make(map[domain.MarketCode][]Candidate, len(domain.MarketCodes))

	// One prediction per (match, market). Postgres permits a second row for a
	// new model_version — a model upgrade adds predictions, it never rewrites
	// published ones — and offering both would let one fixture's single pick
	// spend two appearances against itself.
	seen := make(map[matchMarket]bool, len(candidates))

	for _, c := range sortedForSelection(candidates) {
		if kickoff := c.KickoffAt.UTC(); kickoff.Before(from) || !kickoff.Before(until) {
			continue
		}
		key := matchMarket{c.Prediction.MatchID, c.Prediction.MarketCode}
		if seen[key] {
			continue
		}
		seen[key] = true

		if OddsFromProbability(c.Prediction.ConfidencePct / 100).LessThan(MinPublishableOdds) {
			continue
		}
		out[c.Prediction.MarketCode] = append(out[c.Prediction.MarketCode], c)
	}

	for market := range out {
		sortByConfidence(out[market], func(c Candidate) (float64, uuid.UUID) {
			return c.Prediction.ConfidencePct, c.Prediction.ID
		})
	}
	return out
}

type matchMarket struct {
	match  uuid.UUID
	market domain.MarketCode
}

// sortedForSelection puts candidates in kickoff order, which is the order the
// frontend's dayMatches loop sees them in, and settles same-kickoff ties on
// prediction id so the "first row per (match, market)" rule above is not a
// function of the query plan.
func sortedForSelection(candidates []Candidate) []Candidate {
	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].KickoffAt.Equal(out[j].KickoffAt) {
			return out[i].KickoffAt.Before(out[j].KickoffAt)
		}
		return out[i].Prediction.ID.String() < out[j].Prediction.ID.String()
	})
	return out
}

// sortByConfidence orders strongest first, breaking ties on id. Stable, so the
// kickoff order established by sortedForSelection survives as the second key.
func sortByConfidence[T any](items []T, key func(T) (float64, uuid.UUID)) {
	sort.SliceStable(items, func(i, j int) bool {
		ci, idi := key(items[i])
		cj, idj := key(items[j])
		if ci != cj {
			return ci > cj
		}
		return idi.String() < idj.String()
	})
}

// nextMatchday is the UTC date of the earliest candidate kickoff.
func nextMatchday(candidates []Candidate) (time.Time, bool) {
	var earliest time.Time
	for _, c := range candidates {
		if earliest.IsZero() || c.KickoffAt.Before(earliest) {
			earliest = c.KickoffAt
		}
	}
	if earliest.IsZero() {
		return time.Time{}, false
	}
	return utcDay(earliest), true
}

func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func toTip(c Candidate) Tip {
	return Tip{
		PredictionID:  c.Prediction.ID,
		MatchID:       c.Prediction.MatchID,
		MarketCode:    c.Prediction.MarketCode,
		ConfidencePct: c.Prediction.ConfidencePct,
		Odds:          OddsFromProbability(c.Prediction.ConfidencePct / 100),
	}
}
