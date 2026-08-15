package domain

import "errors"

// Sentinel errors the service layer returns and the HTTP layer translates into
// problem+json. Keeping them here means handlers can map status codes without
// importing every service package.
var (
	// ErrNotFound is also what a non-admin gets for a draft slip, so that
	// unpublished slip ids are not discoverable by probing for a 403.
	ErrNotFound = errors.New("not found")

	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// ErrConflict covers state conflicts: already purchased, slip closed,
	// already settled.
	ErrConflict = errors.New("conflict")

	// ErrUnprocessable is a well-formed request rejected on its content — a
	// phone number outside MarzPay's range, for instance.
	ErrUnprocessable = errors.New("unprocessable")

	// ErrValidation is a malformed body, an unknown enum value, or a bad
	// cursor.
	ErrValidation = errors.New("validation failed")
)
