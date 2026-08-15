package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Cursors are opaque to clients and encode (sort_key, id).
//
// Cursor rather than offset pagination on the two endpoints that grow without
// bound: offset pagination over a table that gains rows daily shows duplicates
// across pages, because rows inserted between two requests shift everything
// down.

func encodeCursor(c *postgres.Cursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(
		[]byte(c.SettledAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeCursor(raw string) (*postgres.Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	at, id, ok := strings.Cut(string(decoded), "|")
	if !ok {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	settledAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	return &postgres.Cursor{SettledAt: settledAt, ID: parsed}, nil
}

func encodeSlipCursor(c *postgres.SlipCursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(
		[]byte(c.PublishedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

func decodeSlipCursor(raw string) (*postgres.SlipCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	at, id, ok := strings.Cut(string(decoded), "|")
	if !ok {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", domain.ErrValidation)
	}
	return &postgres.SlipCursor{PublishedAt: publishedAt, ID: parsed}, nil
}
