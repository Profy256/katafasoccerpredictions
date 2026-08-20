package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Admin money endpoints. These answer two questions that otherwise require
// somebody to log into MarzPay and guess:
//
//	"where did this month's money come from?"       → GET /admin/revenue
//	"what is this line on the statement?"           → GET /admin/payments/{traceCode}
//
// Both are admin-only and no-store. A revenue figure is not something to leave
// in a shared cache.

func (s *Server) handleAdminRevenue(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseWindow(r, 30)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	report, err := s.DB.Revenue(r.Context(), from, to)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, report)
}

// handleAdminPaymentTrace resolves a trace code read off a MarzPay statement,
// or quoted by a user from their SMS, to the slip and the buyer behind it.
func (s *Server) handleAdminPaymentTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := s.DB.PaymentByTraceCode(r.Context(), r.PathValue("traceCode"))
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, trace)
}

// handleAdminPaymentLedger lists every collection attempt in a window, the way
// a statement is reconciled: successes, failures and pending alike.
func (s *Server) handleAdminPaymentLedger(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseWindow(r, 7)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			render.Error(w, r, fmt.Errorf("%w: limit must be a positive integer", domain.ErrValidation), s.Log)
			return
		}
		limit = parsed
	}

	ledger, err := s.DB.PaymentLedger(r.Context(), from, to, limit)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "rows": ledger,
	})
}

// parseWindow reads from/to as YYYY-MM-DD, defaulting to the last defaultDays.
//
// `to` is exclusive and rounded up to the end of its day, so "from=1st&to=31st"
// includes the 31st — which is what anyone asking for a month expects.
func parseWindow(r *http.Request, defaultDays int) (time.Time, time.Time, error) {
	query := r.URL.Query()

	to := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -defaultDays)

	if raw := query.Get("from"); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf(
				"%w: from must be YYYY-MM-DD", domain.ErrValidation)
		}
		from = parsed.UTC()
	}
	if raw := query.Get("to"); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf(
				"%w: to must be YYYY-MM-DD", domain.ErrValidation)
		}
		to = parsed.UTC().AddDate(0, 0, 1)
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"%w: to is not after from", domain.ErrValidation)
	}
	return from, to, nil
}
