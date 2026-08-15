// Package auth handles password hashing, opaque session tokens, and the
// middleware that resolves a cookie into a user.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. These are deliberately expensive: the whole point of a
// password hash is that verifying one is slow enough to make guessing
// impractical.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrInvalidHash is returned when a stored hash cannot be parsed. It is
// treated as a failed login rather than a server error, so a corrupt row
// cannot be used to distinguish accounts.
var ErrInvalidHash = errors.New("auth: invalid password hash")

// HashPassword returns a PHC-format argon2id string, which carries its own
// parameters. That is what lets the cost be raised later without invalidating
// every existing password: old hashes still verify under the parameters they
// were made with.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether the password matches the stored hash.
//
// The comparison is constant time. A timing difference here leaks whether the
// first bytes of a guess were right, which is enough to attack a hash offline.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, fmt.Errorf("%w: unsupported argon2 version %d", ErrInvalidHash, version)
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// DummyHash is verified against when an email does not exist, so that a login
// attempt costs the same whether or not the account is real. Without it,
// response time enumerates accounts.
var DummyHash string

func init() {
	h, err := HashPassword("katafa-nonexistent-account-placeholder")
	if err != nil {
		panic("auth: cannot build dummy hash: " + err.Error())
	}
	DummyHash = h
}
