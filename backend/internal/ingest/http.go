package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// ErrRateLimited is a 429. The caller honours Retry-After and halves the local
// limiter for the rest of the day.
var ErrRateLimited = errors.New("ingest: provider rate limited")

// Archiver stores a response before it is parsed, so a parser bug is
// replayable without spending request budget.
type Archiver interface {
	ArchivePayload(ctx context.Context, provider, endpoint, requestURL string, httpStatus int, body []byte) error
}

// Fetcher is the shared HTTP behaviour of both provider clients: retries,
// rate-limit handling, and payload archiving.
type Fetcher struct {
	Provider string
	HTTP     *http.Client
	Archive  Archiver
	// Header is set on every request — the provider's auth header.
	Header func(*http.Request)
}

// MaxAttempts caps retries. Exponential backoff, and only on 5xx and network
// errors: a 4xx will not become a 2xx by being asked again.
const MaxAttempts = 5

// Get fetches and archives one endpoint.
func (f *Fetcher) Get(ctx context.Context, endpoint, url string) ([]byte, error) {
	var lastErr error

	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("ingest: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if f.Header != nil {
			f.Header(req)
		}

		resp, err := f.HTTP.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("ingest: %s %s: %w", f.Provider, endpoint, err)
			if !sleepBackoff(ctx, attempt) {
				return nil, lastErr
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()

		// Archived before parsing, whatever the status.
		if f.Archive != nil {
			if err := f.Archive.ArchivePayload(ctx, f.Provider, endpoint, url, resp.StatusCode, body); err != nil {
				return nil, fmt.Errorf("ingest: archive payload: %w", err)
			}
		}
		if readErr != nil {
			return nil, fmt.Errorf("ingest: read %s response: %w", endpoint, readErr)
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			if wait := retryAfter(resp); wait > 0 && attempt < MaxAttempts {
				if !sleepFor(ctx, wait) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, ErrRateLimited

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("ingest: %s %s returned %d", f.Provider, endpoint, resp.StatusCode)
			if !sleepBackoff(ctx, attempt) {
				return nil, lastErr
			}
			continue

		case resp.StatusCode >= 400:
			// Not retried. Logged, archived, and the job fails.
			return nil, fmt.Errorf("ingest: %s %s returned %d: %s",
				f.Provider, endpoint, resp.StatusCode, truncate(body, 256))
		}

		return body, nil
	}
	return nil, lastErr
}

func retryAfter(resp *http.Response) time.Duration {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return 0
}

func sleepBackoff(ctx context.Context, attempt int) bool {
	if attempt >= MaxAttempts {
		return false
	}
	return sleepFor(ctx, time.Duration(math.Pow(2, float64(attempt)))*time.Second)
}

func sleepFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
