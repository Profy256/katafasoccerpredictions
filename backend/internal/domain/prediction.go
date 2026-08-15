package domain

import (
	"time"

	"github.com/google/uuid"
)

// OutcomeProbability is one entry of a market's probability distribution.
// Probability is 0..1.
type OutcomeProbability struct {
	Value       string  `json:"value"`
	Probability float64 `json:"probability"`
}

// Prediction is a published model pick. It is immutable once written: there is
// no update path anywhere in this codebase, and the database enforces that
// with a trigger rather than trusting it.
type Prediction struct {
	ID              uuid.UUID
	MatchID         uuid.UUID
	MarketCode      MarketCode
	PredictionValue string
	// ConfidencePct is the model's own probability for PredictionValue, as a
	// percentage. It is never a separately fabricated number.
	ConfidencePct float64
	Distribution  []OutcomeProbability
	ModelVersion  string
	CreatedAt     time.Time
}

// PredictionResult is a graded prediction. Primary-keyed on PredictionID in
// the schema, which is what makes settling twice a conflict rather than a
// double-counted win.
type PredictionResult struct {
	PredictionID  uuid.UUID
	ActualOutcome string
	WasCorrect    bool
	SettledAt     time.Time
}

// FormChar is a single result from one team's perspective.
type FormChar string

const (
	FormWin  FormChar = "W"
	FormDraw FormChar = "D"
	FormLoss FormChar = "L"
)

// Venue is which side of a fixture a team's strength is measured at. Strengths
// are venue-split because home and away performance differ systematically.
type Venue string

const (
	VenueHome Venue = "home"
	VenueAway Venue = "away"
)

// FormSummary is rolling form for one team at one venue, as the model uses it.
// Mirrors FormSummary in src/api/types.ts.
type FormSummary struct {
	TeamID       uuid.UUID  `json:"teamId"`
	Venue        Venue      `json:"venue"`
	Played       int        `json:"played"`
	Wins         int        `json:"wins"`
	Draws        int        `json:"draws"`
	Losses       int        `json:"losses"`
	GoalsFor     int        `json:"goalsFor"`
	GoalsAgainst int        `json:"goalsAgainst"`
	Recent       []FormChar `json:"recent"`
	// AttackStrength and DefenseStrength are multiples of the league average,
	// where 1.0 is exactly average.
	AttackStrength  float64 `json:"attackStrength"`
	DefenseStrength float64 `json:"defenseStrength"`
}

type HeadToHeadMatch struct {
	MatchID    uuid.UUID `json:"matchId"`
	KickoffAt  time.Time `json:"kickoffAt"`
	HomeTeamID uuid.UUID `json:"homeTeamId"`
	AwayTeamID uuid.UUID `json:"awayTeamId"`
	HomeScore  int       `json:"homeScore"`
	AwayScore  int       `json:"awayScore"`
}

type Scoreline struct {
	Home        int     `json:"home"`
	Away        int     `json:"away"`
	Probability float64 `json:"probability"`
}

// SampleSize is how much history the model had. Stored and served because the
// frontend renders it: a pick drawn from eight matches must not look like a
// pick drawn from two hundred.
type SampleSize struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

// MatchReasoning is a snapshot of what the model saw at the time it decided,
// never a recomputation. Recomputing from today's data would show a different,
// flattering picture.
type MatchReasoning struct {
	MatchID       uuid.UUID         `json:"matchId"`
	XGHome        float64           `json:"xgHome"`
	XGAway        float64           `json:"xgAway"`
	HomeForm      FormSummary       `json:"homeForm"`
	AwayForm      FormSummary       `json:"awayForm"`
	HeadToHead    []HeadToHeadMatch `json:"headToHead"`
	TopScorelines []Scoreline       `json:"topScorelines"`
	ModelVersion  string            `json:"modelVersion"`
	SampleSize    SampleSize        `json:"sampleSize"`
}
