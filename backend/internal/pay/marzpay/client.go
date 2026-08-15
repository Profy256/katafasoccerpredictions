// Package marzpay is the MarzPay collections client.
//
// Verified against https://wallet.wearemarz.com/documentation/collections.
package marzpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
)

// Client talks to MarzPay over HTTP Basic auth.
type Client struct {
	BaseURL string
	APIUser string
	APIKey  string
	// WebhookSecret verifies callback signatures.
	WebhookSecret string
	HTTP          *http.Client
}

func New(baseURL, apiUser, apiKey, webhookSecret string) *Client {
	return &Client{
		BaseURL:       strings.TrimSuffix(baseURL, "/"),
		APIUser:       apiUser,
		APIKey:        apiKey,
		WebhookSecret: webhookSecret,
		HTTP:          &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Name() string { return "marzpay" }

// collectPayload is the request body. Amount is whole shillings: UGX has no
// minor unit, so there is no conversion to get wrong.
type collectPayload struct {
	Amount      int64               `json:"amount"`
	Country     string              `json:"country"`
	Reference   string              `json:"reference"`
	PhoneNumber string              `json:"phone_number"`
	Method      string              `json:"method"`
	Description string              `json:"description"`
	CallbackURL string              `json:"callback_url"`
	Metadata    []map[string]string `json:"metadata,omitempty"`
}

type collectResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Transaction struct {
			UUID   string `json:"uuid"`
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"transaction"`
		Provider string `json:"provider"`
	} `json:"data"`
}

func (c *Client) Collect(ctx context.Context, req pay.CollectRequest) (pay.CollectResponse, error) {
	metadata := make([]map[string]string, 0, len(req.Metadata))
	for k, v := range req.Metadata {
		metadata = append(metadata, map[string]string{k: v})
	}

	body := collectPayload{
		Amount:      int64(req.AmountUGX),
		Country:     "UG",
		Reference:   req.Reference.String(),
		PhoneNumber: req.PhoneNumber,
		Method:      "mobile_money",
		Description: req.Description,
		CallbackURL: req.CallbackURL,
		Metadata:    metadata,
	}

	raw, err := c.post(ctx, "/collect-money", body)
	if err != nil {
		return pay.CollectResponse{Raw: raw}, err
	}

	var decoded collectResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return pay.CollectResponse{Raw: raw}, fmt.Errorf("marzpay: decode collect response: %w", err)
	}

	return pay.CollectResponse{
		ProviderUUID:   decoded.Data.Transaction.UUID,
		ProviderTxnID:  decoded.Data.Transaction.ID,
		Status:         mapStatus(decoded.Data.Transaction.Status, decoded.Status),
		MobileProvider: decoded.Data.Provider,
		Raw:            raw,
	}, nil
}

func (c *Client) Status(ctx context.Context, providerUUID string) (pay.CollectResponse, error) {
	raw, err := c.get(ctx, "/collect-money/"+providerUUID)
	if err != nil {
		return pay.CollectResponse{Raw: raw}, err
	}

	var decoded collectResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return pay.CollectResponse{Raw: raw}, fmt.Errorf("marzpay: decode status response: %w", err)
	}
	return pay.CollectResponse{
		ProviderUUID:   decoded.Data.Transaction.UUID,
		ProviderTxnID:  decoded.Data.Transaction.ID,
		Status:         mapStatus(decoded.Data.Transaction.Status, decoded.Status),
		MobileProvider: decoded.Data.Provider,
		Raw:            raw,
	}, nil
}

func (c *Client) Refund(ctx context.Context, reference uuid.UUID, amount domain.UGX, phone, reason string) (pay.CollectResponse, error) {
	body := collectPayload{
		Amount:      int64(amount),
		Country:     "UG",
		Reference:   reference.String(),
		PhoneNumber: phone,
		Method:      "mobile_money",
		Description: reason,
	}
	raw, err := c.post(ctx, "/send-money", body)
	if err != nil {
		return pay.CollectResponse{Raw: raw}, err
	}

	var decoded collectResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return pay.CollectResponse{Raw: raw}, fmt.Errorf("marzpay: decode refund response: %w", err)
	}
	return pay.CollectResponse{
		ProviderUUID:  decoded.Data.Transaction.UUID,
		ProviderTxnID: decoded.Data.Transaction.ID,
		Status:        mapStatus(decoded.Data.Transaction.Status, decoded.Status),
		Raw:           raw,
	}, nil
}

// VerifyWebhook checks the HMAC-SHA256 of the raw body.
//
// hmac.Equal rather than ==: a timing difference here would let an attacker
// discover a valid signature byte by byte.
func (c *Client) VerifyWebhook(body []byte, signature string) bool {
	if c.WebhookSecret == "" || signature == "" {
		return false
	}
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")

	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.WebhookSecret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), provided)
}

// mapStatus translates MarzPay's vocabulary into ours. An unrecognised status
// maps to pending, never to completed: the worst outcome of guessing pending
// is that reconciliation polls again, while guessing completed grants a slip
// nobody paid for.
func mapStatus(txStatus, envelope string) pay.TxStatus {
	switch strings.ToLower(txStatus) {
	case "successful", "success", "completed":
		return pay.TxCompleted
	case "failed", "rejected", "cancelled":
		return pay.TxFailed
	case "expired", "timeout":
		return pay.TxExpired
	case "processing", "in_progress":
		return pay.TxProcessing
	case "pending":
		return pay.TxPending
	}
	if strings.EqualFold(envelope, "error") {
		return pay.TxFailed
	}
	return pay.TxPending
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marzpay: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("marzpay: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("marzpay: build request: %w", err)
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	req.SetBasicAuth(c.APIUser, c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marzpay: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// The body is read whatever the status: an error body is the only
	// explanation of a failure, and it is archived alongside the request.
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("marzpay: read response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("marzpay: %s %s returned %d: %s",
			req.Method, req.URL.Path, resp.StatusCode, truncate(raw, 512))
	}
	return raw, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
