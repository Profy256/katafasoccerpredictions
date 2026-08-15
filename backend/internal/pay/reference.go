package pay

import (
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// Money arriving in the MarzPay account has to be attributable to something.
// Three things carry that attribution, and they are deliberately redundant
// because each is visible in a different place:
//
//   - reference — the UUIDv4 idempotency key. Exact, but unreadable, and it is
//     what the provider deduplicates on.
//   - TraceCode — a short code derived from that reference, embedded in the
//     description so it shows up in the MarzPay dashboard, in the payer's SMS,
//     and in a settlement statement. This is what somebody actually reads off
//     a screen and searches for.
//   - metadata — the structured breakdown, for reconciling in bulk rather than
//     one payment at a time.
//
// A payment that cannot be traced back to a slip and a buyer is a payment that
// turns into a support ticket nobody can close.

// TracePrefix marks a collection as ours in a statement that may carry other
// businesses' traffic.
const TracePrefix = "KTF-"

// TraceCode is the first eight hex digits of the reference, uppercased:
// KTF-3F9A2B7C.
//
// Eight hex digits is 4.3 billion values. Collisions do not matter for
// correctness — the reference is still the key and trace_code is not unique —
// only for a human searching, and at that volume a clash is a curiosity rather
// than a problem.
func TraceCode(reference uuid.UUID) string {
	return TracePrefix + strings.ToUpper(strings.ReplaceAll(reference.String(), "-", "")[:8])
}

// maxDescriptionLen keeps the description inside what a gateway, an SMS, and a
// bank statement narration will each carry without truncating from the right —
// which would eat the slip title first and the trace code never, since the
// code leads.
const maxDescriptionLen = 96

// CollectionDescription is what the payer and the account statement see.
//
// The trace code comes first so it survives any truncation downstream, and the
// package name comes before the title because it is the more useful grouping
// when scanning a statement.
//
//	KTF-3F9A2B7C VIP slip: Saturday Banker
func CollectionDescription(reference uuid.UUID, pkg domain.PackageCode, title string) string {
	label := packageLabel(pkg)

	prefix := TraceCode(reference) + " " + label + " slip"
	title = sanitiseNarration(title)
	if title == "" {
		return prefix
	}

	full := prefix + ": " + title
	if len(full) <= maxDescriptionLen {
		return full
	}
	// Trim the title, never the code. A description ending mid-word is
	// readable; one missing its trace code is not traceable.
	//
	// The marker is ASCII "..." rather than an ellipsis rune: this string
	// passes through SMS gateways and statement exports that mangle anything
	// outside ASCII, and it is measured in bytes here because that is what the
	// length limit counts.
	const marker = "..."
	keep := maxDescriptionLen - len(prefix) - len(": ") - len(marker)
	if keep <= 0 {
		return prefix
	}
	return prefix + ": " + strings.TrimSpace(truncateRunes(title, keep)) + marker
}

// truncateRunes cuts to at most n bytes without splitting a UTF-8 character in
// half, which would put a replacement glyph in the payer's SMS.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := 0
	for i := range s {
		if i > n {
			break
		}
		cut = i
	}
	return s[:cut]
}

// RefundDescription marks a disbursement so an outgoing payment is not
// mistaken for a failed collection when the statement is read later.
func RefundDescription(reference uuid.UUID, original string, reason string) string {
	base := TraceCode(reference) + " refund"
	if original != "" {
		base += " of " + original
	}
	reason = sanitiseNarration(reason)
	if reason == "" {
		return base
	}
	full := base + ": " + reason
	if len(full) > maxDescriptionLen {
		return full[:maxDescriptionLen]
	}
	return full
}

// packageLabel is the human name of a package, used in narrations.
func packageLabel(pkg domain.PackageCode) string {
	switch pkg {
	case domain.PackageOrdinary:
		return "Ordinary"
	case domain.PackageVIP:
		return "VIP"
	case domain.PackageAkatambula:
		return "Akatambula"
	default:
		return "Katafa"
	}
}

// sanitiseNarration strips what mobile money narrations mangle: control
// characters, newlines, and the punctuation that tends to break CSV exports of
// a settlement statement.
func sanitiseNarration(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t' || unicode.IsSpace(r):
			if !lastSpace && b.Len() > 0 {
				b.WriteRune(' ')
				lastSpace = true
			}
		case unicode.IsControl(r), r == ',', r == '"', r == ';':
			// dropped
		default:
			b.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// CollectionMetadata is the structured attribution sent alongside the payment.
//
// MarzPay takes metadata as a list of single-entry objects, which the client
// handles; this is the flat form. Everything here is an identifier or a label,
// never a phone number or an email — metadata travels further than the
// transaction does and is not the place for personal data.
func CollectionMetadata(purchaseID, slipID, userID uuid.UUID, pkg domain.PackageCode, env string) map[string]string {
	return map[string]string{
		"purchase_id": purchaseID.String(),
		"slip_id":     slipID.String(),
		"user_id":     userID.String(),
		"package":     string(pkg),
		"product":     "slip",
		"environment": env,
		"source":      "katafa-web",
	}
}
