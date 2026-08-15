package pay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Service runs the purchase flow, webhook processing and reconciliation.
type Service struct {
	DB          *postgres.DB
	Provider    Provider
	Log         *slog.Logger
	CallbackURL string
	// Environment is stamped into payment metadata so a sandbox transaction is
	// never mistaken for a live one when reconciling an account statement.
	Environment string
}

// PurchaseResult is what the API returns immediately. The user's phone then
// gets a mobile money prompt; the frontend polls the purchase or waits for the
// webhook.
type PurchaseResult struct {
	PurchaseID uuid.UUID `json:"purchaseId"`
	Status     string    `json:"status"`
	// TraceCode is the short code that appears in the payer's SMS and on the
	// MarzPay statement. Returned so support can be given it by a user who
	// says they paid, and match it without asking for a phone number.
	TraceCode string `json:"traceCode"`
}

// Purchase starts a collection for one slip.
func (s *Service) Purchase(ctx context.Context, userID, slipID uuid.UUID, phone string) (PurchaseResult, error) {
	slip, err := s.DB.Slip(ctx, slipID, userID, false)
	if err != nil {
		return PurchaseResult{}, err
	}
	if slip.Status != string(domain.SlipOpen) {
		return PurchaseResult{}, fmt.Errorf(
			"%w: slip %s is not open for purchase", domain.ErrConflict, slipID)
	}

	owns, err := s.DB.AlreadyOwns(ctx, s.DB.Pool, userID, slipID)
	if err != nil {
		return PurchaseResult{}, err
	}
	if owns {
		return PurchaseResult{}, fmt.Errorf("%w: slip %s is already purchased", domain.ErrConflict, slipID)
	}

	normalised, err := NormalisePhone(phone)
	if err != nil {
		return PurchaseResult{}, err
	}
	if err := ValidateCollection(slip.PriceUGX, normalised); err != nil {
		return PurchaseResult{}, err
	}

	rawRequest, err := json.Marshal(map[string]any{
		"slip_id": slipID, "user_id": userID,
		"amount": int64(slip.PriceUGX), "phone_number": normalised,
		"slip_title": slip.Title, "package": slip.PackageCode,
	})
	if err != nil {
		return PurchaseResult{}, fmt.Errorf("encode request record: %w", err)
	}

	// Committed before the outbound call, deliberately. A crash between the
	// two leaves an 'initiated' row that reconciliation will pick up; the
	// reverse order would leave a charge with no local record.
	var intent postgres.PurchaseIntent
	err = s.DB.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		intent, err = s.DB.CreatePurchaseIntent(ctx, tx, userID, slipID, slip.PriceUGX, normalised, rawRequest)
		return err
	})
	if err != nil {
		return PurchaseResult{}, err
	}

	// The trace code leads the description so that a line on a MarzPay
	// statement can be tied back to a slip and a buyer without opening a
	// support ticket.
	traceCode := TraceCode(intent.Reference)

	resp, err := s.Provider.Collect(ctx, CollectRequest{
		Reference:   intent.Reference,
		AmountUGX:   slip.PriceUGX,
		PhoneNumber: normalised,
		Description: CollectionDescription(intent.Reference, slip.PackageCode, slip.Title),
		CallbackURL: s.CallbackURL,
		Metadata:    CollectionMetadata(intent.PurchaseID, slipID, userID, slip.PackageCode, s.Environment),
	})
	if err != nil {
		// The transaction stays 'initiated' and reconciliation will resolve
		// it. Reporting failure here would be a guess: the gateway may well
		// have taken the request before the connection broke.
		s.Log.Error("collection request failed",
			"purchase_id", intent.PurchaseID, "trace_code", traceCode,
			"reference", intent.Reference, "err", err)
		if len(resp.Raw) > 0 {
			_ = s.DB.RecordCollectResponse(ctx, intent.TransactionID, "", "", "", string(TxInitiated), resp.Raw)
		}
		return PurchaseResult{
			PurchaseID: intent.PurchaseID, Status: string(TxProcessing), TraceCode: traceCode,
		}, nil
	}

	if err := s.DB.RecordCollectResponse(ctx, intent.TransactionID,
		resp.ProviderUUID, resp.ProviderTxnID, resp.MobileProvider, string(resp.Status), resp.Raw); err != nil {
		s.Log.Error("could not record collect response", "purchase_id", intent.PurchaseID, "err", err)
	}

	// Logged at info so the money trail is reconstructable from logs alone if
	// the database is ever the thing in question.
	s.Log.Info("collection requested",
		"purchase_id", intent.PurchaseID, "trace_code", traceCode,
		"provider_uuid", resp.ProviderUUID, "provider_txn_id", resp.ProviderTxnID,
		"amount_ugx", int64(slip.PriceUGX), "package", slip.PackageCode, "slip_id", slipID)

	return PurchaseResult{
		PurchaseID: intent.PurchaseID, Status: string(TxProcessing), TraceCode: traceCode,
	}, nil
}

