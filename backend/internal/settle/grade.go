// Package settle turns a final score into a graded record.
//
// It is the only part of the system whose output is a claim about the past, so
// it is the part most worth being strict about. Settlement never involves a
// model, a heuristic, or an LLM: it reads home_score and away_score from a
// finished match and applies a pure function. If the score is unknown, nothing
// is graded and the pick stays pending. There is no such thing as an inferred
// result.
package settle

import (
	"fmt"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Grade returns the outcome the market settled to for the given final score.
// It is total over its domain: every (market, score) pair has an outcome.
//
// It deliberately does not take the prediction. It computes what actually
// happened; comparing that to the pick is Matches' one-line job. Passing the
// prediction in invites a grader that quietly agrees with it.
//
// Goals are full-time only — extra time and penalties are excluded, so a cup
// tie level at 90 minutes settles as a draw on 1X2.
func Grade(market domain.MarketCode, home, away int) (string, error) {
	if home < 0 || away < 0 {
		return "", fmt.Errorf("settle: negative score %d-%d", home, away)
	}

	switch market {
	case domain.MarketOneXTwo:
		return oneXTwo(home, away), nil

	case domain.MarketDoubleChance:
		// Double Chance does not fit the "one outcome per market" shape: its
		// three selections overlap, so two of them win on every result. It
		// therefore settles to a *side* rather than to one of its own
		// selections, and Matches decides wins by membership.
		switch oneXTwo(home, away) {
		case domain.OutcomeHome:
			return domain.SideHome, nil
		case domain.OutcomeDraw:
			return domain.SideDraw, nil
		default:
			return domain.SideAway, nil
		}

	case domain.MarketBTTS:
		if home > 0 && away > 0 {
			return domain.OutcomeYes, nil
		}
		return domain.OutcomeNo, nil

	case domain.MarketOverUnder15, domain.MarketOverUnder25, domain.MarketOverUnder35:
		line, ok := domain.OverUnderLine[market]
		if !ok {
			return "", fmt.Errorf("settle: no goal line configured for %s", market)
		}
		// Every line is a half-goal, so a push is impossible and every
		// selection resolves to won or lost. Do not add push handling here;
		// add it only if a whole-number line is ever introduced.
		if float64(home+away) > line {
			return domain.OutcomeOver, nil
		}
		return domain.OutcomeUnder, nil

	default:
		return "", fmt.Errorf("settle: unknown market %q", market)
	}
}

// Matches reports whether a published selection won, given the outcome Grade
// produced. Every market except Double Chance is plain equality.
func Matches(market domain.MarketCode, selection, outcome string) bool {
	if market != domain.MarketDoubleChance {
		return selection == outcome
	}
	// '1X' covers a home win and a draw, '12' either win, 'X2' a draw and an
	// away win.
	switch selection {
	case domain.SelectionHomeOrDraw:
		return outcome == domain.SideHome || outcome == domain.SideDraw
	case domain.SelectionHomeOrAway:
		return outcome == domain.SideHome || outcome == domain.SideAway
	case domain.SelectionDrawOrAway:
		return outcome == domain.SideDraw || outcome == domain.SideAway
	default:
		return false
	}
}

// GradeSelection is the two together, for callers that have both and want one
// call. The pair is kept separate above so that Grade can be tested — and
// read — without any pick in scope.
func GradeSelection(market domain.MarketCode, selection string, home, away int) (outcome string, won bool, err error) {
	outcome, err = Grade(market, home, away)
	if err != nil {
		return "", false, err
	}
	return outcome, Matches(market, selection, outcome), nil
}

// ValidSelection reports whether a selection is one the market can actually
// carry. A prediction stored with a selection no market outcome matches would
// grade as a loss forever without anyone noticing.
func ValidSelection(market domain.MarketCode, selection string) bool {
	switch market {
	case domain.MarketOneXTwo:
		return selection == domain.OutcomeHome ||
			selection == domain.OutcomeDraw ||
			selection == domain.OutcomeAway
	case domain.MarketDoubleChance:
		return selection == domain.SelectionHomeOrDraw ||
			selection == domain.SelectionHomeOrAway ||
			selection == domain.SelectionDrawOrAway
	case domain.MarketBTTS:
		return selection == domain.OutcomeYes || selection == domain.OutcomeNo
	case domain.MarketOverUnder15, domain.MarketOverUnder25, domain.MarketOverUnder35:
		return selection == domain.OutcomeOver || selection == domain.OutcomeUnder
	default:
		return false
	}
}

func oneXTwo(home, away int) string {
	switch {
	case home > away:
		return domain.OutcomeHome
	case home < away:
		return domain.OutcomeAway
	default:
		return domain.OutcomeDraw
	}
}
