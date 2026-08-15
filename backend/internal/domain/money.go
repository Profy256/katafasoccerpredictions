package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// UGX is a whole number of Ugandan shillings.
//
// UGX is a zero-decimal currency: there is no minor unit in practice, so this
// is never float, never "cents", and never a type that secretly multiplies by
// 100. It maps to BIGINT in Postgres.
type UGX int64

func (u UGX) String() string { return fmt.Sprintf("UGX %d", int64(u)) }

// MarzPay's collection endpoint enforces this range per transaction. Validated
// before calling out, so a rejection is caught as a 422 rather than as a
// provider error the user cannot act on.
const (
	MinCollectionUGX UGX = 500
	MaxCollectionUGX UGX = 10_000_000
)

// InCollectionRange reports whether an amount can be collected via MarzPay.
func (u UGX) InCollectionRange() bool {
	return u >= MinCollectionUGX && u <= MaxCollectionUGX
}

// Odds are decimal odds, stored as NUMERIC(7,3). Accumulator odds multiply,
// and float drift across five legs is visible to users, so this is
// decimal.Decimal throughout and never float64.
type Odds = decimal.Decimal

// AccumulateOdds multiplies the odds of every leg. An empty slice returns 1 —
// an accumulator with no surviving legs has no price, and the caller (slip
// settlement, when every tip voided) treats that as the void case.
func AccumulateOdds(legs []Odds) Odds {
	total := decimal.NewFromInt(1)
	for _, o := range legs {
		total = total.Mul(o)
	}
	return total
}
