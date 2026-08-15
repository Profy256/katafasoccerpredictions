// Command worker runs the River job runner: ingestion, prediction,
// settlement and payment reconciliation.
//
// Separate from the API so that an ingestion crash-loop cannot take the public
// site down with it.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest/apifootball"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest/footballdata"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/jobs"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/logging"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay/marzpay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/predict"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/publish"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

	deps := buildDeps(cfg, db, log)

	workers := river.NewWorkers()
	jobs.Register(workers, deps)

	client, err := river.NewClient(riverpgxv5.New(db.Pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers:      workers,
		PeriodicJobs: jobs.PeriodicJobs(),
		Logger:       log,
	})
	if err != nil {
		return fmt.Errorf("build river client: %w", err)
	}

	log.Info("worker starting", "env", cfg.Env)
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start river: %w", err)
	}

	<-ctx.Done()
	log.Info("worker shutting down")

	// Let in-flight jobs finish. A settlement batch cut in half would replay
	// cleanly, but finishing is cheaper than replaying.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return client.Stop(shutdownCtx)
}

func buildDeps(cfg *config.Config, db *postgres.DB, log *slog.Logger) jobs.Deps {
	providers := map[string]ingest.Provider{}
	if cfg.FootballDataToken != "" {
		providers[ingest.ProviderFootballData] = footballdata.New(cfg.FootballDataToken, db)
	}
	if cfg.APIFootballKey != "" {
		providers[ingest.ProviderAPIFootball] = apifootball.New(cfg.APIFootballKey, db)
	}
	if len(providers) == 0 {
		// Not fatal: settlement, publication and payments all still work, and
		// a development machine has no provider credentials.
		log.Warn("no ingestion providers configured; fixture sync will do nothing")
	}

	var payments pay.Provider = pay.NewFake()
	if cfg.MarzPayConfigured() {
		payments = marzpay.New(cfg.MarzPayBaseURL, cfg.MarzPayAPIUser,
			cfg.MarzPayAPIKey, cfg.MarzPayWebhookSecret)
	} else {
		log.Warn("MarzPay is not configured; using the fake payment provider")
	}

	return jobs.Deps{
		DB:     db,
		Log:    log,
		Config: cfg,
		Syncer: &ingest.Syncer{
			DB:        db,
			Budget:    ingest.NewBudget(db),
			Log:       log,
			Providers: providers,
		},
		Settler: &settle.Service{DB: db, Log: log},
		Predictor: &predict.Service{
			DB:           db,
			Engine:       model.NewPoissonEngine(cfg.ModelVersion),
			Log:          log,
			ModelVersion: cfg.ModelVersion,
		},
		Publisher: &publish.Service{
			DB:           db,
			Log:          log,
			ModelVersion: cfg.ModelVersion,
		},
		Payments: &pay.Service{
			DB:          db,
			Provider:    payments,
			Log:         log,
			CallbackURL: cfg.WebhookCallbackURL(),
			Environment: string(cfg.Env),
		},
	}
}
