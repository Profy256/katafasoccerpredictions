package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/logging"
)

// The middleware chain, outermost first:
//
//	request id → logging → recovery → CORS → rate limit → session
//
// Recovery sits inside logging so a panic is still logged with its request id,
// and inside request id so the response can carry one.

type middleware func(http.Handler) http.Handler

func chain(h http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// statusRecorder captures the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// requestID attaches an id to the context and echoes it as X-Request-Id, so a
// user reporting a failure can be traced to exact log lines.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		logging.FromContext(r.Context(), s.Log).Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
		)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.FromContext(r.Context(), s.Log).Error("panic",
					"recovered", rec, "path", r.URL.Path)
				render.Problemf(w, r, http.StatusInternalServerError,
					"internal", "Something went wrong", "The request could not be completed")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors allows the configured Next.js origins only. There is no wildcard: most
// reads are server-side, so the browser-facing surface is narrow by default.
func (s *Server) cors(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(s.Config.AllowedOrigins))
	for _, o := range s.Config.AllowedOrigins {
		allowed[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Rate limits. Public reads are limited per IP; writes per user where there is
// one, per IP otherwise.
const (
	readLimit   = 300
	readWindow  = time.Minute
	writeLimit  = 30
	writeWindow = time.Minute
	loginLimit  = 10
	loginWindow = 15 * time.Minute
)

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, window := readLimit, readWindow
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			limit, window = writeLimit, writeWindow
		}

		bucket := "ip:" + clientIP(r)
		if u, ok := auth.UserFrom(r.Context()); ok {
			bucket = "user:" + u.ID.String()
		}

		allowed, retryAfter, err := s.DB.AllowRequest(r.Context(), bucket, limit, window)
		if err != nil {
			// A limiter that cannot reach Postgres must not take the site
			// down with it. Log and allow: availability beats a perfect limit
			// on a read-mostly public API.
			logging.FromContext(r.Context(), s.Log).Warn("rate limiter unavailable", "err", err)
			next.ServeHTTP(w, r)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			render.Problemf(w, r, http.StatusTooManyRequests, "rate-limited",
				"Too many requests", "Retry in %d seconds", int(retryAfter.Seconds())+1)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// session resolves the cookie into a user, if one is present and valid.
//
// It never rejects: endpoints that require a user say so themselves via
// requireUser, and endpoints where auth is optional (the slip list) need the
// request to continue either way.
func (s *Server) session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, err := s.DB.UserBySessionToken(r.Context(), auth.HashToken(cookie.Value))
		if err != nil {
			if !errors.Is(err, domain.ErrUnauthorized) {
				logging.FromContext(r.Context(), s.Log).Error("session lookup", "err", err)
			}
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
	})
}

// requireUser wraps a handler that needs a session.
func (s *Server) requireUser(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); !ok {
			render.Error(w, r, domain.ErrUnauthorized, s.Log)
			return
		}
		h(w, r)
	}
}

// requireAdmin wraps a handler that needs role = 'admin'.
func (s *Server) requireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFrom(r.Context())
		if !ok {
			render.Error(w, r, domain.ErrUnauthorized, s.Log)
			return
		}
		if user.Role != domain.RoleAdmin {
			render.Error(w, r, domain.ErrForbidden, s.Log)
			return
		}
		h(w, r)
	}
}

// clientIP prefers X-Forwarded-For's first entry, which is the client as seen
// by the edge. It is only trustworthy behind a proxy that overwrites it, which
// is how this is deployed.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginBucket rate-limits per email *and* per IP. Per IP alone lets a botnet
// spray one account; per email alone lets one IP enumerate accounts.
func loginBuckets(r *http.Request, email string) []string {
	return []string{
		fmt.Sprintf("login-ip:%s", clientIP(r)),
		fmt.Sprintf("login-email:%s", strings.ToLower(email)),
	}
}
