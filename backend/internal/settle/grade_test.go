package settle_test

import (
	"fmt"
	"testing"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

// This is the highest-value test file in the repo. A bug in grading silently
// rewrites the published track record — nothing crashes, no alert fires, and
// the accuracy dashboard reports a number that is simply not true.
//
// So it is exhaustive rather than representative: the explicit table pins the
// cases a human would check by hand, and the sweep below asserts the
// structural properties across every scoreline the Poisson matrix can produce.

func TestGradeKnownScorelines(t *testing.T) {
	cases := []struct {
		market     domain.MarketCode
		home, away int
		want       string
	}{
		// 1X2 — the boundary is equality, not a margin.
		{domain.MarketOneXTwo, 1, 0, domain.OutcomeHome},
		{domain.MarketOneXTwo, 0, 0, domain.OutcomeDraw},
		{domain.MarketOneXTwo, 3, 3, domain.OutcomeDraw},
		{domain.MarketOneXTwo, 0, 1, domain.OutcomeAway},
		{domain.MarketOneXTwo, 7, 0, domain.OutcomeHome},

		// Double Chance settles to a side; two of its three selections win.
		{domain.MarketDoubleChance, 2, 1, domain.SideHome},
		{domain.MarketDoubleChance, 1, 1, domain.SideDraw},
		{domain.MarketDoubleChance, 0, 2, domain.SideAway},

		// BTTS needs both sides on the scoresheet, so any nil-nil-side result
		// is NO regardless of how many the other side scored.
		{domain.MarketBTTS, 1, 1, domain.OutcomeYes},
		{domain.MarketBTTS, 4, 3, domain.OutcomeYes},
		{domain.MarketBTTS, 0, 0, domain.OutcomeNo},
		{domain.MarketBTTS, 5, 0, domain.OutcomeNo},
		{domain.MarketBTTS, 0, 5, domain.OutcomeNo},

		// Over/Under 1.5 — the line sits between 1 and 2 total goals.
		{domain.MarketOverUnder15, 0, 0, domain.OutcomeUnder},
		{domain.MarketOverUnder15, 1, 0, domain.OutcomeUnder},
		{domain.MarketOverUnder15, 1, 1, domain.OutcomeOver},
		{domain.MarketOverUnder15, 2, 0, domain.OutcomeOver},

		// Over/Under 2.5 — between 2 and 3.
		{domain.MarketOverUnder25, 1, 1, domain.OutcomeUnder},
		{domain.MarketOverUnder25, 2, 0, domain.OutcomeUnder},
		{domain.MarketOverUnder25, 2, 1, domain.OutcomeOver},
		{domain.MarketOverUnder25, 3, 0, domain.OutcomeOver},

		// Over/Under 3.5 — between 3 and 4.
		{domain.MarketOverUnder35, 2, 1, domain.OutcomeUnder},
		{domain.MarketOverUnder35, 3, 0, domain.OutcomeUnder},
		{domain.MarketOverUnder35, 2, 2, domain.OutcomeOver},
		{domain.MarketOverUnder35, 4, 0, domain.OutcomeOver},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d-%d", tc.market, tc.home, tc.away), func(t *testing.T) {
			got, err := settle.Grade(tc.market, tc.home, tc.away)
			if err != nil {
				t.Fatalf("Grade: %v", err)
			}
			if got != tc.want {
				t.Errorf("Grade(%s, %d, %d) = %q, want %q", tc.market, tc.home, tc.away, got, tc.want)
			}
		})
	}
}

// maxGoals matches the Poisson matrix truncation in internal/model, so the
// sweep covers every scoreline the model can actually produce a pick for.
const maxGoals = 10

// Every market must produce exactly one valid outcome for every scoreline. A
// market that errored or returned an unrecognised string on some corner of the
// score space would leave those predictions permanently ungraded.
func TestGradeIsTotalOverEveryScoreline(t *testing.T) {
	for _, market := range domain.MarketCodes {
		for home := 0; home <= maxGoals; home++ {
			for away := 0; away <= maxGoals; away++ {
				outcome, err := settle.Grade(market, home, away)
				if err != nil {
					t.Fatalf("Grade(%s, %d, %d): %v", market, home, away, err)
				}
				if outcome == "" {
					t.Fatalf("Grade(%s, %d, %d) returned an empty outcome", market, home, away)
				}
			}
		}
	}
}

