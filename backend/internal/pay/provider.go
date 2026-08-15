// Package pay handles mobile money collection and entitlement.
//
// The provider sits behind an interface so tests use a fake: there is no code
// path by which a test can reach the live API, which matters when the live API
// moves real money.
package pay

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// TxStatus is the lifecycle of one collection attempt.
type TxStatus string

const (
	TxInitiated  TxStatus = "initiated"
	TxProcessing TxStatus = "processing"
	TxPending    TxStatus = "pending"
	TxCompleted  TxStatus = "completed"
	TxFailed     TxStatus = "failed"
	TxExpired    TxStatus = "expired"
)

// IsFinal reports whether the status can still change. Reconciliation only
// polls non-final transactions.
func (s TxStatus) IsFinal() bool {
	return s == TxCompleted || s == TxFailed || s == TxExpired
}

// CollectRequest is one mobile money collection.
type CollectRequest struct {
	// Reference is our idempotency key, a UUIDv4. The provider enforces
	// uniqueness on it, so a retried attempt uses a *new* reference and a new
	// transaction row against the same purchase.
	Reference   uuid.UUID
	AmountUGX   domain.UGX
	PhoneNumber string
	Description string
	CallbackURL string
	Metadata    map[string]string
}

// CollectResponse is what the provider said.
type CollectResponse struct {
	ProviderUUID   string
	ProviderTxnID  string
	Status         TxStatus
	MobileProvider string
	Raw            []byte
}

// Provider is the payment gateway.
type Provider interface {
	Name() string
	// Collect requests payment from the payer's mobile money account.
	Collect(ctx context.Context, req CollectRequest) (CollectResponse, error)
	// Status polls one transaction. This — not the webhook — is what
	// guarantees users get what they paid for; the webhook is an optimisation
	// that makes the common case fast.
	Status(ctx context.Context, providerUUID string) (CollectResponse, error)
	// Refund disburses back to the payer.
	Refund(ctx context.Context, reference uuid.UUID, amount domain.UGX, phone, reason string) (CollectResponse, error)
	// VerifyWebhook reports whether a callback body carries a valid signature.
	VerifyWebhook(body []byte, signature string) bool
}

// ValidateCollection checks what can be checked before spending a network
// call, so a rejection surfaces as a 422 the caller can act on rather than as
// a provider error they cannot.
func ValidateCollection(amount domain.UGX, phone string) error {
	if !amount.InCollectionRange() {
		return fmt.Errorf("%w: amount %d is outside the %d–%d shilling range the gateway accepts",
			domain.ErrUnprocessable, int64(amount), int64(domain.MinCollectionUGX), int64(domain.MaxCollectionUGX))
	}
	if _, err := NormalisePhone(phone); err != nil {
		return err
	}
	return nil
}
