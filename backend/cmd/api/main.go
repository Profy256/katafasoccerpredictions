// Command api is the HTTP server. Stateless; scale horizontally.
//
// It is a separate process from the worker because their failure modes are
// unrelated: a provider outage stalls the worker, while the API keeps serving
// yesterday's published tips, which are already in Postgres and do not need
// the provider to be reachable.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/logging"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay/marzpay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
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

	server := &api.Server{
		DB:       db,
		Log:      log,
		Config:   cfg,
		Payments: paymentProvider(cfg, log),
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: server.Handler(),
		// Generous enough for a slow mobile connection, short enough that a
		// stuck client cannot hold a connection open indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "port", cfg.Port, "env", cfg.Env)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// Drain in-flight requests before exiting, so a deploy does not cut a
	// purchase off mid-flight.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

// paymentProvider returns the live client when credentials are configured and
// the fake otherwise. Development and tests therefore cannot reach the live
// API even by accident.
func paymentProvider(cfg *config.Config, log *slog.Logger) pay.Provider {
	if cfg.MarzPayConfigured() {
		return marzpay.New(cfg.MarzPayBaseURL, cfg.MarzPayAPIUser, cfg.MarzPayAPIKey, cfg.MarzPayWebhookSecret)
	}
	log.Warn("MarzPay is not configured; using the fake payment provider")
	return pay.NewFake()
}
