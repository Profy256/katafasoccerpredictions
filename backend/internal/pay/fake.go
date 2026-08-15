package pay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Fake is the in-memory payment provider used in development and tests.
//
// It exists so that no test can reach the live API. Behaviour is scripted
// rather than random: a test that needs a lost webhook, a duplicate callback,
// or a failure after a success has to be able to ask for exactly that.
type Fake struct {
	mu sync.Mutex

	// NextStatus is what the next Collect reports. Defaults to processing,
	// which is what the real gateway returns while the payer is being
	// prompted.
	NextStatus TxStatus
	// StatusByUUID overrides what Status reports per transaction, so
	// reconciliation tests can flip a transaction to completed without a
	// webhook ever arriving.
	StatusByUUID map[string]TxStatus
	// FailCollect makes the outbound call fail, to exercise the path where a
	// transaction row exists but the provider was never reached.
	FailCollect error

	Secret string

	Collected []CollectRequest
	Refunded  []CollectRequest
}

func NewFake() *Fake {
	return &Fake{
		NextStatus:   TxProcessing,
		StatusByUUID: map[string]TxStatus{},
		Secret:       "fake-webhook-secret",
	}
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) Collect(ctx context.Context, req CollectRequest) (CollectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.FailCollect != nil {
		return CollectResponse{}, f.FailCollect
	}
	f.Collected = append(f.Collected, req)

	providerUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.Reference.String())).String()
	status := f.NextStatus
	if status == "" {
		status = TxProcessing
	}
	f.StatusByUUID[providerUUID] = status

	raw, _ := json.Marshal(map[string]any{
		"status": "success",
		"data": map[string]any{
			"transaction": map[string]any{"uuid": providerUUID, "status": string(status)},
		},
	})
	return CollectResponse{
		ProviderUUID:   providerUUID,
		ProviderTxnID:  "fake-txn-" + providerUUID[:8],
		Status:         status,
		MobileProvider: MobileProvider(strings.TrimPrefix(req.PhoneNumber, "+256")),
		Raw:            raw,
	}, nil
}

func (f *Fake) Status(ctx context.Context, providerUUID string) (CollectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	status, ok := f.StatusByUUID[providerUUID]
	if !ok {
		status = TxPending
	}
	raw, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"transaction": map[string]any{"uuid": providerUUID, "status": string(status)},
		},
	})
	return CollectResponse{
		ProviderUUID:  providerUUID,
		ProviderTxnID: "fake-txn-" + providerUUID[:8],
		Status:        status,
		Raw:           raw,
	}, nil
}

func (f *Fake) Refund(ctx context.Context, reference uuid.UUID, amount domain.UGX, phone, reason string) (CollectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Refunded = append(f.Refunded, CollectRequest{
		Reference: reference, AmountUGX: amount, PhoneNumber: phone, Description: reason,
	})
	return CollectResponse{
		ProviderUUID: uuid.NewString(),
		Status:       TxCompleted,
		Raw:          []byte(`{"status":"success"}`),
	}, nil
}

// SetStatus scripts what Status will report for a transaction.
func (f *Fake) SetStatus(providerUUID string, status TxStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.StatusByUUID[providerUUID] = status
}

func (f *Fake) VerifyWebhook(body []byte, signature string) bool {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(f.Secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}

// Sign produces a valid signature for a body, so tests can send authentic
// callbacks as well as forged ones.
func (f *Fake) Sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(f.Secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
