package ingest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// Budget is per-provider request accounting.
//
// Free tiers have hard ceilings, and the job of this layer is to spend a small
// request budget well. Every outbound call passes through Acquire.
type Budget struct {
	DB *postgres.DB

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	// throttled records providers that returned 429 today. Their local rate is
	// halved for the rest of the day.
	throttled map[string]bool
}

// Limits per provider.
//
// football-data.org: 10 requests/minute, no daily cap.
// API-Football: 100 requests/day, resets 00:00 UTC, smoothed to ~4/hour so a
// single job cannot spend the whole allocation in one burst.
const (
	FootballDataPerMinute = 10
	APIFootballPerDay     = 100
)

// APIFootball's 100 daily requests, allocated by priority.
//
// Results outrank fixtures deliberately: a missing fixture costs one day's
// tips on one league, while a missing result leaves published picks ungraded,
// which is the visible failure.
const (
	BudgetResults        = 40 // matches kicked off ≥2h ago
	BudgetFixturesNear   = 25 // next 48h
	BudgetFixturesFuture = 20 // days 3–14
	BudgetRosters        = 5  // team/competition refresh
	BudgetManualReserve  = 10 // admin backfill
)

func NewBudget(db *postgres.DB) *Budget {
	return &Budget{
		DB:        db,
		limiters:  map[string]*rate.Limiter{},
		throttled: map[string]bool{},
	}
}

func (b *Budget) limiter(provider string) *rate.Limiter {
	b.mu.Lock()
	defer b.mu.Unlock()

	if l, ok := b.limiters[provider]; ok {
		return l
	}

	var l *rate.Limiter
	switch provider {
	case ProviderFootballData:
		l = rate.NewLimiter(rate.Every(time.Minute/FootballDataPerMinute), 3)
	default:
		// Smoothed across the day rather than burst: ~4/hour.
		l = rate.NewLimiter(rate.Every(24*time.Hour/APIFootballPerDay), 2)
	}
	if b.throttled[provider] {
		l.SetLimit(l.Limit() / 2)
	}
	b.limiters[provider] = l
	return l
}

// DailyLimit is the provider's request ceiling per day, or 0 for no cap.
func DailyLimit(provider string) int {
	if provider == ProviderAPIFootball {
		return APIFootballPerDay
	}
	return 0
}

// Acquire blocks on the provider's rate limiter and reserves one request from
// today's allocation.
//
// The count is incremented in the same transaction as the job, so a crash
// cannot lose it and over-spend tomorrow's allowance.
func (b *Budget) Acquire(ctx context.Context, provider string) error {
	limit := DailyLimit(provider)
	if limit > 0 {
		used, err := b.DB.ProviderRequestsUsed(ctx, provider)
		if err != nil {
			return err
		}
		if used >= limit {
			return fmt.Errorf("%w: %s used %d of %d", ErrBudgetExhausted, provider, used, limit)
		}
	}

	if err := b.limiter(provider).Wait(ctx); err != nil {
		return fmt.Errorf("ingest: waiting for %s rate limit: %w", provider, err)
	}
	return b.DB.IncrementProviderBudget(ctx, provider)
}

// Remaining reports how many requests are left today, or -1 when uncapped.
func (b *Budget) Remaining(ctx context.Context, provider string) (int, error) {
	limit := DailyLimit(provider)
	if limit == 0 {
		return -1, nil
	}
	used, err := b.DB.ProviderRequestsUsed(ctx, provider)
	if err != nil {
		return 0, err
	}
	if used > limit {
		return 0, nil
	}
	return limit - used, nil
}

// Throttle halves a provider's local rate for the rest of the day after a 429,
// and records it so a worker restart does not undo the back-off.
func (b *Budget) Throttle(ctx context.Context, provider string) error {
	b.mu.Lock()
	b.throttled[provider] = true
	if l, ok := b.limiters[provider]; ok {
		l.SetLimit(l.Limit() / 2)
	}
	b.mu.Unlock()

	return b.DB.MarkProviderThrottled(ctx, provider)
}
