// Package api is the HTTP surface, mapped one-to-one onto the functions in
// src/api/client.ts.
//
// Handlers hold no business logic: grading lives in settle, selection in tips,
// entitlement in the SQL that postgres issues. A handler translates HTTP to a
// call and back, which is what keeps the rules unit-testable without spinning
// up a server.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Server carries the handler dependencies. A plain struct rather than a DI
// container: the dependency graph is small and explicit.
type Server struct {
	DB      *postgres.DB
	Log     *slog.Logger
	Config  *config.Config
	Payments pay.Provider
	// Enqueue hands work to River. Nil in tests that do not exercise the
	// asynchronous paths.
	Enqueue Enqueuer
}

// Enqueuer is the subset of the job client the API needs. Keeping it an
// interface means the HTTP tests do not need a running worker.
type Enqueuer interface {
	EnqueueWebhook(ctx context.Context, eventID int64) error
}

func (s *Server) cookieOptions() auth.CookieOptions {
	return auth.CookieOptions{
		Domain: s.Config.SessionCookieDomain,
		Secure: s.Config.Env.IsProduction() || s.Config.Env == config.EnvStaging,
	}
}

// Handler builds the routed, wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Outside /v1: liveness and readiness are infrastructure concerns and must
	// not move with the API version.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Public reads.
	mux.HandleFunc("GET /v1/leagues", s.handleLeagues)
	mux.HandleFunc("GET /v1/markets", s.handleMarkets)
	mux.HandleFunc("GET /v1/feed", s.handleFeed)
	mux.HandleFunc("GET /v1/matches/{id}", s.handleMatchDetail)
	mux.HandleFunc("GET /v1/accuracy", s.handleAccuracy)
	mux.HandleFunc("GET /v1/predictions/settled", s.handleSettledPredictions)
	mux.HandleFunc("GET /v1/tips/free", s.handleFreeTips)
	mux.HandleFunc("GET /v1/tips/free/history", s.handleFreeTipsHistory)
	mux.HandleFunc("GET /v1/packages", s.handlePackages)
	mux.HandleFunc("GET /v1/analysts", s.handleAnalysts)
	mux.HandleFunc("GET /v1/analysts/leaderboard", s.handleAnalystLeaderboard)
	mux.HandleFunc("GET /v1/analysts/{slug}", s.handleAnalystRecord)
	mux.HandleFunc("GET /v1/stats/coverage", s.handleCoverage)

	// Slips: auth is optional, and decides whether tips come back at all.
	mux.HandleFunc("GET /v1/slips", s.handleSlips)
	mux.HandleFunc("GET /v1/slips/{id}", s.handleSlip)

	// Accounts.
	mux.HandleFunc("POST /v1/auth/register", s.handleRegister)
	mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /v1/auth/me", s.requireUser(s.handleMe))

	// Purchases.
	mux.HandleFunc("POST /v1/slips/{id}/purchase", s.requireUser(s.handlePurchase))
	mux.HandleFunc("GET /v1/purchases", s.requireUser(s.handlePurchases))
	mux.HandleFunc("GET /v1/purchases/{id}", s.requireUser(s.handlePurchase_Get))

	// Public, signature-verified. No session.
	mux.HandleFunc("POST /v1/webhooks/marzpay", s.handleMarzPayWebhook)

	// Admin. Every route writes audit_log.
	mux.HandleFunc("POST /v1/admin/slips", s.requireAdmin(s.handleAdminCreateSlip))
	mux.HandleFunc("POST /v1/admin/slips/{id}/tips", s.requireAdmin(s.handleAdminAddTip))
	mux.HandleFunc("POST /v1/admin/slips/{id}/publish", s.requireAdmin(s.handleAdminPublishSlip))
	mux.HandleFunc("DELETE /v1/admin/slips/{id}", s.requireAdmin(s.handleAdminDeleteSlip))
	mux.HandleFunc("POST /v1/admin/tips/{id}/settle", s.requireAdmin(s.handleAdminSettleTip))
	mux.HandleFunc("POST /v1/admin/matches/{id}/correct", s.requireAdmin(s.handleAdminCorrectMatch))
	mux.HandleFunc("GET /v1/admin/ingest/status", s.requireAdmin(s.handleAdminIngestStatus))

	// Money: where it came from, and what a given statement line was for.
	mux.HandleFunc("GET /v1/admin/revenue", s.requireAdmin(s.handleAdminRevenue))
	mux.HandleFunc("GET /v1/admin/payments", s.requireAdmin(s.handleAdminPaymentLedger))
	mux.HandleFunc("GET /v1/admin/payments/{traceCode}", s.requireAdmin(s.handleAdminPaymentTrace))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		render.Problemf(w, r, http.StatusNotFound, "not-found",
			"Not found", "No route for %s %s", r.Method, r.URL.Path)
	})

	return chain(mux,
		s.requestID,
		s.logRequests,
		s.recoverPanics,
		s.cors,
		s.session,
		s.rateLimit,
	)
}

// handleHealthz reports that the process is up. It deliberately does not touch
// Postgres: a liveness probe that fails on a database blip restarts a healthy
// process and makes an outage worse.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	render.Status(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz reports that this instance can serve: Postgres is reachable and
// the schema is current. An API serving against a half-migrated schema
// produces errors that look like application bugs.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.Ping(r.Context()); err != nil {
		render.Status(w, http.StatusServiceUnavailable,
			map[string]string{"status": "unavailable", "reason": "database unreachable"})
		return
	}

	current, err := postgres.MigrationsCurrent(r.Context(), s.DB.Pool)
	if err != nil {
		render.Status(w, http.StatusServiceUnavailable,
			map[string]string{"status": "unavailable", "reason": "migration state unknown"})
		return
	}
	if !current {
		render.Status(w, http.StatusServiceUnavailable,
			map[string]string{"status": "unavailable", "reason": "migrations pending"})
		return
	}
	render.Status(w, http.StatusOK, map[string]string{"status": "ready"})
}
