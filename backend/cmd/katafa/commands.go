package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/predict"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/publish"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

// jobsCmd installs River's own tables. Kept separate from the schema
// migrations so the queue can be upgraded independently of the domain schema.
func jobsCmd(ctx context.Context, db *postgres.DB, args []string) error {
	if len(args) == 0 || args[0] != "migrate" {
		return errors.New("jobs needs the migrate subcommand")
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(db.Pool), nil)
	if err != nil {
		return fmt.Errorf("build river migrator: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("migrate river: %w", err)
	}
	for _, v := range res.Versions {
		fmt.Printf("river migration %d applied\n", v.Version)
	}
	return nil
}

// seedCmd loads league and team reference data.
//
// Leagues start with is_published = false: a league gets published tips only
// once its teams have enough history for the strength estimates to mean
// anything. Flipping that flag is a deliberate act, not a side effect of
// seeding.
func seedCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	slug := fs.String("slug", "", "league slug")
	name := fs.String("name", "", "league name")
	shortName := fs.String("short-name", "", "league short name")
	country := fs.String("country", "", "country")
	countryCode := fs.String("country-code", "", "country code")
	region := fs.String("region", "", "europe|east-africa|africa|americas|asia")
	tier := fs.Int("tier", 1, "league tier")
	season := fs.Int("season", time.Now().UTC().Year(), "season start year")
	provider := fs.String("provider", "", "football-data|api-football")
	providerID := fs.String("provider-id", "", "the provider's competition id")

	if len(args) > 0 && args[0] == "leagues" {
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" || *name == "" || *region == "" {
		return errors.New("seed leagues needs at least -slug, -name and -region")
	}
	if _, err := domain.ParseRegion(*region); err != nil {
		return err
	}
	if *shortName == "" {
		*shortName = *name
	}

	return db.InTx(ctx, func(tx pgx.Tx) error {
		var leagueID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO leagues (slug, name, short_name, country, country_code, tier, region)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`,
			*slug, *name, *shortName, *country, *countryCode, *tier, *region).Scan(&leagueID); err != nil {
			return fmt.Errorf("insert league: %w", err)
		}

		label := fmt.Sprintf("%d/%02d", *season, (*season+1)%100)
		if _, err := tx.Exec(ctx, `
			INSERT INTO seasons (league_id, label, start_year, is_current)
			VALUES ($1,$2,$3,TRUE)
			ON CONFLICT (league_id, start_year) DO NOTHING`,
			leagueID, label, *season); err != nil {
			return fmt.Errorf("insert season: %w", err)
		}

		if *provider != "" && *providerID != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO competition_sources (league_id, provider, provider_competition_id, is_primary)
				VALUES ($1,$2,$3,TRUE)
				ON CONFLICT (league_id, provider)
				DO UPDATE SET provider_competition_id = EXCLUDED.provider_competition_id`,
				leagueID, *provider, *providerID); err != nil {
				return fmt.Errorf("link competition source: %w", err)
			}
		}

		log.Info("league seeded", "slug", *slug, "id", leagueID, "published", false)
		return nil
	})
}