// WebhookPayload is the subset of a callback this system reads.
type WebhookPayload struct {
	EventType  string `json:"event_type"`
	Event      string `json:"event"`
	Reference  string `json:"reference"`
	Collection struct {
		ProviderTransactionID string `json:"provider_transaction_id"`
		Reference             string `json:"reference"`
		Status                string `json:"status"`
	} `json:"collection"`
	Data struct {
		ProviderTransactionID string `json:"provider_transaction_id"`
		Reference             string `json:"reference"`
		Status                string `json:"status"`
	} `json:"data"`
}

// ParseWebhook extracts the identifiers a callback carries, tolerating the
// shapes the gateway uses across event types. It never fails: an unparseable
// body is still recorded, because the record is what a charge dispute is
// resolved from.
func ParseWebhook(body []byte) (eventType, providerTxnID, reference string) {
	var p WebhookPayload
	_ = json.Unmarshal(body, &p)

	eventType = firstNonEmpty(p.EventType, p.Event)
	providerTxnID = firstNonEmpty(p.Collection.ProviderTransactionID, p.Data.ProviderTransactionID)
	reference = firstNonEmpty(p.Reference, p.Collection.Reference, p.Data.Reference)
	return eventType, providerTxnID, reference
}

// ProcessWebhookEvent applies a recorded callback. Idempotent: a duplicate
// delivery must not create a second entitlement.
func (s *Service) ProcessWebhookEvent(ctx context.Context, eventID int64) error {
	event, err := s.DB.WebhookEventByID(ctx, s.DB.Pool, eventID)
	if err != nil {
		return err
	}
	if !event.SignatureValid {
		// Recorded, never acted on.
		return s.DB.MarkWebhookProcessed(ctx, s.DB.Pool, eventID, "signature invalid")
	}

	_, providerTxnID, reference := ParseWebhook(event.Payload)
	txn, err := s.DB.FindTransaction(ctx, s.DB.Pool, providerTxnID, reference)
	if errors.Is(err, domain.ErrNotFound) {
		// A callback about a transaction we do not recognise is left
		// unprocessed and alerted on rather than discarded: it may be the only
		// evidence that a user was charged.
		s.Log.Error("webhook for unknown transaction",
			"event_id", eventID, "provider_txn_id", providerTxnID, "reference", reference)
		return s.DB.MarkWebhookProcessed(ctx, s.DB.Pool, eventID, "unknown transaction")
	}
	if err != nil {
		return err
	}

	status := webhookStatus(event.Payload, event.EventType)
	if err := s.applyStatus(ctx, txn, status); err != nil {
		_ = s.DB.MarkWebhookProcessed(ctx, s.DB.Pool, eventID, err.Error())
		return err
	}
	return s.DB.MarkWebhookProcessed(ctx, s.DB.Pool, eventID, "")
}

