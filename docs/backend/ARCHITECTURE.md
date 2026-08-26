# Architecture

## Binaries

Three, from one module. They share `internal/` and differ only in what they start.

```
cmd/api      HTTP server. Stateless. Scale horizontally.
cmd/worker   River job runner: ingestion, prediction, settlement, payment reconciliation.
cmd/katafa   Admin CLI: migrations, seeding, backfills, manual settlement, key rotation.
```

`api` and `worker` are separate processes because their failure modes are
unrelated. A provider outage stalls the worker; the API keeps serving yesterday's
published tips, which are already in Postgres and do not need the provider to be
reachable. Deploying them together would couple an ingestion crash-loop to the
public site.

## Package layout

```
backend/
  cmd/{api,worker,katafa}/main.go
  internal/
    config/        env → typed Config, validated at boot
    postgres/      pgxpool setup, txmanager, sqlc-generated queries
    domain/        core types: Match, Prediction, Slip, Tip, Money, Odds
    ingest/
      provider.go  the Provider interface
      footballdata/
      apifootball/
      sync.go      upsert + reconciliation logic shared by all providers
      budget.go    per-provider daily request accounting
    model/
      poisson/     scoreline matrix, market derivation
      strength/    venue-split recency-weighted attack/defence
      engine.go    Engine interface + Go implementation
    settle/
      grade.go     pure per-market grading functions
      predictions.go
      slips.go
    tips/          free shortlist selection + freezing
    pay/
      provider.go  PaymentProvider interface
      marzpay/
      entitlement.go
    auth/          argon2id, opaque session tokens, middleware
    api/
      router.go    net/http ServeMux + middleware chain
      handlers/
      render/      response encoding, ETag, problem+json errors
    jobs/          River job definitions and cron registration
    audit/         append-only audit_log writer
  migrations/      goose SQL migrations
  queries/         sqlc .sql sources
```

`domain` imports nothing from the other internal packages. Everything else may
import `domain`. That single rule keeps the grading functions testable without a
database and stops the schema from leaking into the model.

## Libraries

| Concern | Choice | Why this one |
|---|---|---|
| Postgres driver | `jackc/pgx/v5` + `pgxpool` | Native protocol, real `NUMERIC` and `TIMESTAMPTZ` support, no `database/sql` conversions to get wrong |
| Queries | `sqlc` | Generates typed Go from plain SQL. The schema stays readable and reviewable, which matters when the constraints *are* the safety model |
| Migrations | `pressly/goose` | Plain SQL up/down, embeddable in the `katafa` binary |
| Jobs + cron | `riverqueue/river` | Postgres-backed, transactional enqueue, cron built in. No Redis |
| Decimals | `shopspring/decimal` | Odds arithmetic. Accumulator odds multiply, and float drift there is visible to users |
| HTTP router | stdlib `net/http` | Go's `ServeMux` handles `GET /v1/slips/{id}` patterns. A router dependency earns nothing here |
| Logging | `log/slog` | JSON to stdout, request id in context |
| Rate limiting | `golang.org/x/time/rate` | Per-provider token buckets in the worker |
| Password hashing | `golang.org/x/crypto/argon2` | argon2id |

Deliberately absent: an ORM, a DI framework, and a config library. Handlers take
a struct of dependencies; `config` reads env vars into a struct and validates
them at boot.

## Request flow

```
Next.js server component
  └─> GET /v1/… with session cookie
        └─> middleware: request id → logging → recovery → rate limit → session
              └─> handler
                    ├─> sqlc query (entitlement folded into the WHERE clause)
                    └─> render: ETag, Cache-Control, problem+json on error
```

The handler layer holds no business logic. Grading lives in `settle`,
selection in `tips`, entitlement in `pay`. Handlers translate HTTP to a call and
back, so the rules stay unit-testable without spinning up a server.

## Job flow

River cron schedules, all times UTC:

| Job | Schedule | Does |
|---|---|---|
| `sync_competitions` | weekly, Mon 02:00 | Refresh league/team rosters per provider |
| `sync_fixtures` | daily 03:00 | Pull the next 14 days of fixtures for every active competition |
| `sync_fixtures_near` | hourly | Re-pull fixtures kicking off in the next 36h to catch reschedules |
| `sync_results` | every 30 min | Pull finals for matches whose kickoff was ≥ 2h ago and are still unfinished |
| `generate_predictions` | daily 04:00 | Run the engine for every fixture inside `predict.Horizon` (7 days) that has no prediction |
| `publish_free_tips` | daily 05:00 | Select and **freeze** the day's free shortlist |
| `settle_predictions` | every 30 min, after `sync_results` | Grade newly finished matches |
| `settle_slips` | every 30 min, after `settle_predictions` | Grade auto-gradable tips, close fully-graded slips |
| `refresh_accuracy` | after any settlement batch | `REFRESH MATERIALIZED VIEW CONCURRENTLY accuracy_rollup` |
| `reconcile_payments` | every 15 min | Poll MarzPay for transactions stuck in `processing` |
| `expire_sessions` | daily 01:00 | Delete expired session rows |

`predict.Horizon` is the number that decides how much football the site shows.
A fixture with no prediction is dropped by the feed, so anything ingested past
the horizon is invisible — no feed row, no team or league page entry, no match
URL in the sitemap, nothing for the free shortlist to select from. It ran at
48 hours until a midweek day left the entire site showing one fixture.

It is not simply maximised: predictions are immutable and a fixture is priced
once per model version, so a pick made seven days out is locked in on
seven-day-old form and enters the accuracy record that way. Seven days keeps
the coming weekend visible from midweek without pricing a fortnight on stale
form. It must stay at least `tips.MaxWindowDays` long or the shortlist window
reaches days with nothing on them — `TestHorizonCoversShortlistWindow` asserts
it.

Ordering matters and is not left to luck: `settle_predictions` enqueues
`settle_slips` on completion rather than both racing on a shared cron minute.

## Caching without Redis

Two mechanisms cover it:

- **Materialised views** for anything aggregating the full settled history.
  `accuracy_rollup` and `analyst_rollup` are refreshed after settlement, not per
  request. Computing a hit rate over every settled prediction on each page view
  is the one query guaranteed to get slower every day the product succeeds.
- **HTTP caching** on read endpoints. Published data is immutable, so `ETag` +
  `Cache-Control: public, max-age=…` is safe and lets the Next.js data cache and
  any CDN do the work. Authenticated slip responses are `private, no-store`.

## Configuration

All via environment, validated at boot — the process refuses to start on a
missing or malformed value rather than failing at 3am on the first webhook.

```
DATABASE_URL
PORT
ENV                        development | staging | production
SESSION_COOKIE_DOMAIN
FOOTBALL_DATA_TOKEN
APIFOOTBALL_KEY
MARZPAY_API_USER
MARZPAY_API_KEY
MARZPAY_WEBHOOK_SECRET
MARZPAY_BASE_URL           default https://wallet.wearemarz.com/api/v1
PUBLIC_BASE_URL            used to build callback_url
MODEL_VERSION              stamped onto every prediction
```

## Testing

- **Grading is table-driven and pure.** `settle/grade_test.go` enumerates every
  market × every scoreline shape. This is the highest-value test file in the
  repo; a bug here silently rewrites the track record.
- **Repository tests run against real Postgres** via testcontainers. The
  invariants are enforced by triggers and constraints, so a mocked database
  would test nothing that matters — the tests must assert that inserting a
  post-kickoff prediction *fails*.
- **Provider clients test against recorded fixtures**, not the live API. Free
  tiers have daily budgets that a test suite would burn through by lunchtime.
- **Walk-forward assertion in backtests.** A test that fails if the model reads
  any match with `kickoff_at >= target.kickoff_at`.