// The over/under lines are all half-goals, so every selection resolves to won
// or lost and a push is impossible. If this ever fails, a whole-number line has
// been introduced and push handling is now genuinely required.
func TestOverUnderNeverPushes(t *testing.T) {
	for _, market := range []domain.MarketCode{
		domain.MarketOverUnder15, domain.MarketOverUnder25, domain.MarketOverUnder35,
	} {
		for home := 0; home <= maxGoals; home++ {
			for away := 0; away <= maxGoals; away++ {
				outcome, err := settle.Grade(market, home, away)
				if err != nil {
					t.Fatalf("Grade(%s, %d, %d): %v", market, home, away, err)
				}
				over := settle.Matches(market, domain.OutcomeOver, outcome)
				under := settle.Matches(market, domain.OutcomeUnder, outcome)
				if over == under {
					t.Errorf("%s at %d-%d: OVER and UNDER both %v — that is a push",
						market, home, away, over)
				}
			}
		}
	}
}

// Exactly two of Double Chance's three selections win on every result, and the
// pair that wins is the one containing the 1X2 outcome. This is the market
// most likely to be graded wrong, because equality looks like it should work.
func TestDoubleChanceWinsByMembership(t *testing.T) {
	for home := 0; home <= maxGoals; home++ {
		for away := 0; away <= maxGoals; away++ {
			result, err := settle.Grade(domain.MarketOneXTwo, home, away)
			if err != nil {
				t.Fatalf("Grade 1X2 %d-%d: %v", home, away, err)
			}
			outcome, err := settle.Grade(domain.MarketDoubleChance, home, away)
			if err != nil {
				t.Fatalf("Grade DC %d-%d: %v", home, away, err)
			}

			want := map[string]bool{
				domain.SelectionHomeOrDraw: result == domain.OutcomeHome || result == domain.OutcomeDraw,
				domain.SelectionHomeOrAway: result == domain.OutcomeHome || result == domain.OutcomeAway,
				domain.SelectionDrawOrAway: result == domain.OutcomeDraw || result == domain.OutcomeAway,
			}

			won := 0
			for selection, expected := range want {
				got := settle.Matches(domain.MarketDoubleChance, selection, outcome)
				if got != expected {
					t.Errorf("DC %s at %d-%d (1X2 %s): got %v, want %v",
						selection, home, away, result, got, expected)
				}
				if got {
					won++
				}
			}
			if won != 2 {
				t.Errorf("DC at %d-%d: %d selections won, want exactly 2", home, away, won)
			}
		}
	}
}

// Markets are derived from one scoreline matrix so that they can never
// contradict each other. Grading must preserve that: if Over 2.5 settled, Over
// 1.5 must have settled too, and BTTS YES implies at least two goals.
func TestGradedMarketsStayConsistentWithEachOther(t *testing.T) {
	for home := 0; home <= maxGoals; home++ {
		for away := 0; away <= maxGoals; away++ {
			over := func(market domain.MarketCode) bool {
				outcome, err := settle.Grade(market, home, away)
				if err != nil {
					t.Fatalf("Grade(%s, %d, %d): %v", market, home, away, err)
				}
				return outcome == domain.OutcomeOver
			}

			o15, o25, o35 := over(domain.MarketOverUnder15), over(domain.MarketOverUnder25), over(domain.MarketOverUnder35)
			if o35 && !o25 {
				t.Errorf("%d-%d: over 3.5 but not over 2.5", home, away)
			}
			if o25 && !o15 {
				t.Errorf("%d-%d: over 2.5 but not over 1.5", home, away)
			}

			btts, err := settle.Grade(domain.MarketBTTS, home, away)
			if err != nil {
				t.Fatalf("Grade BTTS %d-%d: %v", home, away, err)
			}
			if btts == domain.OutcomeYes && !o15 {
				t.Errorf("%d-%d: both teams scored but the game was under 1.5 goals", home, away)
			}

			result, err := settle.Grade(domain.MarketOneXTwo, home, away)
			if err != nil {
				t.Fatalf("Grade 1X2 %d-%d: %v", home, away, err)
			}
			if result == domain.OutcomeDraw && home != away {
				t.Errorf("%d-%d graded as a draw", home, away)
			}
		}
	}
}

