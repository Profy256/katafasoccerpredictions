package pay

import (
	"fmt"
	"strings"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Ugandan mobile money prefixes, after the +256 country code.
//
// The network matters because a collection is routed to MTN or Airtel by
// number, and a number on neither network cannot be collected from at all —
// better to say so than to let the gateway reject it later.
var (
	mtnPrefixes    = []string{"77", "78", "76", "39"}
	airtelPrefixes = []string{"70", "75", "74", "20"}
)

// NormalisePhone converts any of the formats a Ugandan user might type into
// the +256XXXXXXXXX form the gateway expects.
//
//	0771234567     → +256771234567
//	256771234567   → +256771234567
//	+256 77 123 4567 → +256771234567
func NormalisePhone(input string) (string, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '+' {
			return r
		}
		return -1
	}, input)

	var national string
	switch {
	case strings.HasPrefix(cleaned, "+256"):
		national = strings.TrimPrefix(cleaned, "+256")
	case strings.HasPrefix(cleaned, "256"):
		national = strings.TrimPrefix(cleaned, "256")
	case strings.HasPrefix(cleaned, "0"):
		national = strings.TrimPrefix(cleaned, "0")
	default:
		national = cleaned
	}

	if len(national) != 9 {
		return "", fmt.Errorf("%w: %q is not a 9-digit Ugandan mobile number", domain.ErrUnprocessable, input)
	}
	if MobileProvider(national) == "" {
		return "", fmt.Errorf("%w: %q is not on a supported mobile money network", domain.ErrUnprocessable, input)
	}
	return "+256" + national, nil
}

// MobileProvider returns "mtn", "airtel", or "" for a 9-digit national number.
func MobileProvider(national string) string {
	if len(national) < 2 {
		return ""
	}
	prefix := national[:2]
	for _, p := range mtnPrefixes {
		if prefix == p {
			return "mtn"
		}
	}
	for _, p := range airtelPrefixes {
		if prefix == p {
			return "airtel"
		}
	}
	return ""
}
