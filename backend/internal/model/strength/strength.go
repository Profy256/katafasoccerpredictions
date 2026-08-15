// Package strength estimates team attack and defence.
//
// Strengths are venue-split and recency-weighted, expressed as multiples of
// the league average (1.0 == exactly average). They feed straight into the
// expected-goals calculation the Poisson matrix is built from.
//
// This is a port of src/lib/model.ts. Every constant below is carried across
// unchanged; changing one changes every published probability.
package strength

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// PlayedMatch is one finished fixture, reduced to what strength estimation
// needs. Ordering is by KickoffAt, oldest first.
type PlayedMatch struct {
	MatchID    uuid.UUID
	HomeTeamID uuid.UUID
	AwayTeamID uuid.UUID
	HomeScore  int
	AwayScore  int
	KickoffAt  time.Time
}

// Baselines are what an average team scores at each venue in this league.
type Baselines struct {
	HomeGoals float64
	AwayGoals float64
	Matches   int
}

// DefaultBaselines are the fallbacks used until a league has enough played
// matches to speak for itself.
var DefaultBaselines = Baselines{HomeGoals: 1.5, AwayGoals: 1.15, Matches: 0}

const (
	// FormWindow is how many matches at a given venue are considered.
	FormWindow = 12

	// RecencyHalfLife: weight halves every this many matches back, biasing
	// toward recent form.
	RecencyHalfLife = 8

	// PriorWeight is pseudo-matches of "league average" mixed into every
	// estimate. It stops a team with two freak results from being handed an
	// extreme strength rating.
	PriorWeight = 3.5

	// xG is clamped to this range; the Poisson tail gets silly outside it.
	MinXG = 0.15
	MaxXG = 4.75
)

// ComputeBaselines averages goals per venue across a league's played history.
func ComputeBaselines(matches []PlayedMatch) Baselines {
	if len(matches) == 0 {
		return DefaultBaselines
	}
	home, away := 0, 0
	for _, m := range matches {
		home += m.HomeScore
		away += m.AwayScore
	}
	n := float64(len(matches))
	return Baselines{
		HomeGoals: float64(home) / n,
		AwayGoals: float64(away) / n,
		Matches:   len(matches),
	}
}

// VenueStrength is one team's record and strength at one venue.
type VenueStrength struct {
	// Attack is goals scored relative to league average at this venue; higher
	// is better. Defense is goals conceded relative to average; lower is
	// better.
	Attack       float64
	Defense      float64
	Games        int
	GoalsFor     int
	GoalsAgainst int
	Wins         int
	Draws        int
	Losses       int
}

var emptyVenue = VenueStrength{Attack: 1, Defense: 1}

type TeamStrengths struct {
	TeamID     uuid.UUID
	Home       VenueStrength
	Away       VenueStrength
	TotalGames int
}

// venueStrength expects history for this team only, oldest first.
func venueStrength(teamID uuid.UUID, history []PlayedMatch, venue domain.Venue, baselines Baselines) VenueStrength {
	atVenue := make([]PlayedMatch, 0, len(history))
	for _, m := range history {
		if (venue == domain.VenueHome && m.HomeTeamID == teamID) ||
			(venue == domain.VenueAway && m.AwayTeamID == teamID) {
			atVenue = append(atVenue, m)
		}
	}
	window := atVenue
	if len(window) > FormWindow {
		window = window[len(window)-FormWindow:]
	}
	if len(window) == 0 {
		return emptyVenue
	}

	// Reference points: what an average team scores and concedes at this venue.
	refFor, refAgainst := baselines.HomeGoals, baselines.AwayGoals
	if venue == domain.VenueAway {
		refFor, refAgainst = baselines.AwayGoals, baselines.HomeGoals
	}

	var weight, weightedFor, weightedAgainst float64
	var goalsFor, goalsAgainst, wins, draws, losses int

	for index, m := range window {
		matchesAgo := len(window) - 1 - index
		w := math.Pow(0.5, float64(matchesAgo)/RecencyHalfLife)

		gf, ga := m.HomeScore, m.AwayScore
		if venue == domain.VenueAway {
			gf, ga = m.AwayScore, m.HomeScore
		}

		weight += w
		weightedFor += w * float64(gf)
		weightedAgainst += w * float64(ga)
		goalsFor += gf
		goalsAgainst += ga
		switch {
		case gf > ga:
			wins++
		case gf == ga:
			draws++
		default:
			losses++
		}
	}

	rawAttack, rawDefense := 1.0, 1.0
	if refFor > 0 {
		rawAttack = weightedFor / weight / refFor
	}
	if refAgainst > 0 {
		rawDefense = weightedAgainst / weight / refAgainst
	}

	// Shrink toward league average in proportion to how little evidence we
	// have. This is what stops a three-match sample producing a confident pick.
	denom := weight + PriorWeight
	return VenueStrength{
		Attack:       (weight*rawAttack + PriorWeight) / denom,
		Defense:      (weight*rawDefense + PriorWeight) / denom,
		Games:        len(window),
		GoalsFor:     goalsFor,
		GoalsAgainst: goalsAgainst,
		Wins:         wins,
		Draws:        draws,
		Losses:       losses,
	}
}

