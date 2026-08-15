# Build order

Sequenced so that each phase produces something verifiable, and so the
trust-critical machinery exists before anything is published.

## Phase 0 — Foundations

- Go module, three `cmd/` binaries, `internal/` skeleton
- `config` with boot-time validation
- pgxpool, goose migrations, sqlc wiring, testcontainers harness
- `/healthz`, `/readyz`, structured logging, request ids

**Done when** `katafa migrate up` runs against a clean database and `api` serves
`/readyz`.

## Phase 1 — Schema and the invariants

Every table from [DATA-MODEL.md](DATA-MODEL.md), plus the triggers.

Write the trigger tests first: inserting a post-kickoff prediction must fail;
updating a published prediction must fail; repricing a published slip must fail.

**Done when** the invariants are enforced by the database and proven by tests
that assert failure. Nothing else in this plan is safe to build until this is
true.

## Phase 2 — Ingestion

- `Provider` interface, both clients, recorded-fixture tests
- Budget accounting, upsert/matching, `provider_payloads` archive
- `sync_competitions`, `sync_fixtures`, `sync_results` on River cron
- `katafa backfill --csv` for the football-data.co.uk seed

**Done when** a week of real fixtures and results for a handful of European
leagues is in Postgres and the daily jobs stay inside budget.

## Phase 3 — Model

- Port `src/lib/poisson.ts` and `src/lib/model.ts` to `internal/model`
- `generate_predictions` job, `match_reasoning` snapshots
- Walk-forward backtest harness with the assertion that fails on leakage

**Done when** Go and TypeScript produce identical output for the same inputs.
Port the TS test fixtures across and diff — a silent numerical divergence here
is very hard to find later.

## Phase 4 — Settlement and accuracy

- `settle/grade.go` with the exhaustive table-driven tests
- `settle_predictions`, void handling, `accuracy_rollup`
- `GET /accuracy`, `GET /predictions/settled`

**Done when** the accuracy dashboard is served from Postgres over real settled
matches.

## Phase 5 — Free tier

- `internal/tips` port of the selection logic, constants and comments intact
- `publish_free_tips` writing frozen `free_tip_days` / `free_tips`
- `GET /tips/free`, `GET /tips/free/history`
- Frontend: `getFreeTips` becomes a `fetch`; delete the selection code

**Done when** yesterday's shortlist can be shown with its results, read from
frozen rows.

## Phase 6 — Accounts and Pro reads

- argon2id, sessions, auth middleware, rate limiting
- Admin slip authoring: draft → add tips → publish, with the future-kickoff check
- `GET /slips`, `GET /slips/{id}` with the entitlement clause in SQL
- `settle_slips`, admin settlement, `analyst_rollup`

**Done when** an unpaid viewer provably receives zero tip rows — asserted at the
query level, not the response level.

## Phase 7 — MarzPay

- `pay.PaymentProvider`, MarzPay client, purchase flow
- Webhook endpoint, async processing, `payment_webhook_events`
- `reconcile_payments`, refunds
- Sandbox end-to-end, then a live low-value transaction

**Done when** a lost webhook still results in a completed purchase, proven by a
test that never delivers one.

## Phase 8 — Cutover

- Rewrite `src/api/client.ts` bodies as `fetch` calls — signatures unchanged
- Delete `src/data/` and `src/lib/session.ts`
- Remove `DemoDataBanner`
- Wire the ad gate's acknowledgement to the real network callback

**Done when** the demo banner is gone and nothing in `src/` generates data.

## Deliberately later

- Deferred markets: correct score, HT/FT, handicaps, cards, corners, scorers
- Push notifications, native apps
- Analyst self-service authoring (admin-entered until the record is established)
- Bookmaker odds ingestion — currently odds are derived from model probability,
  which is honest but not a real price

## What to watch after launch

The calibration chart, weekly. If the 70–80% confidence band does not hit near
70%, published confidence figures are misleading users and the model needs
recalibrating before more leagues are added. It is the earliest honest signal
that something is wrong, and it is visible to users at the same time it is
visible to you.
