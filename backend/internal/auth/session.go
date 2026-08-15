package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// SessionCookieName is the cookie the opaque token travels in.
const SessionCookieName = "katafa_session"

// SessionTTL is how long a session lasts before it must be re-established.
const SessionTTL = 30 * 24 * time.Hour

// tokenBytes is the entropy of an opaque session token.
const tokenBytes = 32

// NewToken returns a fresh opaque token and its sha256 hash.
//
// Opaque tokens hashed at rest, not JWTs: a purchase must be revocable
// immediately, and a stolen JWT stays valid until it expires. Slip access is
// worth money, so "revoke now" has to mean now.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: read token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is what is stored and what lookups compare against.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CookieOptions are the deployment-dependent parts of the session cookie.
type CookieOptions struct {
	Domain string
	// Secure is false only in local development, where there is no TLS. It is
	// derived from the environment rather than exposed as its own setting, so
	// it cannot be switched off in production by editing one value.
	Secure bool
}

// SetSessionCookie writes the session cookie.
//
// HttpOnly so script cannot read it, SameSite=Lax so it survives ordinary
// top-level navigation from the site while not riding along on cross-site
// POSTs.
func SetSessionCookie(w http.ResponseWriter, token string, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   opts.Domain,
		Expires:  time.Now().Add(SessionTTL),
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the cookie on logout.
func ClearSessionCookie(w http.ResponseWriter, opts CookieOptions) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   opts.Domain,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   opts.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

type ctxKey int

const userKey ctxKey = iota

// WithUser attaches the authenticated user to the context.
func WithUser(ctx context.Context, u domain.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFrom returns the authenticated user, if any.
func UserFrom(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey).(domain.User)
	return u, ok
}

// IsAdmin reports whether the request is from an admin.
func IsAdmin(ctx context.Context) bool {
	u, ok := UserFrom(ctx)
	return ok && u.Role == domain.RoleAdmin
}
