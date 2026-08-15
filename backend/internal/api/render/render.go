// Package render encodes responses: JSON bodies, ETags, cache headers, and
// RFC 9457 problem+json errors.
package render

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// ErrorBaseURI namespaces the `type` field of problem responses.
const ErrorBaseURI = "https://katafa.app/errors/"

// Problem is an RFC 9457 problem detail.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// JSON writes a 200 with an ETag and the given Cache-Control.
//
// Published data is immutable, so ETag plus a max-age is safe here and lets the
// Next.js data cache and any CDN do the work.
func JSON(w http.ResponseWriter, r *http.Request, body any, cacheControl string) {
	encoded, err := json.Marshal(body)
	if err != nil {
		Error(w, r, fmt.Errorf("encode response: %w", err), nil)
		return
	}

	etag := weakETag(encoded)
	w.Header().Set("ETag", etag)
	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}

	// A conditional request that still matches costs the client nothing but a
	// round trip.
	if matches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

// Status writes a JSON body with an explicit status and no caching. Used for
// writes, which are never cacheable.
func Status(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// NoContent writes a 204.
func NoContent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// Error maps a domain error onto a problem+json response.
//
// Only the sentinel errors carry their message to the client. Anything else is
// a 500 with a generic detail, because an unexpected error's text tends to
// contain a query, a column name, or a path.
func Error(w http.ResponseWriter, r *http.Request, err error, log *slog.Logger) {
	problem := Problem{
		Type:     ErrorBaseURI + "internal",
		Title:    "Something went wrong",
		Status:   http.StatusInternalServerError,
		Instance: r.URL.Path,
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		problem = Problem{
			Type: ErrorBaseURI + "not-found", Title: "Not found",
			Status: http.StatusNotFound, Instance: r.URL.Path,
		}
	case errors.Is(err, domain.ErrUnauthorized):
		problem = Problem{
			Type: ErrorBaseURI + "unauthorized", Title: "Authentication required",
			Status: http.StatusUnauthorized, Instance: r.URL.Path,
		}
	case errors.Is(err, domain.ErrForbidden):
		problem = Problem{
			Type: ErrorBaseURI + "forbidden", Title: "Not permitted",
			Status: http.StatusForbidden, Instance: r.URL.Path,
		}
	case errors.Is(err, domain.ErrConflict):
		problem = Problem{
			Type: ErrorBaseURI + "conflict", Title: "State conflict",
			Status: http.StatusConflict, Detail: message(err), Instance: r.URL.Path,
		}
	case errors.Is(err, domain.ErrUnprocessable):
		problem = Problem{
			Type: ErrorBaseURI + "unprocessable", Title: "Request rejected",
			Status: http.StatusUnprocessableEntity, Detail: message(err), Instance: r.URL.Path,
		}
	case errors.Is(err, domain.ErrValidation):
		problem = Problem{
			Type: ErrorBaseURI + "invalid-request", Title: "Invalid request",
			Status: http.StatusBadRequest, Detail: message(err), Instance: r.URL.Path,
		}
	default:
		if log != nil {
			log.Error("unhandled error", "err", err, "path", r.URL.Path, "method", r.Method)
		}
	}

	writeProblem(w, problem)
}

// Problemf writes a specific problem, for cases the sentinel errors do not
// cover — a rate limit with its Retry-After, for instance.
func Problemf(w http.ResponseWriter, r *http.Request, status int, kind, title, format string, args ...any) {
	writeProblem(w, Problem{
		Type:     ErrorBaseURI + kind,
		Title:    title,
		Status:   status,
		Detail:   fmt.Sprintf(format, args...),
		Instance: r.URL.Path,
	})
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// message strips the sentinel prefix so the client sees the specific detail
// rather than "conflict: slip … is not open".
func message(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"conflict: ", "unprocessable: ", "validation failed: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	return msg
}

func weakETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`
}

// matches handles the comma-separated If-None-Match list, including "*".
func matches(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

// Cache-Control values, one per endpoint class in API.md.
const (
	CacheReference = "public, max-age=3600" // leagues, packages, analysts
	CacheFeed      = "public, max-age=300"  // feed, match detail
	CacheAccuracy  = "public, max-age=900"  // accuracy, settled ledger, analyst records
	CacheFreeTips  = "public, max-age=600"  // today's shortlist
	CacheStats     = "public, max-age=600"  // coverage
	// Slip responses vary by whether the viewer owns each one, so they are
	// never shared-cacheable even though the metadata looks public.
	CachePrivate = "private, no-store"
	CacheNone    = "no-store"
)
