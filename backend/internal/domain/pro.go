package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PackageCode is a Pro tier product line. Mirrors PackageCode in
// src/api/types.ts.
type PackageCode string

const (
	PackageOrdinary   PackageCode = "ordinary"
	PackageVIP        PackageCode = "vip"
	PackageAkatambula PackageCode = "akatambula"
)

var PackageCodes = []PackageCode{PackageOrdinary, PackageVIP, PackageAkatambula}

func ParsePackageCode(s string) (PackageCode, error) {
	for _, p := range PackageCodes {
		if PackageCode(s) == p {
			return p, nil
		}
	}
	return "", fmt.Errorf("unknown package code %q", s)
}

type Package struct {
	Code            PackageCode `json:"code"`
	Name            string      `json:"name"`
	Tagline         string      `json:"tagline"`
	Description     string      `json:"description"`
	TypicalPriceUGX UGX         `json:"typicalPriceUgx"`
	Highlights      []string    `json:"highlights"`
}

type Analyst struct {
	ID       uuid.UUID     `json:"id"`
	Slug     string        `json:"slug"`
	Name     string        `json:"name"`
	Handle   string        `json:"handle"`
	Initials string        `json:"initials"`
	Bio      string        `json:"bio"`
	Packages []PackageCode `json:"packages"`
	JoinedAt time.Time     `json:"joinedAt"`
}

// SlipStatus advances draft → open → settled, or draft → open → void. It never
// moves backwards; the slips_guard_update trigger enforces that.
type SlipStatus string

const (
	SlipDraft   SlipStatus = "draft"
	SlipOpen    SlipStatus = "open"
	SlipSettled SlipStatus = "settled"
	SlipVoid    SlipStatus = "void"
)

// Slip is a hand-built accumulator sold as one indivisible product.
type Slip struct {
	ID          uuid.UUID
	PackageCode PackageCode
	Title       string
	PriceUGX    UGX
	TotalOdds   Odds
	TipCount    int
	Status      SlipStatus
	PublishedAt *time.Time
	SettledAt   *time.Time
	WonTips     *int
	// SettledOdds is the accumulator after void legs were removed. Written
	// separately because TotalOdds is frozen at publication and must keep
	// showing what was advertised — both figures are shown.
	SettledOdds *Odds
	CreatedBy   uuid.UUID
	AnalystIDs  []uuid.UUID
	CreatedAt   time.Time
}

// Tip is one leg of a slip. It is either fully structured and auto-gradable
// (MatchID, MarketCode and SelectionValue all present) or free text an admin
// grades by hand. The schema's num_nonnulls CHECK forbids the in-between,
// which would silently never settle.
type Tip struct {
	ID             uuid.UUID
	SlipID         uuid.UUID
	AnalystID      uuid.UUID
	MatchID        *uuid.UUID
	FixtureLabel   string
	MarketLabel    string
	SelectionLabel string
	MarketCode     *MarketCode
	SelectionValue *string
	Odds           Odds
	KickoffAt      time.Time
	Note           *string
	Position       int
}

// AutoGradable reports whether settlement can grade this tip from a match
// score without a human.
func (t Tip) AutoGradable() bool {
	return t.MatchID != nil && t.MarketCode != nil && t.SelectionValue != nil
}

type SettledBy string

const (
	SettledByAuto  SettledBy = "auto"
	SettledByAdmin SettledBy = "admin"
)

type TipResult struct {
	TipID         uuid.UUID
	WasCorrect    bool
	ActualOutcome string
	SettledAt     time.Time
	SettledBy     SettledBy
	// SettledByUser names the human who made the call. Mandatory for admin
	// settlements — a person overriding the record must be identified.
	SettledByUser *uuid.UUID
}

// VoidOutcome is the stored actual_outcome for a tip whose match never
// produced a full-time score. A void is not a loss: it has no outcome to
// report, so it is excluded from the accumulator and from hit rates, while a
// loss is counted in both.
const VoidOutcome = "VOID"

type Role string

const (
	RoleUser    Role = "user"
	RoleAnalyst Role = "analyst"
	RoleAdmin   Role = "admin"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Phone        *string   `json:"phone,omitempty"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

type PurchaseStatus string

const (
	PurchasePending  PurchaseStatus = "pending"
	PurchasePaid     PurchaseStatus = "paid"
	PurchaseFailed   PurchaseStatus = "failed"
	PurchaseRefunded PurchaseStatus = "refunded"
)

// Purchase carries its own PriceUGX rather than reading the slip's. It is what
// the user was actually charged and must survive independently of the slip.
type Purchase struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"-"`
	SlipID    uuid.UUID      `json:"slipId"`
	PriceUGX  UGX            `json:"priceUgx"`
	Status    PurchaseStatus `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	PaidAt    *time.Time     `json:"paidAt,omitempty"`
}