// backfillCmd loads football-data.co.uk historical results.
//
// This is how the cold start is survived: free API tiers sell fixtures going
// forward, not history going back, so the deep European history is seeded once
// from CSV and everything after that is accumulated.
//
// Expected columns: Date, HomeTeam, AwayTeam, FTHG, FTAG (the standard
// football-data.co.uk layout). FTHG/FTAG are full-time 90-minute goals.
func backfillCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	path := fs.String("csv", "", "path to a football-data.co.uk CSV")
	leagueSlug := fs.String("league", "", "league slug the CSV belongs to")
	season := fs.Int("season", 0, "season start year")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" || *leagueSlug == "" {
		return errors.New("backfill needs -csv and -league")
	}

	file, err := os.Open(*path)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read csv header: %w", err)
	}
	index := map[string]int{}
	for i, column := range header {
		index[strings.TrimSpace(column)] = i
	}
	for _, required := range []string{"Date", "HomeTeam", "AwayTeam", "FTHG", "FTAG"} {
		if _, ok := index[required]; !ok {
			return fmt.Errorf("csv is missing the %s column", required)
		}
	}

	var leagueID, seasonID string
	if err := db.Pool.QueryRow(ctx,
		`SELECT id FROM leagues WHERE slug = $1`, *leagueSlug).Scan(&leagueID); err != nil {
		return fmt.Errorf("unknown league %q: %w", *leagueSlug, err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT id FROM seasons
		WHERE league_id = $1 AND ($2 = 0 OR start_year = $2)
		ORDER BY is_current DESC, start_year DESC LIMIT 1`,
		leagueID, *season).Scan(&seasonID); err != nil {
		return fmt.Errorf("no season for league %q: %w", *leagueSlug, err)
	}

	imported, skipped := 0, 0
	err = db.InTx(ctx, func(tx pgx.Tx) error {
		for {
			record, err := reader.Read()
			if err != nil {
				break
			}
			if len(record) <= index["FTAG"] {
				skipped++
				continue
			}

			kickoff, err := parseCSVDate(record[index["Date"]])
			if err != nil {
				skipped++
				continue
			}
			homeGoals, err1 := strconv.Atoi(strings.TrimSpace(record[index["FTHG"]]))
			awayGoals, err2 := strconv.Atoi(strings.TrimSpace(record[index["FTAG"]]))
			if err1 != nil || err2 != nil {
				skipped++
				continue
			}

			homeName := strings.TrimSpace(record[index["HomeTeam"]])
			awayName := strings.TrimSpace(record[index["AwayTeam"]])
			if homeName == "" || awayName == "" {
				skipped++
				continue
			}

			// The CSV has no provider ids, so a synthetic source id keyed on
			// the fixture makes the load idempotent: re-running it updates
			// nothing rather than duplicating a season.
			homeID, _, err := db.TeamIDBySource(ctx, tx, "football-data",
				"csv:"+*leagueSlug+":"+postgres.NormaliseTeamName(homeName), uuidFrom(leagueID), homeName, "")
			if err != nil {
				return err
			}
			awayID, _, err := db.TeamIDBySource(ctx, tx, "football-data",
				"csv:"+*leagueSlug+":"+postgres.NormaliseTeamName(awayName), uuidFrom(leagueID), awayName, "")
			if err != nil {
				return err
			}
			if homeID == awayID {
				skipped++
				continue
			}

			providerMatchID := fmt.Sprintf("csv:%s:%s:%s:%s",
				*leagueSlug, kickoff.Format("2006-01-02"),
				postgres.NormaliseTeamName(homeName), postgres.NormaliseTeamName(awayName))

			if _, _, err := db.UpsertMatch(ctx, tx, "football-data", providerMatchID,
				uuidFrom(leagueID), uuidFrom(seasonID), homeID, awayID,
				kickoff, domain.StatusFinished, &homeGoals, &awayGoals, nil); err != nil {
				return err
			}
			imported++
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Info("backfill complete", "league", *leagueSlug, "imported", imported, "skipped", skipped)
	return nil
}

// parseCSVDate handles both the two- and four-digit year forms
// football-data.co.uk has used over the years.
func parseCSVDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"02/01/2006", "02/01/06", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			// No kickoff time in the CSV. 15:00 UTC is a reasonable stand-in;
			// only the ordering matters, and these are historical results that
			// no prediction is graded against.
			return time.Date(t.Year(), t.Month(), t.Day(), 15, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable date %q", raw)
}

func settleCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, cfg *config.Config, args []string) error {
	service := &settle.Service{DB: db, Log: log}
	if len(args) == 0 {
		return errors.New("settle needs predictions or slips")
	}

	switch args[0] {
	case "predictions":
		graded, err := service.SettlePredictions(ctx)
		if err != nil {
			return err
		}
		voided, err := service.VoidUngradablePredictions(ctx)
		if err != nil {
			return err
		}
		if err := db.RefreshRollups(ctx); err != nil {
			return err
		}
		fmt.Printf("graded %d, voided %d\n", graded, voided)
		return nil

	case "slips":
		graded, err := service.SettleTips(ctx)
		if err != nil {
			return err
		}
		closed, voided, err := service.CloseSlips(ctx)
		if err != nil {
			return err
		}
		if err := db.RefreshRollups(ctx); err != nil {
			return err
		}
		fmt.Printf("tips graded %d, slips settled %d, slips voided %d\n", graded, closed, len(voided))
		return nil

	default:
		return fmt.Errorf("unknown settle target %q", args[0])
	}
}

func publishCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, cfg *config.Config, args []string) error {
	if len(args) == 0 || args[0] != "free-tips" {
		return errors.New("publish needs the free-tips subcommand")
	}
	service := &publish.Service{DB: db, Log: log, ModelVersion: cfg.ModelVersion}

	day, err := service.PublishNextDay(ctx)
	if err != nil {
		return err
	}
	if day.IsEmpty() {
		fmt.Println("nothing eligible to publish")
		return nil
	}
	fmt.Printf("published %d tips for %s\n", day.TotalTips, day.Day.Format(time.DateOnly))
	return nil
}

func predictCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("predict", flag.ContinueOnError)
	hours := fs.Int("hours", int(predict.Horizon/time.Hour), "how far ahead to predict")
	if err := fs.Parse(args); err != nil {
		return err
	}

	service := &predict.Service{
		DB:           db,
		Engine:       model.NewPoissonEngine(cfg.ModelVersion),
		Log:          log,
		ModelVersion: cfg.ModelVersion,
	}
	stats, err := service.GenerateUpcoming(ctx, time.Duration(*hours)*time.Hour)
	if err != nil {
		return err
	}
	fmt.Printf("fixtures %d, predictions %d, skipped %d\n", stats.Fixtures, stats.Predictions, stats.Skipped)
	return nil
}

func pruneCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, args []string) error {
	if len(args) == 0 || args[0] != "payloads" {
		return errors.New("prune needs the payloads subcommand")
	}
	deleted, err := db.PrunePayloads(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("pruned %d provider payloads\n", deleted)
	return nil
}

// riverClientUnused keeps the river import honest if jobsCmd changes shape.
var _ = river.QueueDefault

// uuidFrom parses an id read back from Postgres, where it is already known to
// be well-formed.
func uuidFrom(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic("katafa: malformed uuid from database: " + s)
	}
	return id
}