// ComputeTeamStrengths filters a league's history down to this team and splits
// it by venue. leagueHistory must be oldest first and must contain only
// matches that kicked off before the fixture being predicted — see
// AssertWalkForward.
func ComputeTeamStrengths(teamID uuid.UUID, leagueHistory []PlayedMatch, baselines Baselines) TeamStrengths {
	history := make([]PlayedMatch, 0, len(leagueHistory))
	for _, m := range leagueHistory {
		if m.HomeTeamID == teamID || m.AwayTeamID == teamID {
			history = append(history, m)
		}
	}
	return TeamStrengths{
		TeamID:     teamID,
		Home:       venueStrength(teamID, history, domain.VenueHome, baselines),
		Away:       venueStrength(teamID, history, domain.VenueAway, baselines),
		TotalGames: len(history),
	}
}

type ExpectedGoals struct {
	Home float64
	Away float64
}

// Expected: a team's expected goals is its attacking strength times the
// opponent's defensive weakness times the league's venue baseline.
func Expected(home, away TeamStrengths, baselines Baselines) ExpectedGoals {
	clamp := func(v float64) float64 {
		return math.Min(MaxXG, math.Max(MinXG, v))
	}
	return ExpectedGoals{
		Home: clamp(home.Home.Attack * away.Away.Defense * baselines.HomeGoals),
		Away: clamp(away.Away.Attack * home.Home.Defense * baselines.AwayGoals),
	}
}

// RecentForm is the most recent results first, as W/D/L from teamID's
// perspective, across both venues.
func RecentForm(teamID uuid.UUID, leagueHistory []PlayedMatch, limit int) []domain.FormChar {
	involved := make([]PlayedMatch, 0, len(leagueHistory))
	for _, m := range leagueHistory {
		if m.HomeTeamID == teamID || m.AwayTeamID == teamID {
			involved = append(involved, m)
		}
	}
	if len(involved) > limit {
		involved = involved[len(involved)-limit:]
	}

	out := make([]domain.FormChar, 0, len(involved))
	for i := len(involved) - 1; i >= 0; i-- {
		m := involved[i]
		gf, ga := m.HomeScore, m.AwayScore
		if m.AwayTeamID == teamID {
			gf, ga = m.AwayScore, m.HomeScore
		}
		switch {
		case gf > ga:
			out = append(out, domain.FormWin)
		case gf == ga:
			out = append(out, domain.FormDraw)
		default:
			out = append(out, domain.FormLoss)
		}
	}
	return out
}

// FormSummaryFor builds the read model the match detail page renders.
func FormSummaryFor(s TeamStrengths, venue domain.Venue, recent []domain.FormChar) domain.FormSummary {
	v := s.Home
	if venue == domain.VenueAway {
		v = s.Away
	}
	return domain.FormSummary{
		TeamID:          s.TeamID,
		Venue:           venue,
		Played:          v.Games,
		Wins:            v.Wins,
		Draws:           v.Draws,
		Losses:          v.Losses,
		GoalsFor:        v.GoalsFor,
		GoalsAgainst:    v.GoalsAgainst,
		Recent:          recent,
		AttackStrength:  v.Attack,
		DefenseStrength: v.Defense,
	}
}
