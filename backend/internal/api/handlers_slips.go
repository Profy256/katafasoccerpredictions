package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// handleSlips lists published slips. Auth is optional.
//
// The response is no-store despite carrying only metadata, because whether the
// viewer owns each slip varies per request and a shared cache would serve one
// viewer's view to another.
func (s *Server) handleSlips(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := postgres.SlipQuery{}

	if raw := query.Get("package"); raw != "" {
		code, err := domain.ParsePackageCode(raw)
		if err != nil {
			render.Error(w, r, fmt.Errorf("%w: %s", domain.ErrValidation, err), s.Log)
			return
		}
		q.PackageCode = code
	}
	switch status := query.Get("status"); status {
	case "":
	case "open", "settled":
		q.Status = status
	default:
		render.Error(w, r, fmt.Errorf("%w: status must be open or settled", domain.ErrValidation), s.Log)
		return
	}
	if raw := query.Get("analyst"); raw != "" {
		id, err := parseUUID(raw, "analyst id")
		if err != nil {
			render.Error(w, r, err, s.Log)
			return
		}
		q.AnalystID = &id
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			render.Error(w, r, fmt.Errorf("%w: limit must be a positive integer", domain.ErrValidation), s.Log)
			return
		}
		q.Limit = limit
	}
	if raw := query.Get("cursor"); raw != "" {
		cursor, err := decodeSlipCursor(raw)
		if err != nil {
			render.Error(w, r, err, s.Log)
			return
		}
		q.Cursor = cursor
	}

	slips, next, err := s.DB.Slips(r.Context(), q)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	render.JSON(w, r, struct {
		Items      []domain.APISlip `json:"items"`
		NextCursor string           `json:"nextCursor,omitempty"`
	}{Items: orEmpty(slips), NextCursor: encodeSlipCursor(next)}, render.CacheNone)
}

// handleSlip returns one slip. Whether its tips come back is decided in SQL by
// the entitlement clause, not here.
func (s *Server) handleSlip(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"), "slip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	viewerID := uuid.Nil
	if user, ok := auth.UserFrom(r.Context()); ok {
		viewerID = user.ID
	}

	slip, err := s.DB.Slip(r.Context(), id, viewerID, auth.IsAdmin(r.Context()))
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.JSON(w, r, slip, render.CachePrivate)
}

type purchaseRequest struct {
	PhoneNumber string `json:"phone_number"`
}

func (s *Server) handlePurchase(w http.ResponseWriter, r *http.Request) {
	slipID, err := parseUUID(r.PathValue("id"), "slip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	var req purchaseRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	user, _ := auth.UserFrom(r.Context())
	service := s.paymentService()
	result, err := service.Purchase(r.Context(), user.ID, slipID, req.PhoneNumber)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	// 202: the collection is in flight. The user's phone gets a mobile money
	// prompt and the frontend polls the purchase or waits for the webhook.
	render.Status(w, http.StatusAccepted, result)
}

func (s *Server) handlePurchase_Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"), "purchase id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	user, _ := auth.UserFrom(r.Context())

	purchase, err := s.DB.Purchase(r.Context(), id, user.ID)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, purchase)
}

func (s *Server) handlePurchases(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	purchases, err := s.DB.PurchasesForUser(r.Context(), user.ID)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, orEmpty(purchases))
}

// handleMarzPayWebhook records every callback before doing anything with it.
func (s *Server) handleMarzPayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		render.Error(w, r, fmt.Errorf("%w: unreadable body", domain.ErrValidation), s.Log)
		return
	}

	signature := firstHeader(r, "X-Marzpay-Signature", "X-Signature", "Signature")
	valid := s.Payments.VerifyWebhook(body, signature)

	eventType, providerTxnID, reference := pay.ParseWebhook(body)

	// Written before any processing, whatever the contents. A malformed,
	// unsigned, or unrecognised callback is still recorded: when a user says
	// they were charged and got nothing, this table is the first place to look
	// and it must not have gaps.
	eventID, err := s.DB.RecordWebhookEvent(r.Context(), "marzpay",
		eventType, providerTxnID, reference, body, valid)
	if err != nil {
		s.Log.Error("could not record webhook", "err", err)
		// A 500 makes the provider retry, which is what we want: the callback
		// is not yet recorded anywhere.
		render.Error(w, r, err, s.Log)
		return
	}

	if !valid {
		// 200, not 401. Never let an attacker distinguish a rejected forgery
		// from an accepted one, and never make the provider retry a forgery.
		s.Log.Warn("webhook signature invalid", "event_id", eventID)
		render.Status(w, http.StatusOK, map[string]string{"status": "received"})
		return
	}

	// Processed asynchronously: the provider's retry policy is not ours to
	// depend on, and a slow entitlement write must not cause a timeout that
	// triggers duplicate callbacks.
	if s.Enqueue != nil {
		if err := s.Enqueue.EnqueueWebhook(r.Context(), eventID); err != nil {
			s.Log.Error("could not enqueue webhook processing", "event_id", eventID, "err", err)
		}
	} else {
		// No worker wired (tests, or a single-process deployment): process
		// inline rather than dropping the event.
		if err := s.paymentService().ProcessWebhookEvent(r.Context(), eventID); err != nil {
			s.Log.Error("inline webhook processing failed", "event_id", eventID, "err", err)
		}
	}

	render.Status(w, http.StatusOK, map[string]string{"status": "received"})
}

func (s *Server) paymentService() *pay.Service {
	return &pay.Service{
		DB:          s.DB,
		Provider:    s.Payments,
		Log:         s.Log,
		CallbackURL: s.Config.WebhookCallbackURL(),
		Environment: string(s.Config.Env),
	}
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if v := r.Header.Get(name); v != "" {
			return v
		}
	}
	return ""
}
