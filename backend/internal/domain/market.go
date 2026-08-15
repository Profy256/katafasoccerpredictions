// Package domain holds the core types the rest of the backend is written in
// terms of. It imports nothing from the other internal packages, which is what
// keeps grading and the model testable without a database.
package domain

import "fmt"

// MarketCode identifies a betting market. The MVP six, matching MarketCode in
// src/api/types.ts exactly — these strings cross the API boundary verbatim.
type MarketCode string

const (
	MarketOneXTwo      MarketCode = "ONE_X_TWO"
	MarketDoubleChance MarketCode = "DOUBLE_CHANCE"
	MarketBTTS         MarketCode = "BTTS"
	MarketOverUnder15  MarketCode = "OVER_UNDER_1_5"
	MarketOverUnder25  MarketCode = "OVER_UNDER_2_5"
	MarketOverUnder35  MarketCode = "OVER_UNDER_3_5"
)

// MarketCodes is every market in display order, mirroring MARKET_CODES in
// src/api/types.ts. Order is load-bearing: the free shortlist walks markets in
// this sequence, and the per-fixture appearance cap makes the result depend on
// it.
var MarketCodes = []MarketCode{
	MarketOneXTwo,
	MarketDoubleChance,
	MarketBTTS,
	MarketOverUnder15,
	MarketOverUnder25,
	MarketOverUnder35,
}

// ParseMarketCode validates an incoming market string. An unknown code is an
// error rather than a silent skip — API.md requires a 400 for a typo'd market
// rather than an unfiltered response.
func ParseMarketCode(s string) (MarketCode, error) {
	for _, m := range MarketCodes {
		if MarketCode(s) == m {
			return m, nil
		}
	}
	return "", fmt.Errorf("unknown market code %q", s)
}

// OverUnderLine is the goal line for each Over/Under market, keyed by code.
// Mirrors OVER_UNDER_LINES in src/lib/markets.ts.
//
// Every line is a half-goal, so a push is impossible and every selection
// resolves to won or lost. Whole-number lines would need push handling in
// settle.Grade; do not add one without adding that.
var OverUnderLine = map[MarketCode]float64{
	MarketOverUnder15: 1.5,
	MarketOverUnder25: 2.5,
	MarketOverUnder35: 3.5,
}

// Outcome is one selectable result within a market.
type Outcome struct {
	Value      string `json:"value"`
	Label      string `json:"label"`
	ShortLabel string `json:"shortLabel"`
}

// MarketType is the market_types row, and the MarketType interface in
// src/api/types.ts.
type MarketType struct {
	Code        MarketCode `json:"code"`
	DisplayName string     `json:"displayName"`
	ShortName   string     `json:"shortName"`
	TabLabel    string     `json:"tabLabel"`
	Slug        string     `json:"slug"`
	Outcomes    []Outcome  `json:"outcomes"`
}

// 1X2 outcome values. These are also what settle.Grade returns for
// MarketOneXTwo, and what Double Chance membership is tested against.
const (
	OutcomeHome = "HOME"
	OutcomeDraw = "DRAW"
	OutcomeAway = "AWAY"
)

// Double Chance selection values.
const (
	SelectionHomeOrDraw = "1X"
	SelectionHomeOrAway = "12"
	SelectionDrawOrAway = "X2"
)

// BTTS and Over/Under outcome values.
const (
	OutcomeYes   = "YES"
	OutcomeNo    = "NO"
	OutcomeOver  = "OVER"
	OutcomeUnder = "UNDER"
)

// Double Chance settles to a *side*, not to one of its own selections — two of
// its three selections win on every result. These are the stored
// actual_outcome values for the market, and they match the strings
// settledOutcomeLabel() in src/lib/model.ts already renders, so the frontend
// needs no change at cutover.
const (
	SideHome = "HOME_SIDE"
	SideDraw = "DRAW_SIDE"
	SideAway = "AWAY_SIDE"
)