// A cup tie level after 90 minutes is a draw, whatever happened in extra time
// or on penalties. Ingestion stores the 90-minute figure and nothing else;
// this test documents the contract that depends on it.
func TestFullTimeOnlyForKnockoutTies(t *testing.T) {
	// 1-1 after 90, home side won 5-4 on penalties. The stored score is 1-1.
	outcome, err := settle.Grade(domain.MarketOneXTwo, 1, 1)
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if outcome != domain.OutcomeDraw {
		t.Errorf("a tie level at 90 minutes graded as %q, want DRAW", outcome)
	}
	if settle.Matches(domain.MarketOneXTwo, domain.OutcomeHome, outcome) {
		t.Error("a HOME pick won a match that was level after 90 minutes")
	}
}

func TestGradeRejectsUnknownMarketsAndScores(t *testing.T) {
	if _, err := settle.Grade("CORRECT_SCORE", 1, 0); err == nil {
		t.Error("graded a market that does not exist")
	}
	if _, err := settle.Grade(domain.MarketOneXTwo, -1, 0); err == nil {
		t.Error("graded a negative score")
	}
}

func TestMatchesRejectsSelectionsTheMarketCannotCarry(t *testing.T) {
	outcome, err := settle.Grade(domain.MarketDoubleChance, 2, 0)
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	// 'HOME' is a 1X2 selection, not a Double Chance one. It must not win by
	// accidentally resembling the side the market settled to.
	if settle.Matches(domain.MarketDoubleChance, domain.OutcomeHome, outcome) {
		t.Error("a 1X2 selection won a Double Chance market")
	}
	if settle.Matches(domain.MarketDoubleChance, "", outcome) {
		t.Error("an empty selection won")
	}
}

func TestValidSelection(t *testing.T) {
	valid := map[domain.MarketCode][]string{
		domain.MarketOneXTwo:      {domain.OutcomeHome, domain.OutcomeDraw, domain.OutcomeAway},
		domain.MarketDoubleChance: {domain.SelectionHomeOrDraw, domain.SelectionHomeOrAway, domain.SelectionDrawOrAway},
		domain.MarketBTTS:         {domain.OutcomeYes, domain.OutcomeNo},
		domain.MarketOverUnder15:  {domain.OutcomeOver, domain.OutcomeUnder},
		domain.MarketOverUnder25:  {domain.OutcomeOver, domain.OutcomeUnder},
		domain.MarketOverUnder35:  {domain.OutcomeOver, domain.OutcomeUnder},
	}

	for _, market := range domain.MarketCodes {
		for _, selection := range valid[market] {
			if !settle.ValidSelection(market, selection) {
				t.Errorf("%s rejected its own selection %q", market, selection)
			}
		}
		for _, selection := range []string{"", "MAYBE", domain.SideHome} {
			if settle.ValidSelection(market, selection) {
				t.Errorf("%s accepted %q", market, selection)
			}
		}
	}

	// Every valid selection must also be winnable: a selection the market
	// accepts but that never wins on any scoreline is a silent permanent loss.
	for _, market := range domain.MarketCodes {
		for _, selection := range valid[market] {
			everWon := false
			for home := 0; home <= maxGoals && !everWon; home++ {
				for away := 0; away <= maxGoals; away++ {
					outcome, err := settle.Grade(market, home, away)
					if err != nil {
						t.Fatalf("Grade: %v", err)
					}
					if settle.Matches(market, selection, outcome) {
						everWon = true
						break
					}
				}
			}
			if !everWon {
				t.Errorf("%s selection %q never wins on any scoreline", market, selection)
			}
		}
	}
}

func TestGradeSelectionAgreesWithItsParts(t *testing.T) {
	for _, market := range domain.MarketCodes {
		for home := 0; home <= 5; home++ {
			for away := 0; away <= 5; away++ {
				outcome, err := settle.Grade(market, home, away)
				if err != nil {
					t.Fatalf("Grade: %v", err)
				}
				for _, selection := range []string{
					domain.OutcomeHome, domain.OutcomeDraw, domain.OutcomeAway,
					domain.SelectionHomeOrDraw, domain.SelectionHomeOrAway, domain.SelectionDrawOrAway,
					domain.OutcomeYes, domain.OutcomeNo, domain.OutcomeOver, domain.OutcomeUnder,
				} {
					gotOutcome, gotWon, err := settle.GradeSelection(market, selection, home, away)
					if err != nil {
						t.Fatalf("GradeSelection: %v", err)
					}
					if gotOutcome != outcome || gotWon != settle.Matches(market, selection, outcome) {
						t.Errorf("GradeSelection(%s, %s, %d, %d) disagrees with Grade+Matches",
							market, selection, home, away)
					}
				}
			}
		}
	}
}
