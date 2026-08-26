package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/jobs"
)

// runOnce runs every worker job directly, once, without River — no queue,
// no persistent process. It exists so the worker can be driven by an
// external scheduler (a GitHub Actions cron, in this deployment) instead of
// an always-on machine, which no free hosting tier actually offers.
//
// This only works because every job below is already idempotent by design —
// re-running it is a no-op or a safe no-op-equivalent, documented at each
// call site in internal/settle, internal/publish and internal/tips. The hour
// gates that remain exist only to avoid pointless provider calls and DB
// churn, never for correctness: a tick that fires late, or twice, produces
// the same result as one that fires on time.
//
// The one place order matters — predictions must exist before the free
// shortlist freezes — is handled by generating on every tick rather than
// once at 04:00. That is deliberately more eager than the original River
// schedule (PeriodicJobs in internal/jobs/jobs.go), and removes the
// dependency on the 04:00 batch having finished before 05:00 arrives.
func runOnce(ctx context.Context, deps jobs.Deps, log *slog.Logger) error {
	now := time.Now().UTC()
	// Every stage runs even if an earlier one failed: a provider outage
	// must not take settlement, the free-tips freeze or payment
	// reconciliation down with it. All collected errors are returned
	// together so the caller still sees a non-zero exit.
	var errs []error

	// Near fixture window: catches reschedules, runs every tick.
	if stats, err := deps.Syncer.SyncFixtures(ctx, "", now.Add(-2*time.Hour), now.AddDate(0, 0, 2)); err != nil {
		log.Error("sync fixtures (near) failed", "err", err)
		errs = append(errs, fmt.Errorf("sync fixtures (near): %w", err))
	} else {
		log.Info("fixtures synced", "near", true,
			"fetched", stats.Fetched, "created", stats.Created, "updated", stats.Updated)
	}

	// Far fixture window (14 days): provider-budget-sensitive, keeps its
	// original once-a-day cadence.
	if now.Hour() == 3 {
		if stats, err := deps.Syncer.SyncFixtures(ctx, "", now.Add(-2*time.Hour), now.AddDate(0, 0, 14)); err != nil {
			log.Error("sync fixtures (far) failed", "err", err)
			errs = append(errs, fmt.Errorf("sync fixtures (far): %w", err))
		} else {
			log.Info("fixtures synced", "near", false,
				"fetched", stats.Fetched, "created", stats.Created, "updated", stats.Updated)
		}
	}

	// Finals for matches that kicked off ≥2h ago.
	if stats, err := deps.Syncer.SyncResults(ctx, "", now.AddDate(0, 0, -3), now.Add(-2*time.Hour)); err != nil {
		log.Error("sync results failed", "err", err)
		errs = append(errs, fmt.Errorf("sync results: %w", err))
	} else {
		log.Info("results synced", "fetched", stats.Fetched, "updated", stats.Updated)
	}

	// Idempotent: skips fixtures that already have a prediction at the
	// current model version. Safe, and deliberately more frequent than the
	// original daily-04:00 run.
	if stats, err := deps.Predictor.GenerateUpcoming(ctx, 48*time.Hour); err != nil {
		log.Error("generate predictions failed", "err", err)
		errs = append(errs, fmt.Errorf("generate predictions: %w", err))
	} else if stats.Predictions > 0 {
		log.Info("predictions generated", "fixtures", stats.Fixtures, "predictions", stats.Predictions)
	}

	// PublishNextDay is documented idempotent: a day already published is
	// left exactly as it was. Firing every tick past 05:00 UTC is
	// self-healing against one missed or delayed run.
	if now.Hour() >= 5 {
		day, err := deps.Publisher.PublishNextDay(ctx)
		if err != nil {
			log.Error("publish free tips failed", "err", err)
			errs = append(errs, fmt.Errorf("publish free tips: %w", err))
		} else if !day.IsEmpty() {
			log.Info("free shortlist published", "day", day.Day.Format(time.DateOnly), "tips", day.TotalTips)
		}
	}

	// Settlement chain — sequential instead of enqueue-on-completion, since
	// there is no River client here to enqueue into.
	graded, err := deps.Settler.SettlePredictions(ctx)
	if err != nil {
		log.Error("settle predictions failed", "err", err)
		errs = append(errs, fmt.Errorf("settle predictions: %w", err))
	}
	voided, err := deps.Settler.VoidUngradablePredictions(ctx)
	if err != nil {
		log.Error("void ungradable predictions failed", "err", err)
		errs = append(errs, fmt.Errorf("void ungradable predictions: %w", err))
	}
	if _, err := deps.Settler.SettleTips(ctx); err != nil {
		log.Error("settle tips failed", "err", err)
		errs = append(errs, fmt.Errorf("settle tips: %w", err))
	}
	_, voidedSlips, err := deps.Settler.CloseSlips(ctx)
	if err != nil {
		log.Error("close slips failed", "err", err)
		errs = append(errs, fmt.Errorf("close slips: %w", err))
	} else {
		for _, slip := range voidedSlips {
			// Every leg on the slip was called off, so its buyers are refunded.
			if err := deps.Payments.RefundSlip(ctx, slip.SlipID, "every selection on the slip was voided"); err != nil {
				log.Error("refund slip failed", "slip_id", slip.SlipID, "err", err)
				errs = append(errs, fmt.Errorf("refund slip %s: %w", slip.SlipID, err))
			}
		}
	}
	if graded+voided > 0 {
		if err := deps.DB.RefreshRollups(ctx); err != nil {
			log.Error("refresh rollups failed", "err", err)
			errs = append(errs, fmt.Errorf("refresh rollups: %w", err))
		}
	}

	// Poll transactions stuck in flight — the guarantee that makes a lost
	// webhook still complete the purchase.
	if resolved, err := deps.Payments.Reconcile(ctx, 100); err != nil {
		log.Error("reconcile payments failed", "err", err)
		errs = append(errs, fmt.Errorf("reconcile payments: %w", err))
	} else if resolved > 0 {
		log.Info("payments reconciled", "resolved", resolved)
	}

	// Daily housekeeping, once past 01:00 UTC.
	if now.Hour() == 1 {
		if deleted, err := deps.DB.DeleteExpiredSessions(ctx); err != nil {
			log.Error("expire sessions failed", "err", err)
			errs = append(errs, fmt.Errorf("expire sessions: %w", err))
		} else if deleted > 0 {
			log.Info("expired sessions removed", "count", deleted)
		}
		if err := deps.DB.PruneRateLimits(ctx); err != nil {
			log.Error("prune rate limits failed", "err", err)
			errs = append(errs, fmt.Errorf("prune rate limits: %w", err))
		}
		if deleted, err := deps.DB.PrunePayloads(ctx); err != nil {
			log.Error("prune payloads failed", "err", err)
			errs = append(errs, fmt.Errorf("prune payloads: %w", err))
		} else if deleted > 0 {
			log.Info("provider payloads pruned", "count", deleted)
		}
	}

	return errors.Join(errs...)
}
