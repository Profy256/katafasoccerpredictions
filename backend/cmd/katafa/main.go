// Command katafa is the admin CLI: migrations, seeding, backfills, manual
// settlement and key rotation.
//
// It shares internal/ with the api and worker binaries, so an operation run
// here goes through exactly the same code — and the same database triggers —
// as one run by a job. There is no admin back door that skips the invariants.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/logging"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "katafa: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `katafa — Katafa admin CLI

Usage:
  katafa migrate up|down|status   apply, roll back, or report schema migrations
  katafa jobs migrate             install the River job queue tables
  katafa seed leagues <file.json> load league/team reference data
  katafa backfill --csv <file>    load football-data.co.uk historical results
  katafa settle predictions       run a settlement pass now
  katafa settle slips             grade auto-gradable tips and close slips
  katafa publish free-tips [day]  select and freeze a day's free shortlist
  katafa predict [--hours N]      generate predictions for upcoming fixtures
  katafa prune payloads           drop provider_payloads older than 180 days
`)
}

func run() error {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel, !cfg.Env.IsProduction())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "migrate":
		return migrateCmd(ctx, db, args[1:])
	case "jobs":
		return jobsCmd(ctx, db, args[1:])
	case "seed":
		return seedCmd(ctx, db, log, args[1:])
	case "backfill":
		return backfillCmd(ctx, db, log, args[1:])
	case "settle":
		return settleCmd(ctx, db, log, cfg, args[1:])
	case "publish":
		return publishCmd(ctx, db, log, cfg, args[1:])
	case "predict":
		return predictCmd(ctx, db, log, cfg, args[1:])
	case "prune":
		return pruneCmd(ctx, db, log, args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func migrateCmd(ctx context.Context, db *postgres.DB, args []string) error {
	if len(args) == 0 {
		return errors.New("migrate needs up, down or status")
	}
	switch args[0] {
	case "up":
		if err := postgres.MigrateUp(ctx, db.Pool); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	case "down":
		return postgres.MigrateDown(ctx, db.Pool)
	case "status":
		return postgres.MigrationStatus(ctx, db.Pool)
	default:
		return fmt.Errorf("unknown migrate subcommand %q", args[0])
	}
}
