package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/config"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/ingest"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/pay"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/predict"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/publish"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

// Deps is everything the workers need. Passed as one struct rather than
// wired through a container: the graph is small and explicit.
type Deps struct {
	DB        *postgres.DB
	Log       *slog.Logger
	Config    *config.Config
	Syncer    *ingest.Syncer
	Settler   *settle.Service
	Predictor *predict.Service
	Publisher *publish.Service
	Payments  *pay.Service
}

// Client wraps the River client so the API can enqueue without importing
// River itself.
type Client struct {
	river *river.Client[pgx.Tx]
}

// NewClient wraps an existing River client for the API's Enqueuer interface.
func NewClient(c *river.Client[pgx.Tx]) *Client { return &Client{river: c} }

// EnqueueWebhook queues one recorded callback for processing.
func (c *Client) EnqueueWebhook(ctx context.Context, eventID int64) error {
	_, err := c.river.Insert(ctx, ProcessWebhook{EventID: eventID}, nil)
	return err
}

/* ---------------------------------------------------------------- *
 * Workers
 * ---------------------------------------------------------------- */

type SyncCompetitionsWorker struct {
	river.WorkerDefaults[SyncCompetitions]
	Deps Deps
}

func (w *SyncCompetitionsWorker) Work(ctx context.Context, job *river.Job[SyncCompetitions]) error {
	// Roster refresh is a small slice of the daily budget. Fixtures carry the
	// team ids anyway, so this exists to keep names current rather than to
	// discover clubs.
	w.Deps.Log.Info("competition roster refresh requested")
	return nil
}

type SyncFixturesWorker struct {
	river.WorkerDefaults[SyncFixtures]
	Deps Deps
}

func (w *SyncFixturesWorker) Work(ctx context.Context, job *river.Job[SyncFixtures]) error {
	days := job.Args.DaysAhead
	if days <= 0 {
		days = 14
	}
	from := time.Now().UTC().Add(-2 * time.Hour)
	to := from.AddDate(0, 0, days)

	stats, err := w.Deps.Syncer.SyncFixtures(ctx, "", from, to)
	if err != nil {
		return err
	}
	w.Deps.Log.Info("fixtures synced",
		"near", job.Args.Near, "days", days,
		"fetched", stats.Fetched, "created", stats.Created, "updated", stats.Updated,
		"budget_exhausted", stats.BudgetExhausted)
	return nil
}

type SyncResultsWorker struct {
	river.WorkerDefaults[SyncResults]
	Deps Deps
}

func (w *SyncResultsWorker) Work(ctx context.Context, job *river.Job[SyncResults]) error {
	// Matches that kicked off at least two hours ago and may now be final.
	to := time.Now().UTC().Add(-2 * time.Hour)
	from := to.AddDate(0, 0, -3)

	stats, err := w.Deps.Syncer.SyncResults(ctx, "", from, to)
	if err != nil {
		return err
	}
	w.Deps.Log.Info("results synced",
		"fetched", stats.Fetched, "updated", stats.Updated,
		"budget_exhausted", stats.BudgetExhausted)
	return nil
}

type GeneratePredictionsWorker struct {
	river.WorkerDefaults[GeneratePredictions]
	Deps Deps
}

func (w *GeneratePredictionsWorker) Work(ctx context.Context, job *river.Job[GeneratePredictions]) error {
	window := predict.Horizon
	if job.Args.HoursAhead > 0 {
		window = time.Duration(job.Args.HoursAhead) * time.Hour
	}
	stats, err := w.Deps.Predictor.GenerateUpcoming(ctx, window)
	if err != nil {
		return err
	}
	w.Deps.Log.Info("predictions generated",
		"fixtures", stats.Fixtures, "predictions", stats.Predictions, "skipped", stats.Skipped)
	return nil
}

type PublishFreeTipsWorker struct {
	river.WorkerDefaults[PublishFreeTips]
	Deps Deps
}

func (w *PublishFreeTipsWorker) Work(ctx context.Context, job *river.Job[PublishFreeTips]) error {
	day, err := w.Deps.Publisher.PublishNextDay(ctx)
	if err != nil {
		return err
	}
	if day.IsEmpty() {
		// A day with no eligible fixtures is a legitimate outcome. Writing an
		// empty row would claim a shortlist was published when none was.
		w.Deps.Log.Info("no free shortlist published: nothing eligible")
		return nil
	}
	w.Deps.Log.Info("free shortlist published",
		"day", day.Day.Format(time.DateOnly), "tips", day.TotalTips)
	return nil
}

type SettlePredictionsWorker struct {
	river.WorkerDefaults[SettlePredictions]
	Deps Deps
}