// applyStatus is the single place a transaction's outcome is acted on, shared
// by the webhook and by reconciliation so the two cannot diverge.
func (s *Service) applyStatus(ctx context.Context, txn postgres.PaymentTransaction, status TxStatus) error {
	switch status {
	case TxCompleted:
		return s.DB.InTx(ctx, func(tx pgx.Tx) error {
			granted, err := s.DB.CompletePurchase(ctx, tx, txn.ID, txn.PurchaseID)
			if err != nil {
				return err
			}
			if !granted {
				// Zero rows updated: already processed. Not an error.
				return nil
			}
			return s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
				ActorType: postgres.ActorSystem,
				Action:    "purchase.paid",
				Entity:    "purchase",
				EntityID:  &txn.PurchaseID,
				After:     map[string]any{"transaction_id": txn.ID, "amount_ugx": int64(txn.AmountUGX)},
			})
		})

	case TxFailed, TxExpired:
		return s.DB.InTx(ctx, func(tx pgx.Tx) error {
			return s.DB.FailPurchase(ctx, tx, txn.ID, txn.PurchaseID, string(status))
		})

	default:
		// Still in flight. Reconciliation will look again.
		return nil
	}
}

// Reconcile polls non-final transactions and applies whatever the gateway
// says. Webhooks get lost; this is what makes that survivable.
func (s *Service) Reconcile(ctx context.Context, limit int) (int, error) {
	pending, err := s.DB.UnreconciledTransactions(ctx, limit)
	if err != nil {
		return 0, err
	}

	resolved := 0
	for _, txn := range pending {
		if txn.ProviderUUID == nil || *txn.ProviderUUID == "" {
			// The outbound call never landed, so there is nothing to poll.
			// ExpireStaleTransactions closes these out after 24 hours.
			continue
		}
		resp, err := s.Provider.Status(ctx, *txn.ProviderUUID)
		if err != nil {
			s.Log.Warn("reconcile poll failed", "transaction_id", txn.ID, "err", err)
			continue
		}
		if err := s.applyStatus(ctx, txn, resp.Status); err != nil {
			s.Log.Error("reconcile apply failed", "transaction_id", txn.ID, "err", err)
			continue
		}
		if resp.Status.IsFinal() {
			resolved++
		}
	}

	if _, err := s.DB.ExpireStaleTransactions(ctx); err != nil {
		return resolved, err
	}
	return resolved, nil
}

// RefundSlip refunds every paid purchase of a slip. Used when every leg voided
// and the slip returned nothing.
func (s *Service) RefundSlip(ctx context.Context, slipID uuid.UUID, reason string) error {
	purchases, err := s.DB.PaidPurchasesForSlip(ctx, s.DB.Pool, slipID)
	if err != nil {
		return err
	}

	for _, p := range purchases {
		if err := s.DB.InTx(ctx, func(tx pgx.Tx) error {
			if err := s.DB.RefundPurchase(ctx, tx, p.ID, p.PriceUGX, reason); err != nil {
				return err
			}
			return s.DB.WriteAudit(ctx, tx, postgres.AuditEntry{
				ActorType: postgres.ActorSystem,
				Action:    "purchase.refunded",
				Entity:    "purchase",
				EntityID:  &p.ID,
				Reason:    reason,
			})
		}); err != nil {
			return err
		}
		s.Log.Info("purchase refunded", "purchase_id", p.ID, "slip_id", slipID, "reason", reason)
	}
	return nil
}

// webhookStatus decides the outcome a callback reports. The callback fires
// only on a final status, so anything not recognisably a success is treated as
// a failure — except that applyStatus refuses to downgrade a paid purchase.
func webhookStatus(body []byte, eventType string) TxStatus {
	var p WebhookPayload
	_ = json.Unmarshal(body, &p)

	raw := firstNonEmpty(p.Collection.Status, p.Data.Status)
	switch raw {
	case "successful", "success", "completed":
		return TxCompleted
	case "failed", "rejected", "cancelled":
		return TxFailed
	case "expired", "timeout":
		return TxExpired
	}

	switch firstNonEmpty(eventType, p.EventType, p.Event) {
	case "collection.completed":
		return TxCompleted
	case "collection.failed":
		return TxFailed
	}
	return TxPending
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
