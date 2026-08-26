// Package jobs defines the River job types and the cron schedule.
//
// River runs on Postgres with SELECT … FOR UPDATE SKIP LOCKED and has cron
// built in. One datastore to back up, one to restore, and jobs commit in the
// same transaction as the rows they produce — which is what makes settlement
// exactly-once rather than approximately-once.
package jobs

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/predict"
)

// Job argument types. Kind() is the stable name stored in the queue, so
// renaming one orphans in-flight jobs.

type SyncCompetitions struct{}

func (SyncCompetitions) Kind() string { return "sync_competitions" }

// SyncFixtures pulls a window of upcoming fixtures.
type SyncFixtures struct {
	// DaysAhead is how far forward to pull. The daily run takes 14 days; the
	// hourly near-window run takes 2.
	DaysAhead int `json:"days_ahead"`
	// Near marks the hourly re-pull, which exists to catch reschedules.
	Near bool `json:"near"`
}

func (SyncFixtures) Kind() string { return "sync_fixtures" }

type SyncResults struct{}

func (SyncResults) Kind() string { return "sync_results" }

// GeneratePredictions runs the engine for fixtures kicking off soon that have
// no prediction at the current model version.
type GeneratePredictions struct {
	HoursAhead int `json:"hours_ahead"`
}

func (GeneratePredictions) Kind() string { return "generate_predictions" }

// PublishFreeTips selects and freezes the day's free shortlist.
type PublishFreeTips struct {
	// Day is the matchday to publish, empty for the next one with fixtures.
	Day string `json:"day,omitempty"`
}

func (PublishFreeTips) Kind() string { return "publish_free_tips" }

type SettlePredictions struct{}

func (SettlePredictions) Kind() string { return "settle_predictions" }

type SettleSlips struct{}

func (SettleSlips) Kind() string { return "settle_slips" }

type RefreshAccuracy struct{}

func (RefreshAccuracy) Kind() string { return "refresh_accuracy" }

type ReconcilePayments struct{}

func (ReconcilePayments) Kind() string { return "reconcile_payments" }

// ProcessWebhook applies one recorded payment callback.
type ProcessWebhook struct {
	EventID int64 `json:"event_id"`
}

func (ProcessWebhook) Kind() string { return "process_webhook" }

// RefundSlip refunds every paid purchase of a slip whose legs all voided.
type RefundSlip struct {
	SlipID string `json:"slip_id"`
	Reason string `json:"reason"`
}

func (RefundSlip) Kind() string { return "refund_slip" }

type ExpireSessions struct{}

func (ExpireSessions) Kind() string { return "expire_sessions" }

type PrunePayloads struct{}

func (PrunePayloads) Kind() string { return "prune_payloads" }

// dailyAt is a cron schedule that fires once a day at a fixed UTC time.
//
// All schedules are UTC. A local-time schedule would move the publish window
// twice a year, and the free shortlist's day boundary is UTC.
type dailyAt struct {
	hour   int
	minute int
}

func (d dailyAt) Next(t time.Time) time.Time {
	t = t.UTC()
	next := time.Date(t.Year(), t.Month(), t.Day(), d.hour, d.minute, 0, 0, time.UTC)
	if !next.After(t) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// weeklyAt fires once a week on the given weekday at a fixed UTC time.
type weeklyAt struct {
	weekday time.Weekday
	hour    int
	minute  int
}

func (w weeklyAt) Next(t time.Time) time.Time {
	t = t.UTC()
	next := time.Date(t.Year(), t.Month(), t.Day(), w.hour, w.minute, 0, 0, time.UTC)
	for next.Weekday() != w.weekday || !next.After(t) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// PeriodicJobs is the cron schedule, all times UTC.
//
// Ordering is not left to luck: settle_predictions enqueues settle_slips on
// completion, and settlement enqueues the rollup refresh, rather than all
// three racing on a shared cron minute.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		// Weekly, Monday 02:00 — refresh league and team rosters.
		river.NewPeriodicJob(weeklyAt{time.Monday, 2, 0},
			func() (river.JobArgs, *river.InsertOpts) { return SyncCompetitions{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false}),

		// Daily 03:00 — the next 14 days of fixtures for every active league.
		river.NewPeriodicJob(dailyAt{3, 0},
			func() (river.JobArgs, *river.InsertOpts) { return SyncFixtures{DaysAhead: 14}, nil },
			&river.PeriodicJobOpts{RunOnStart: false}),

		// Hourly — re-pull the next 36 hours to catch reschedules.
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return SyncFixtures{DaysAhead: 2, Near: true}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true}),

		// Every 30 minutes — finals for matches that kicked off ≥2h ago.
		river.NewPeriodicJob(river.PeriodicInterval(30*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return SyncResults{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),

		// Daily 04:00 — price every fixture inside predict.Horizon. A fixture
		// with no prediction is invisible to the feed, so this is what decides
		// how much football the site shows.
		river.NewPeriodicJob(dailyAt{4, 0},
			func() (river.JobArgs, *river.InsertOpts) {
				return GeneratePredictions{HoursAhead: int(predict.Horizon / time.Hour)}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false}),

		// Daily 05:00 — select and freeze the day's free shortlist. It runs
		// after generation so the picks it selects from already exist.
		river.NewPeriodicJob(dailyAt{5, 0},
			func() (river.JobArgs, *river.InsertOpts) { return PublishFreeTips{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false}),

		// Every 30 minutes, after sync_results.
		river.NewPeriodicJob(river.PeriodicInterval(30*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return SettlePredictions{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),

		// Every 15 minutes — poll transactions stuck in flight. This, not the
		// webhook, is what guarantees users get what they paid for.
		river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return ReconcilePayments{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),

		// Daily 01:00 — drop expired sessions.
		river.NewPeriodicJob(dailyAt{1, 0},
			func() (river.JobArgs, *river.InsertOpts) { return ExpireSessions{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false}),

		// Daily 01:30 — 180-day retention on the provider payload archive.
		river.NewPeriodicJob(dailyAt{1, 30},
			func() (river.JobArgs, *river.InsertOpts) { return PrunePayloads{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false}),
	}
}
