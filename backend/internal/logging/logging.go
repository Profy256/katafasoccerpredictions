// Package logging configures structured JSON logging to stdout and carries a
// request id through context so every line of a request can be correlated.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey int

const requestIDKey ctxKey = iota

// New builds the process logger. JSON to stdout: the deployment target
// collects stdout, and a human-readable format there is a format that has to
// be parsed back into fields by something else.
func New(level string, development bool) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	if development {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// WithRequestID attaches a request id to the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request id attached to ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// FromContext returns a logger tagged with the context's request id, so
// handlers do not have to thread one through their own signatures.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return base.With("request_id", id)
	}
	return base
}
