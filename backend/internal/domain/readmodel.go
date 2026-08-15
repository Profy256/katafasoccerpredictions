package domain

import (
	"time"

	"github.com/google/uuid"
)

// The read models the API serves. These are the interfaces in
// src/api/types.ts, field for field, so the frontend swap at cutover is a
// change of transport only. JSON tags are camelCase for the same reason, and
// timestamps serialise as RFC3339 UTC.

// APIMatch is Match in src/api/types.ts. Status is narrowed to the two values
// the frontend understands, and Round is non-null there, so an unknown round
// serialises as 0 rather than omitting the field.
type APIMatch struct {
	ID         uuid.UUID `json:"id"`
	LeagueID   uuid.UUID `json:"leagueId"`
	HomeTeamID uuid.UUID `json:"homeTeamId"`
	AwayTeamID uuid.UUID `json:"awayTeamId"`
	KickoffAt  time.Time `json:"kickoffAt"`
	Status     string    `json:"status"`
	HomeScore  *int      `json:"homeScore"`
	AwayScore  *int      `json:"awayScore"`
	Round      int       `json:"round"`
}

// ToAPI narrows a Match for the wire.
func (m Match) ToAPI() APIMatch {
	round := 0
	if m.Round != nil {
		round = *m.Round
	}
	return APIMatch{
		ID:         m.ID,
		LeagueID:   m.LeagueID,
		HomeTeamID: m.HomeTeamID,
		AwayTeamID: m.AwayTeamID,
		KickoffAt:  m.KickoffAt.UTC(),
		Status:     m.Status.APIStatus(),
		HomeScore:  m.HomeScore,
		AwayScore:  m.AwayScore,
		Round:      round,
	}
}

// APIPrediction is Prediction in src/api/types.ts.
type APIPrediction struct {
	ID              uuid.UUID            `json:"id"`
	MatchID         uuid.UUID            `json:"matchId"`
	MarketType      MarketCode           `json:"marketType"`
	PredictionValue string               `json:"predictionValue"`
	ConfidencePct   float64              `json:"confidencePct"`
	ModelVersion    string               `json:"modelVersion"`
	CreatedAt       time.Time            `json:"createdAt"`
	Distribution    []OutcomeProbability `json:"distribution"`
}

func (p Prediction) ToAPI() APIPrediction {
	distribution := p.Distribution
	if distribution == nil {
		distribution = []OutcomeProbability{}
	}
	return APIPrediction{
		ID:              p.ID,
		MatchID:         p.MatchID,
		MarketType:      p.MarketCode,
		PredictionValue: p.PredictionValue,
		ConfidencePct:   p.ConfidencePct,
		ModelVersion:    p.ModelVersion,
		CreatedAt:       p.CreatedAt.UTC(),
		Distribution:    distribution,
	}
}

// APIPredictionResult is PredictionResult in src/api/types.ts.
type APIPredictionResult struct {
	PredictionID  uuid.UUID `json:"predictionId"`
	ActualOutcome string    `json:"actualOutcome"`
	WasCorrect    bool      `json:"wasCorrect"`
	SettledAt     time.Time `json:"settledAt"`
}

func (r PredictionResult) ToAPI() APIPredictionResult {
	return APIPredictionResult{
		PredictionID:  r.PredictionID,
		ActualOutcome: r.ActualOutcome,
		WasCorrect:    r.WasCorrect,
		SettledAt:     r.SettledAt.UTC(),
	}
}

// MatchWithPredictions is a fixture plus its published predictions, as served
// to the feed.
type MatchWithPredictions struct {
	Match       APIMatch              `json:"match"`
	League      League                `json:"league"`
	HomeTeam    Team                  `json:"homeTeam"`
	AwayTeam    Team                  `json:"awayTeam"`
	Predictions []APIPrediction       `json:"predictions"`
	Results     []APIPredictionResult `json:"results,omitempty"`
}

// MatchDetail adds the reasoning snapshot.
type MatchDetail struct {
	MatchWithPredictions
	Reasoning MatchReasoning `json:"reasoning"`
}

// SettledPrediction is one row of the unfiltered graded ledger — wins and
// losses alike, which is the receipt behind the headline accuracy figures.
type SettledPrediction struct {
	Prediction APIPrediction       `json:"prediction"`
	Result     APIPredictionResult `json:"result"`
	Match      APIMatch            `json:"match"`
	League     League              `json:"league"`
	HomeTeam   Team                `json:"homeTeam"`
	AwayTeam   Team                `json:"awayTeam"`
}

// AccuracyBucket is one aggregate row of the accuracy dashboard.
type AccuracyBucket struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Total   int     `json:"total"`
	Correct int     `json:"correct"`
	HitRate float64 `json:"hitRate"` // 0..1
}

// NewAccuracyBucket keeps the zero-total case at a hit rate of 0 rather than
// NaN, matching the frontend's `rows.length ? correct / rows.length : 0`.
func NewAccuracyBucket(key, label string, total, correct int) AccuracyBucket {
	rate := 0.0
	if total > 0 {
		rate = float64(correct) / float64(total)
	}
	return AccuracyBucket{Key: key, Label: label, Total: total, Correct: correct, HitRate: rate}
}

