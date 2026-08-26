package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// leaguesCmd manages the publication gate.
//
// Leagues seed with is_published = false and stay that way until someone
// decides otherwise (INGESTION.md § Cold start). Until this command existed
// that decision was an UPDATE typed against production by hand: no reason
// recorded, no record of who made it, and nothing checking whether the league
// had the history to justify it. The flag governs the entire public feed —
// fixtures, team pages, league pages, the sitemap — so it is the single
// highest-leverage switch in the system and the one least deserving of being
// invisible.
//
// The gate itself is unchanged. This makes flipping it deliberate, checked and
// attributable, which is what "a deliberate act, not a side effect" was always
// supposed to mean.
func leaguesCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("leagues needs list, publish or unpublish")
	}
	switch args[0] {
	case "list":
		return leaguesListCmd(ctx, db, args[1:])
	case "publish":
		return leaguesSetCmd(ctx, db, log, args[1:], true)
	case "unpublish":
		return leaguesSetCmd(ctx, db, log, args[1:], false)
	default:
		return fmt.Errorf("unknown leagues subcommand %q", args[0])
	}
}

// leaguesListCmd prints every league against the gate.
//
// The two columns that matter are READY and SHORT. READY is how many upcoming
// fixtures clear the sample-size floor — i.e. how many could publish tips
// today — and it is counted with the same predicate the publish job uses, so
// a published league showing READY 0 explains an empty shortlist exactly.
func leaguesListCmd(ctx context.Context, db *postgres.DB, args []string) error {
	fs := flag.NewFlagSet("leagues list", flag.ContinueOnError)
	all := fs.Bool("all", false, "include inactive leagues")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}

	rows, err := db.LeagueReadinessAll(ctx, model.MinHistoryPerTeam)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tLEAGUE\tREGION\tPUBLISHED\tUPCOMING\tREADY\tTEAMS SHORT\tMIN PLAYED")
	shown := 0
	for _, r := range rows {
		if !r.IsActive && !*all {
			continue
		}
		shown++
		state := "shadow"
		if r.IsPublished {
			state = "live"
		}
		if !r.IsActive {
			state += " (inactive)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%d/%d\t%d\n",
			r.Slug, r.Name, r.Region, state, r.Upcoming, r.Ready,
			r.TeamsShort, r.Teams, r.MinPlayed)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if shown == 0 {
		fmt.Println("no leagues seeded yet — run katafa seed leagues")
		return nil
	}
	fmt.Printf("\nREADY counts upcoming fixtures where both teams have %d+ finished matches;\n"+
		"only those can publish tips. A live league with READY 0 publishes nothing.\n",
		model.MinHistoryPerTeam)
	return nil
}

func leaguesSetCmd(ctx context.Context, db *postgres.DB, log *slog.Logger, args []string, publish bool) error {
	verb := "publish"
	if !publish {
		verb = "unpublish"
	}

	fs := flag.NewFlagSet("leagues "+verb, flag.ContinueOnError)
	by := fs.String("by", "", "email of the admin making the change (required)")
	reason := fs.String("reason", "", "why (required)")
	force := fs.Bool("force", false,
		"publish a league with no fixtures clearing the sample-size floor")
	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return fmt.Errorf("leagues %s takes one league slug, got %d: %s",
			verb, len(positional), strings.Join(positional, " "))
	}

	slug := ""
	if len(positional) == 1 {
		slug = strings.TrimSpace(positional[0])
	}
	if slug == "" {
		return fmt.Errorf("leagues %s needs a league slug", verb)
	}
	if *by == "" || *reason == "" {
		return errors.New("--by and --reason are both required: this change is written to audit_log")
	}

	// An unknown or non-admin operator fails before anything is touched. The
	// audit row is worth having only if the actor in it is real.
	admin, err := db.UserByEmail(ctx, *by)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("no user with email %q", *by)
		}
		return err
	}
	if admin.Role != "admin" {
		return fmt.Errorf("user %s has role %q, not admin", *by, admin.Role)
	}

	league, err := db.LeagueReadinessBySlug(ctx, slug, model.MinHistoryPerTeam)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("no league with slug %q — try katafa leagues list", slug)
		}
		return err
	}

	if league.IsPublished == publish {
		fmt.Printf("%s is already %s — nothing to do\n", league.Name,
			map[bool]string{true: "published", false: "in shadow mode"}[publish])
		return nil
	}

	// The check the hand-written UPDATE never made. Publishing a league whose
	// teams have eight matches of history is the exact behaviour the accuracy
	// dashboard exists to expose — publicly, and permanently — so it takes an
	// explicit --force rather than a quiet success.
	if publish && league.Ready == 0 {
		msg := fmt.Sprintf(
			"%s has %d upcoming fixtures and %d clearing the %d-match floor "+
				"(%d of %d teams short, fewest played: %d).\n"+
				"Publishing now shows fixtures on the site but produces no tips, "+
				"and any that do publish rest on thin history.",
			league.Name, league.Upcoming, league.Ready, model.MinHistoryPerTeam,
			league.TeamsShort, league.Teams, league.MinPlayed)
		if !*force {
			return fmt.Errorf("%s\nBackfill history first (katafa backfill --csv), or pass --force", msg)
		}
		fmt.Printf("warning: %s\nproceeding because --force was given\n\n", msg)
	}

	err = db.InTx(ctx, func(tx pgx.Tx) error {
		changed, err := db.SetLeaguePublished(ctx, tx, league.ID, publish)
		if err != nil {
			return err
		}
		if !changed {
			// Raced with another operator between the read and the write.
			return fmt.Errorf("%s changed underneath this command — re-run it", league.Name)
		}
		action := "league.published"
		if !publish {
			action = "league.unpublished"
		}
		return db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    action,
			Entity:    "league",
			EntityID:  &league.ID,
			Before:    map[string]any{"is_published": !publish},
			After: map[string]any{
				"is_published":     publish,
				"upcoming":         league.Upcoming,
				"ready":            league.Ready,
				"teams_short":      league.TeamsShort,
				"min_played":       league.MinPlayed,
				"min_history_gate": model.MinHistoryPerTeam,
				"forced":           publish && league.Ready == 0,
			},
			Reason: *reason,
		})
	})
	if err != nil {
		return err
	}

	log.Info("league publication changed",
		"league", league.Slug, "published", publish, "by", admin.Email, "reason", *reason)

	if publish {
		fmt.Printf("%s is live: %d upcoming fixtures, %d eligible for tips.\n",
			league.Name, league.Upcoming, league.Ready)
		fmt.Println("Run katafa publish free-tips to select the next shortlist over it.")
	} else {
		fmt.Printf("%s is back in shadow mode. Predictions keep generating and settling;\n"+
			"nothing from it is shown. Already-published free tips are untouched — they are immutable.\n",
			league.Name)
	}
	return nil
}

// parseFlags parses args in any order, returning the positional arguments.
//
// flag.FlagSet stops at the first non-flag token, so a bare fs.Parse(args)
// silently drops every flag typed *after* the league slug — and
// `katafa leagues publish eng-premier-league -by=… -reason=…` is exactly how
// anyone would type it. Dropping -by and -reason there would have turned the
// required-flag check into a coin flip based on argument order. Parsing
// resumes after each positional instead.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}
