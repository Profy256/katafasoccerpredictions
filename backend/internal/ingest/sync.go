package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Syncer writes provider fixtures into Postgres.
//
// It knows the schema and nothing about provider JSON; the provider packages
// know their JSON and nothing about the schema. RawFixture is the seam.
type Syncer struct {
	DB        *postgres.DB
	Budget    *Budget
	Log       *slog.Logger
	Providers map[string]Provider
}

// SyncStats is what one pass did, for the job log.
type SyncStats struct {
	Fetched          int
	Created          int
	Updated          int
	Skipped          int
	FuzzyTeamMatches int
	BudgetExhausted  bool
}

// SyncFixtures pulls fixtures in a window for every league routed to the given
// provider. Pass an empty provider name for all of them.
func (s *Syncer) SyncFixtures(ctx context.Context, provider string, from, to time.Time) (SyncStats, error) {
	return s.sync(ctx, provider, from, to, false)
}

// SyncResults pulls finals for the window.
func (s *Syncer) SyncResults(ctx context.Context, provider string, from, to time.Time) (SyncStats, error) {
	return s.sync(ctx, provider, from, to, true)
}

func (s *Syncer) sync(ctx context.Context, provider string, from, to time.Time, results bool) (SyncStats, error) {
	var stats SyncStats

	routes, err := s.DB.CompetitionRoutes(ctx, provider)
	if err != nil {
		return stats, err
	}

	for _, route := range routes {
		client, ok := s.Providers[route.Provider]
		if !ok {
			s.Log.Warn("no client configured for provider",
				"provider", route.Provider, "league", route.LeagueName)
			continue
		}

		if err := s.Budget.Acquire(ctx, route.Provider); err != nil {
			if errors.Is(err, ErrBudgetExhausted) {
				// Budget exhaustion is a normal condition. Record what was not
				// fetched and stop; tomorrow's run picks up where this left
				// off.
				s.Log.Info("provider budget exhausted, stopping early",
					"provider", route.Provider, "league", route.LeagueName)
				stats.BudgetExhausted = true
				break
			}
			return stats, err
		}

		var fixtures []RawFixture
		if results {
			fixtures, err = client.Results(ctx, route.ProviderCompetitionID, from, to)
		} else {
			fixtures, err = client.Fixtures(ctx, route.ProviderCompetitionID, from, to)
		}
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				if throttleErr := s.Budget.Throttle(ctx, route.Provider); throttleErr != nil {
					s.Log.Error("could not record throttle", "err", throttleErr)
				}
				s.Log.Warn("provider rate limited, backing off",
					"provider", route.Provider, "league", route.LeagueName)
				break
			}
			// One league's failure must not abandon the rest of the matchday.
			s.Log.Error("fixture fetch failed",
				"provider", route.Provider, "league", route.LeagueName, "err", err)
			continue
		}
		stats.Fetched += len(fixtures)

		routeStats, err := s.persist(ctx, route, fixtures)
		if err != nil {
			return stats, err
		}
		stats.Created += routeStats.Created
		stats.Updated += routeStats.Updated
		stats.Skipped += routeStats.Skipped
		stats.FuzzyTeamMatches += routeStats.FuzzyTeamMatches
	}

	return stats, nil
}

// persist writes one league's fixtures in a single transaction, so a partial
// matchday is never visible to a reader.
func (s *Syncer) persist(ctx context.Context, route postgres.CompetitionRoute, fixtures []RawFixture) (SyncStats, error) {
	var stats SyncStats

	err := s.DB.InTx(ctx, func(tx pgx.Tx) error {
		for _, f := range fixtures {
			if f.HomeProviderID == "" || f.AwayProviderID == "" {
				// Without provider team ids there is no reliable join key, and
				// matching on names is what creates duplicate clubs.
				s.Log.Warn("fixture has no provider team ids, skipping",
					"provider", route.Provider, "fixture", f.ProviderID)
				stats.Skipped++
				continue
			}

			homeID, homeFuzzy, err := s.DB.TeamIDBySource(ctx, tx,
				route.Provider, f.HomeProviderID, route.LeagueID, f.HomeName, "")
			if err != nil {
				return err
			}
			awayID, awayFuzzy, err := s.DB.TeamIDBySource(ctx, tx,
				route.Provider, f.AwayProviderID, route.LeagueID, f.AwayName, "")
			if err != nil {
				return err
			}
			for _, fuzzy := range []struct {
				matched bool
				name    string
				id      string
			}{{homeFuzzy, f.HomeName, f.HomeProviderID}, {awayFuzzy, f.AwayName, f.AwayProviderID}} {
				if fuzzy.matched {
					// Logged for review: a wrong fuzzy match splits one club's
					// history across two half-strength rows, which quietly
					// degrades every prediction in the league.
					s.Log.Info("team matched by normalised name, review",
						"provider", route.Provider, "league", route.LeagueName,
						"name", fuzzy.name, "provider_team_id", fuzzy.id)
					stats.FuzzyTeamMatches++
				}
			}

			if homeID == awayID {
				s.Log.Warn("fixture has the same team on both sides, skipping",
					"provider", route.Provider, "fixture", f.ProviderID)
				stats.Skipped++
				continue
			}

			_, created, err := s.DB.UpsertMatch(ctx, tx,
				route.Provider, f.ProviderID,
				route.LeagueID, route.SeasonID, homeID, awayID,
				f.KickoffAt.UTC(), f.Status, f.HomeScore, f.AwayScore, f.Round)
			if err != nil {
				return err
			}
			if created {
				stats.Created++
			} else {
				stats.Updated++
			}
		}
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("persist %s fixtures: %w", route.LeagueName, err)
	}
	return stats, nil
}