type AccuracyPoint struct {
	Date              string  `json:"date"`
	CumulativeHitRate float64 `json:"cumulativeHitRate"`
	DailyHitRate      float64 `json:"dailyHitRate"`
	Settled           int     `json:"settled"`
}

type AccuracySummary struct {
	Overall          AccuracyBucket   `json:"overall"`
	ByMarket         []AccuracyBucket `json:"byMarket"`
	ByLeague         []AccuracyBucket `json:"byLeague"`
	ByConfidenceBand []AccuracyBucket `json:"byConfidenceBand"`
	Timeline         []AccuracyPoint  `json:"timeline"`
	ModelVersion     string           `json:"modelVersion"`
	FirstSettledAt   string           `json:"firstSettledAt"`
	LastSettledAt    string           `json:"lastSettledAt"`
}

// FreeTip is one published shortlist pick.
type FreeTip struct {
	Match      APIMatch      `json:"match"`
	League     League        `json:"league"`
	HomeTeam   Team          `json:"homeTeam"`
	AwayTeam   Team          `json:"awayTeam"`
	Prediction APIPrediction `json:"prediction"`
	// Odds is indicative, derived from the model price. Serialised as a number
	// because the frontend types it as one; it is decimal everywhere it is
	// stored or multiplied.
	Odds float64 `json:"odds"`
	// Result is present once the pick has been graded, so "yesterday's tips
	// went 4 from 5" can be shown from frozen rows.
	Result *APIPredictionResult `json:"result,omitempty"`
}

type FreeTipGroup struct {
	Market MarketCode `json:"market"`
	Tips   []FreeTip  `json:"tips"`
}

type FreeTipsDay struct {
	Date      string         `json:"date"`
	Groups    []FreeTipGroup `json:"groups"`
	TotalTips int            `json:"totalTips"`
}

// FreeTipsHistoryDay is one past day with its results attached.
type FreeTipsHistoryDay struct {
	Date      string         `json:"date"`
	TotalTips int            `json:"totalTips"`
	Settled   int            `json:"settled"`
	Correct   int            `json:"correct"`
	HitRate   float64        `json:"hitRate"`
	Groups    []FreeTipGroup `json:"groups"`
}

// APISlip is Slip in src/api/types.ts. Status there is 'open' | 'settled'
// only: a draft is never served, and a void slip presents as settled with zero
// winning tips.
type APISlip struct {
	ID          uuid.UUID   `json:"id"`
	PackageCode PackageCode `json:"packageCode"`
	Title       string      `json:"title"`
	AnalystIDs  []uuid.UUID `json:"analystIds"`
	PublishedAt time.Time   `json:"publishedAt"`
	PriceUGX    UGX         `json:"priceUgx"`
	Status      string      `json:"status"`
	TipCount    int         `json:"tipCount"`
	TotalOdds   float64     `json:"totalOdds"`
	WonTips     *int        `json:"wonTips,omitempty"`
	// SettledOdds is the accumulator after void legs were removed. Shown
	// alongside TotalOdds rather than replacing it: buyers were advertised the
	// published price and it must keep showing.
	SettledOdds *float64 `json:"settledOdds,omitempty"`
}

// APITip is Tip in src/api/types.ts.
type APITip struct {
	ID             uuid.UUID   `json:"id"`
	SlipID         uuid.UUID   `json:"slipId"`
	AnalystID      uuid.UUID   `json:"analystId"`
	MatchID        *uuid.UUID  `json:"matchId"`
	FixtureLabel   string      `json:"fixtureLabel"`
	MarketLabel    string      `json:"marketLabel"`
	SelectionLabel string      `json:"selectionLabel"`
	MarketType     *MarketCode `json:"marketType"`
	SelectionValue *string     `json:"selectionValue"`
	Odds           float64     `json:"odds"`
	KickoffAt      time.Time   `json:"kickoffAt"`
	Note           *string     `json:"note,omitempty"`
}

type APITipResult struct {
	TipID         uuid.UUID `json:"tipId"`
	WasCorrect    bool      `json:"wasCorrect"`
	ActualOutcome string    `json:"actualOutcome"`
	SettledAt     time.Time `json:"settledAt"`
	SettledBy     SettledBy `json:"settledBy"`
}

// SlipWithTips is a slip and, if the viewer is entitled to them, its picks.
//
// Tips is empty for an unpaid viewer because the *database returned no rows* —
// the entitlement is folded into the query's WHERE clause. There is no
// filtering step in Go that could be forgotten.
type SlipWithTips struct {
	APISlip
	Tips     []APITip       `json:"tips"`
	Results  []APITipResult `json:"results,omitempty"`
	Analysts []Analyst      `json:"analysts"`
	Package  Package        `json:"package"`
	Unlocked bool           `json:"unlocked"`
}

// AnalystRecord is an analyst's public record.
type AnalystRecord struct {
	Analyst     Analyst          `json:"analyst"`
	Overall     AccuracyBucket   `json:"overall"`
	Last30Days  AccuracyBucket   `json:"last30Days"`
	ByPackage   []AccuracyBucket `json:"byPackage"`
	ProfitUnits float64          `json:"profitUnits"`
	ROI         float64          `json:"roi"`
	AverageOdds float64          `json:"averageOdds"`
	RecentSlips []APISlip        `json:"recentSlips"`
}
