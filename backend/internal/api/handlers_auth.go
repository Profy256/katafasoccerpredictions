package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

type registerRequest struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Name     string  `json:"name"`
	Phone    *string `json:"phone,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// MinPasswordLength is a floor, not a policy. Composition rules push people
// toward predictable substitutions; length is what actually helps.
const MinPasswordLength = 10

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	if !strings.Contains(req.Email, "@") || req.Email == "" {
		render.Error(w, r, fmt.Errorf("%w: a valid email is required", domain.ErrValidation), s.Log)
		return
	}
	if len(req.Password) < MinPasswordLength {
		render.Error(w, r, fmt.Errorf("%w: password must be at least %d characters",
			domain.ErrValidation, MinPasswordLength), s.Log)
		return
	}
	if req.Name == "" {
		render.Error(w, r, fmt.Errorf("%w: name is required", domain.ErrValidation), s.Log)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	var user domain.User
	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		user, err = s.DB.CreateUser(r.Context(), tx, domain.User{
			Email:        req.Email,
			PasswordHash: hash,
			Name:         req.Name,
			Phone:        req.Phone,
			Role:         domain.RoleUser,
		})
		return err
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	if err := s.startSession(w, r, user); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	req.Email = strings.TrimSpace(req.Email)

	// Limited per email *and* per IP: per IP alone lets a botnet spray one
	// account, per email alone lets one IP enumerate accounts.
	for _, bucket := range loginBuckets(r, req.Email) {
		allowed, retryAfter, err := s.DB.AllowRequest(r.Context(), bucket, loginLimit, loginWindow)
		if err != nil {
			render.Error(w, r, err, s.Log)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprint(int(retryAfter.Seconds())+1))
			render.Problemf(w, r, http.StatusTooManyRequests, "rate-limited",
				"Too many attempts", "Retry in %d seconds", int(retryAfter.Seconds())+1)
			return
		}
	}

	user, err := s.DB.UserByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		render.Error(w, r, err, s.Log)
		return
	}

	// A missing account is verified against a dummy hash so the attempt costs
	// the same either way. Without this, response time enumerates accounts.
	storedHash := auth.DummyHash
	if err == nil {
		storedHash = user.PasswordHash
	}

	ok, verifyErr := auth.VerifyPassword(req.Password, storedHash)
	if verifyErr != nil && !errors.Is(verifyErr, auth.ErrInvalidHash) {
		render.Error(w, r, verifyErr, s.Log)
		return
	}
	if !ok || err != nil {
		// One message for both failures: "no such account" and "wrong
		// password" must be indistinguishable.
		render.Problemf(w, r, http.StatusUnauthorized, "invalid-credentials",
			"Invalid credentials", "The email or password is incorrect")
		return
	}

	if err := s.startSession(w, r, user); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
		if err := s.DB.RevokeSession(r.Context(), auth.HashToken(cookie.Value)); err != nil {
			render.Error(w, r, err, s.Log)
			return
		}
	}
	auth.ClearSessionCookie(w, s.cookieOptions())
	render.NoContent(w)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFrom(r.Context())
	render.Status(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user domain.User) error {
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := s.DB.CreateSession(r.Context(), hash, user.ID,
		r.UserAgent(), time.Now().Add(auth.SessionTTL)); err != nil {
		return err
	}
	auth.SetSessionCookie(w, token, s.cookieOptions())
	return nil
}

// maxBodyBytes caps request bodies. Admin slip payloads are the largest thing
// posted here and they are small.
const maxBodyBytes = 256 << 10

func decodeBody(r *http.Request, into any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(body)
	// Unknown fields are an error rather than ignored: a misspelled field that
	// silently does nothing is how a price ends up unset.
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(into); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: a JSON body is required", domain.ErrValidation)
		}
		return fmt.Errorf("%w: %s", domain.ErrValidation, err)
	}
	return nil
}
