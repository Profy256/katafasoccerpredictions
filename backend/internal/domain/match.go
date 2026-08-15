package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Region groups leagues for the feed filter. Mirrors Region in
// src/api/types.ts, plus 'asia' — DATA-MODEL.md requires adding that to the
// frontend union when the backend lands.
type Region string

const (
	RegionEurope     Region = "europe"
	RegionEastAfrica Region = "east-africa"
	RegionAfrica     Region = "africa"
	RegionAmericas   Region = "americas"
	RegionAsia       Region = "asia"
)

var Regions = []Region{RegionEurope, RegionEastAfrica, RegionAfrica, RegionAmericas, RegionAsia}

func ParseRegion(s string) (Region, error) {
	for _, r := range Regions {
		if Region(s) == r {
			return r, nil
		}
	}
	return "", fmt.Errorf("unknown region %q", s)
}

// MatchStatus is the full internal vocabulary. The frontend's MatchStatus is
// narrower ('scheduled' | 'finished'); the mapping to it happens at the API
// boundary, not here — see APIStatus.
type MatchStatus string

const (
	StatusScheduled MatchStatus = "scheduled"
	StatusInPlay    MatchStatus = "in_play"
	StatusFinished  MatchStatus = "finished"
	StatusPostponed MatchStatus = "postponed"
	StatusCancelled MatchStatus = "cancelled"
	StatusAbandoned MatchStatus = "abandoned"
)

var MatchStatuses = []MatchStatus{
	StatusScheduled, StatusInPlay, StatusFinished,
	StatusPostponed, StatusCancelled, StatusAbandoned,
}

// ParseMatchStatus rejects anything outside the vocabulary. Provider packages
// call this after their own mapping, so an unrecognised provider status
// becomes an error rather than silently becoming 'scheduled'.
func ParseMatchStatus(s string) (MatchStatus, error) {
	for _, st := range MatchStatuses {
		if MatchStatus(s) == st {
			return st, nil
		}
	}
	return "", fmt.Errorf("unknown match status %q", s)
}

// IsVoid reports whether a match ended without a full-time score that could
// ever be graded. Postponed is deliberately excluded: a postponed match is
// waited for, not voided — see SETTLEMENT.md § Void handling.
func (s MatchStatus) IsVoid() bool {
	return s == StatusCancelled || s == StatusAbandoned
}

// APIStatus narrows to the two values the frontend's MatchStatus accepts.
// in_play presents as scheduled (no final score yet); the non-completions
// present as scheduled too, and are surfaced as voids on their predictions
// rather than as a match state the frontend has no vocabulary for.
func (s MatchStatus) APIStatus() string {
	if s == StatusFinished {
		return "finished"
	}
	return "scheduled"
}

type League struct {
	ID          uuid.UUID `json:"id"`
	Slug        string    `json:"-"`
	Name        string    `json:"name"`
	ShortName   string    `json:"shortName"`
	Country     string    `json:"country"`
	CountryCode string    `json:"countryCode"`
	Tier        int       `json:"tier"`
	Region      Region    `json:"region"`
	IsActive    bool      `json:"-"`
}

type Season struct {
	ID        uuid.UUID
	LeagueID  uuid.UUID
	Label     string
	StartYear int
	IsCurrent bool
}

type Team struct {
	ID        uuid.UUID `json:"id"`
	LeagueID  uuid.UUID `json:"leagueId"`
	Slug      string    `json:"-"`
	Name      string    `json:"name"`
	ShortName string    `json:"shortName"`
}

// Match is a fixture. HomeScore/AwayScore are the 90-minute full-time figures
// and nothing else — extra time and penalties are excluded, because every
// goals market would otherwise be corrupted in knockout rounds.
type Match struct {
	ID         uuid.UUID
	LeagueID   uuid.UUID
	SeasonID   uuid.UUID
	HomeTeamID uuid.UUID
	AwayTeamID uuid.UUID
	KickoffAt  time.Time
	Status     MatchStatus
	HomeScore  *int
	AwayScore  *int
	Round      *int
	FinishedAt *time.Time
}

// HasScore reports whether the match carries a gradable full-time score. The
// schema's CHECK constraint ties this to status = 'finished', so a
// half-written result can never satisfy it.
func (m Match) HasScore() bool {
	return m.Status == StatusFinished && m.HomeScore != nil && m.AwayScore != nil
}
