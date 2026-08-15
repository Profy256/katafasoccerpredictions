package pay_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
)

func TestTraceCodeIsStableAndDerivedFromTheReference(t *testing.T) {
	reference := uuid.MustParse("3f9a2b7c-1234-4abc-8def-0123456789ab")

	got := pay.TraceCode(reference)
	if got != "KTF-3F9A2B7C" {
		t.Errorf("TraceCode = %q, want KTF-3F9A2B7C", got)
	}

	// Deriving rather than storing is only safe if it is deterministic.
	if again := pay.TraceCode(reference); again != got {
		t.Errorf("TraceCode is not stable: %q then %q", got, again)
	}

	// Different references must not collide within the same payment.
	other := pay.TraceCode(uuid.MustParse("9c8b7a65-1234-4abc-8def-0123456789ab"))
	if other == got {
		t.Error("two different references produced the same trace code")
	}
}

func TestCollectionDescriptionLeadsWithTheTraceCode(t *testing.T) {
	reference := uuid.MustParse("3f9a2b7c-1234-4abc-8def-0123456789ab")

	got := pay.CollectionDescription(reference, domain.PackageVIP, "Saturday Banker")
	if want := "KTF-3F9A2B7C VIP slip: Saturday Banker"; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}

	// The code has to survive truncation downstream, so it must come first and
	// the title must be what gets trimmed.
	long := strings.Repeat("Manchester United versus Liverpool ", 10)
	truncated := pay.CollectionDescription(reference, domain.PackageAkatambula, long)
	if !strings.HasPrefix(truncated, "KTF-3F9A2B7C ") {
		t.Errorf("truncated description lost its trace code: %q", truncated)
	}
	if len(truncated) > 96 {
		t.Errorf("description is %d chars, over the 96 limit: %q", len(truncated), truncated)
	}
}

// A narration goes through a gateway, an SMS, and often a CSV export of a
// settlement statement. Characters that break any of those are stripped.
func TestDescriptionIsSafeForStatementNarrations(t *testing.T) {
	reference := uuid.MustParse("3f9a2b7c-1234-4abc-8def-0123456789ab")

	got := pay.CollectionDescription(reference, domain.PackageOrdinary,
		"Weekend\n\"Special\", the one;  they  stake")

	for _, bad := range []string{"\n", "\r", "\t", `"`, ",", ";"} {
		if strings.Contains(got, bad) {
			t.Errorf("description contains %q, which breaks statement exports: %q", bad, got)
		}
	}
	if strings.Contains(got, "  ") {
		t.Errorf("description has collapsed whitespace left in it: %q", got)
	}
}

func TestDescriptionSurvivesAnEmptyTitle(t *testing.T) {
	reference := uuid.MustParse("3f9a2b7c-1234-4abc-8def-0123456789ab")

	// A slip with a title of only stripped characters must still produce a
	// traceable description rather than a dangling colon.
	got := pay.CollectionDescription(reference, domain.PackageVIP, ",,,")
	if got != "KTF-3F9A2B7C VIP slip" {
		t.Errorf("description = %q, want the bare prefix", got)
	}
	if strings.HasSuffix(got, ":") {
		t.Errorf("description ends with a dangling separator: %q", got)
	}
}

// Metadata travels further than the transaction does. It carries identifiers,
// never the payer's phone number or email.
func TestCollectionMetadataCarriesNoPersonalData(t *testing.T) {
	metadata := pay.CollectionMetadata(uuid.New(), uuid.New(), uuid.New(), domain.PackageVIP, "production")

	for _, required := range []string{"purchase_id", "slip_id", "user_id", "package", "environment"} {
		if metadata[required] == "" {
			t.Errorf("metadata is missing %s", required)
		}
	}
	for key, value := range metadata {
		if strings.HasPrefix(value, "+256") || strings.Contains(value, "@") {
			t.Errorf("metadata key %q carries personal data: %q", key, value)
		}
	}
	// A sandbox payment must never be mistaken for a live one on a statement.
	if metadata["environment"] != "production" {
		t.Errorf("environment = %q, want it stamped through", metadata["environment"])
	}
}

func TestRefundDescriptionIsDistinguishableFromACollection(t *testing.T) {
	got := pay.RefundDescription(
		uuid.MustParse("9c8b7a65-1234-4abc-8def-0123456789ab"),
		"KTF-3F9A2B7C", "every selection was voided")

	if !strings.Contains(got, "refund") {
		t.Errorf("a refund narration must say so: %q", got)
	}
	// It has to point back at the collection it reverses, or the two lines on
	// the statement cannot be paired.
	if !strings.Contains(got, "KTF-3F9A2B7C") {
		t.Errorf("refund narration does not reference the original payment: %q", got)
	}
}
