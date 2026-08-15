package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

func (db *DB) CreateUser(ctx context.Context, q Querier, u domain.User) (domain.User, error) {
	err := q.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, phone, role)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at`,
		u.Email, u.PasswordHash, u.Name, u.Phone, u.Role).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, fmt.Errorf("%w: email already registered", domain.ErrConflict)
		}
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// UserByEmail looks up by CITEXT, so the comparison is case-insensitive in the
// database rather than by lowercasing in Go and hoping every caller does.
func (db *DB) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, phone, role, created_at
		FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Phone, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (db *DB) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var u domain.User
	err := db.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, phone, role, created_at
		FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Phone, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

// CreateSession stores the sha256 of an opaque token. The token itself is
// never written down: a database leak yields hashes, not usable sessions.
func (db *DB) CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, userAgent string, expiresAt time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, user_agent, expires_at)
		VALUES ($1,$2,$3,$4)`, tokenHash, userID, userAgent, expiresAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// UserBySessionToken resolves a session to its user, rejecting expired and
// revoked ones in the query. Revocation must take effect immediately —
// entitlement to a paid slip is worth money.
func (db *DB) UserBySessionToken(ctx context.Context, tokenHash []byte) (domain.User, error) {
	var u domain.User
	err := db.Pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.name, u.phone, u.role, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
		  AND s.revoked_at IS NULL`, tokenHash).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Phone, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("query session: %w", err)
	}
	return u, nil
}

func (db *DB) RevokeSession(ctx context.Context, tokenHash []byte) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions is the daily cleanup. Revoked rows are kept for a
// grace period so that "was this token ever valid" stays answerable.
func (db *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at < now() - interval '30 days'`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AllowRequest is a fixed-window rate limiter kept in Postgres, so it survives
// a restart and works across replicas.
//
// Returns false when the bucket is over its limit for the current window.
// Login is limited per email *and* per IP: per IP alone lets a botnet spray one
// account, per email alone lets one IP enumerate accounts.
func (db *DB) AllowRequest(ctx context.Context, bucket string, limit int, window time.Duration) (bool, time.Duration, error) {
	windowStart := time.Now().UTC().Truncate(window)

	var count int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO rate_limits (bucket, window_start, count)
		VALUES ($1,$2,1)
		ON CONFLICT (bucket, window_start)
		DO UPDATE SET count = rate_limits.count + 1
		RETURNING count`, bucket, windowStart).Scan(&count)
	if err != nil {
		return false, 0, fmt.Errorf("rate limit: %w", err)
	}
	if count > limit {
		return false, time.Until(windowStart.Add(window)), nil
	}
	return true, 0, nil
}

// PruneRateLimits drops windows that have closed.
func (db *DB) PruneRateLimits(ctx context.Context) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM rate_limits WHERE window_start < now() - interval '1 day'`)
	if err != nil {
		return fmt.Errorf("prune rate limits: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
