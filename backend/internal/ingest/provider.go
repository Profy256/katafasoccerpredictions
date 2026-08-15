// Package ingest pulls fixtures and results from the football data providers
// and writes them to Postgres.
//
// The database is the system of record for football history, not the provider.
// Free tiers cap historical seasons, so history is accumulated: every result
// ever fetched is stored permanently, and the model reads only from Postgres.
// A provider is a tap turned on daily, never a database queried at request
// time.
package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Provider names, matching the CHECK constraints on the *_sources tables.
const (
	ProviderFootballData = "football-data"
	ProviderAPIFootball  = "api-football"
)

// RawCompetition is a league as the provider describes it.
type RawCompetition struct {
	ProviderID  string
	Name        string
	Country     string
	CountryCode string
	Season      int
}

// RawTeam is a club as the provider describes it.
type RawTeam struct {
	ProviderID string
	Name       string
	ShortName  string
}

// RawFixture is the seam between provider JSON and the schema.
//
// Provider packages translate their own JSON into this and know nothing about
// the schema; sync.go knows the schema and nothing about provider JSON.
type RawFixture struct {
	ProviderID     string
	CompetitionID  string
	HomeName       string
	AwayName       string
	HomeProviderID string
	AwayProviderID string
	KickoffAt      time.Time // always UTC
	Status         domain.MatchStatus
	// HomeScore and AwayScore are the full-time, 90-minute figures and nothing
	// else. Both providers report extra time and penalties separately; storing
	// an aggregate corrupts every goals market in knockout rounds.
	HomeScore *int
	AwayScore *int
	Round     *int
	Season    int
}

// Provider is one football data source.
type Provider interface {
	Name() string
	Competitions(ctx context.Context) ([]RawCompetition, error)
	Teams(ctx context.Context, competition string, season int) ([]RawTeam, error)
	Fixtures(ctx context.Context, competition string, from, to time.Time) ([]RawFixture, error)
	Results(ctx context.Context, competition string, from, to time.Time) ([]RawFixture, error)
}

// ErrBudgetExhausted is returned when a provider's daily allocation is spent.
//
// This is a normal condition, not something to page on. The job logs what it
// did not fetch and exits successfully; tomorrow's run picks up where it
// stopped. What *is* worth paging on is predictions due within six hours for
// fixtures that never got ingested.
var ErrBudgetExhausted = errors.New("ingest: provider daily budget exhausted")

// ErrUnknownStatus is returned when a provider reports a status this code does
// not recognise.
//
// Mapping an unrecognised status to 'scheduled' would silently resurrect
// abandoned matches and leave finished ones ungraded, so it is an error.
var ErrUnknownStatus = errors.New("ingest: unrecognised provider status")