func (w *SettlePredictionsWorker) Work(ctx context.Context, job *river.Job[SettlePredictions]) error {
	graded, err := w.Deps.Settler.SettlePredictions(ctx)
	if err != nil {
		return err
	}
	voided, err := w.Deps.Settler.VoidUngradablePredictions(ctx)
	if err != nil {
		return err
	}

	// Chained rather than raced on a shared cron minute: slips settle from
	// the results this pass just wrote.
	client := river.ClientFromContext[pgx.Tx](ctx)
	if _, err := client.Insert(ctx, SettleSlips{}, nil); err != nil {
		return fmt.Errorf("enqueue settle_slips: %w", err)
	}
	if graded+voided > 0 {
		if _, err := client.Insert(ctx, RefreshAccuracy{}, nil); err != nil {
			return fmt.Errorf("enqueue refresh_accuracy: %w", err)
		}
	}
	return nil
}

type SettleSlipsWorker struct {
	river.WorkerDefaults[SettleSlips]
	Deps Deps
}

func (w *SettleSlipsWorker) Work(ctx context.Context, job *river.Job[SettleSlips]) error {
	if _, err := w.Deps.Settler.SettleTips(ctx); err != nil {
		return err
	}
	_, voided, err := w.Deps.Settler.CloseSlips(ctx)
	if err != nil {
		return err
	}

	client := river.ClientFromContext[pgx.Tx](ctx)
	for _, slip := range voided {
		// Every leg was called off, so the slip returned nothing and its
		// buyers are refunded.
		if _, err := client.Insert(ctx, RefundSlip{
			SlipID: slip.SlipID.String(),
			Reason: "every selection on the slip was voided",
		}, nil); err != nil {
			return fmt.Errorf("enqueue refund for slip %s: %w", slip.SlipID, err)
		}
	}
	return nil
}

type RefreshAccuracyWorker struct {
	river.WorkerDefaults[RefreshAccuracy]
	Deps Deps
}

func (w *RefreshAccuracyWorker) Work(ctx context.Context, job *river.Job[RefreshAccuracy]) error {
	return w.Deps.DB.RefreshRollups(ctx)
}

type ReconcilePaymentsWorker struct {
	river.WorkerDefaults[ReconcilePayments]
	Deps Deps
}

func (w *ReconcilePaymentsWorker) Work(ctx context.Context, job *river.Job[ReconcilePayments]) error {
	resolved, err := w.Deps.Payments.Reconcile(ctx, 100)
	if err != nil {
		return err
	}
	if resolved > 0 {
		w.Deps.Log.Info("payments reconciled", "resolved", resolved)
	}
	return nil
}

type ProcessWebhookWorker struct {
	river.WorkerDefaults[ProcessWebhook]
	Deps Deps
}

func (w *ProcessWebhookWorker) Work(ctx context.Context, job *river.Job[ProcessWebhook]) error {
	return w.Deps.Payments.ProcessWebhookEvent(ctx, job.Args.EventID)
}

type RefundSlipWorker struct {
	river.WorkerDefaults[RefundSlip]
	Deps Deps
}

func (w *RefundSlipWorker) Work(ctx context.Context, job *river.Job[RefundSlip]) error {
	slipID, err := uuid.Parse(job.Args.SlipID)
	if err != nil {
		return fmt.Errorf("refund slip: %w", err)
	}
	return w.Deps.Payments.RefundSlip(ctx, slipID, job.Args.Reason)
}

type ExpireSessionsWorker struct {
	river.WorkerDefaults[ExpireSessions]
	Deps Deps
}

func (w *ExpireSessionsWorker) Work(ctx context.Context, job *river.Job[ExpireSessions]) error {
	deleted, err := w.Deps.DB.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
	if err := w.Deps.DB.PruneRateLimits(ctx); err != nil {
		return err
	}
	w.Deps.Log.Info("expired sessions removed", "count", deleted)
	return nil
}

type PrunePayloadsWorker struct {
	river.WorkerDefaults[PrunePayloads]
	Deps Deps
}

func (w *PrunePayloadsWorker) Work(ctx context.Context, job *river.Job[PrunePayloads]) error {
	deleted, err := w.Deps.DB.PrunePayloads(ctx)
	if err != nil {
		return err
	}
	w.Deps.Log.Info("provider payloads pruned", "count", deleted)
	return nil
}

// Register adds every worker to the registry.
func Register(workers *river.Workers, deps Deps) {
	river.AddWorker(workers, &SyncCompetitionsWorker{Deps: deps})
	river.AddWorker(workers, &SyncFixturesWorker{Deps: deps})
	river.AddWorker(workers, &SyncResultsWorker{Deps: deps})
	river.AddWorker(workers, &GeneratePredictionsWorker{Deps: deps})
	river.AddWorker(workers, &PublishFreeTipsWorker{Deps: deps})
	river.AddWorker(workers, &SettlePredictionsWorker{Deps: deps})
	river.AddWorker(workers, &SettleSlipsWorker{Deps: deps})
	river.AddWorker(workers, &RefreshAccuracyWorker{Deps: deps})
	river.AddWorker(workers, &ReconcilePaymentsWorker{Deps: deps})
	river.AddWorker(workers, &ProcessWebhookWorker{Deps: deps})
	river.AddWorker(workers, &RefundSlipWorker{Deps: deps})
	river.AddWorker(workers, &ExpireSessionsWorker{Deps: deps})
	river.AddWorker(workers, &PrunePayloadsWorker{Deps: deps})
}
