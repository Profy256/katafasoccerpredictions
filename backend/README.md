# Katafa backend

Go + PostgreSQL. No Redis, no Python, no message broker.

Design lives in [`docs/backend/`](../docs/backend/README.md) — read the relevant
document before changing code here. The schema and the settlement rules are not
obvious from the frontend types alone, and the constraints *are* the safety
model.

## Layout

```
cmd/api        HTTP server. Stateless, scale horizontally.
cmd/worker     River job runner: ingestion, prediction, settlement, reconciliation.
cmd/katafa     Admin CLI: migrations, seeding, backfills, manual settlement.

internal/
  domain/      core types; imports nothing from the other internal packages
  config/      env → typed Config, validated at boot
  postgres/    pgxpool, transactions, and every SQL statement
  ingest/      Provider interface, both clients, budget accounting, upsert
  model/       Poisson engine — poisson/ matrix, strength/ attack+defence
  settle/      pure grading functions, prediction and slip settlement
  tips/        free shortlist selection
  predict/     runs the model over upcoming fixtures
  publish/     freezes the daily shortlist
  pay/         PaymentProvider interface, marzpay/ client, purchase flow
  auth/        argon2id, opaque session tokens
  api/         router, handlers, render/ (ETag, problem+json)
  jobs/        River job definitions and cron registration
migrations/    goose SQL, embedded in the katafa binary
tools/parity/  generates the TypeScript model fixtures the Go port is diffed against
```

`domain` imports nothing from the other internal packages. Everything else may
import `domain`. That one rule keeps grading testable without a database and
stops the schema leaking into the model.

## Running it

```bash
docker run --rm -d --name katafa-pg -p 5433:5432 \
  -e POSTGRES_USER=katafa -e POSTGRES_PASSWORD=katafa -e POSTGRES_DB=katafa \
  postgres:16

export DATABASE_URL='postgres://katafa:katafa@localhost:5433/katafa?sslmode=disable'
export MODEL_VERSION='poisson-1.2.0'
export PUBLIC_BASE_URL='http://localhost:8080'
export ALLOWED_ORIGINS='http://localhost:3000'

go run ./cmd/katafa migrate up     # domain schema
go run ./cmd/katafa jobs migrate   # River's own tables
go run ./cmd/api                   # /healthz, /readyz, /v1/…
go run ./cmd/worker                # jobs and cron
```

Provider and MarzPay credentials are only required when `ENV=production`.
Without them the worker skips ingestion and the fake payment provider is used,
so a development machine never needs live secrets — which is what stops real
ones being pasted into a `.env` that then gets committed.

### Seeding a league

Leagues are created **unpublished**. A league gets published tips only once its
teams have enough history for the strength estimates to mean anything; until
then it runs in shadow mode, predicted and settled internally.

```bash
go run ./cmd/katafa seed leagues \
  -slug=eng-premier-league -name="Premier League" -short-name=EPL \
  -country=England -country-code=ENG -region=europe \
  -provider=football-data -provider-id=2021

# Seed history from football-data.co.uk, which has no rate limit.
go run ./cmd/katafa backfill -csv=E0.csv -league=eng-premier-league

# Then publish it deliberately:
#   UPDATE leagues SET is_published = TRUE WHERE slug = 'eng-premier-league';
```

## Tests

```bash
go test ./...                              # pure tests only
export TEST_DATABASE_URL='postgres://katafa:katafa@localhost:5433/katafa?sslmode=disable'
go test ./...                              # adds the database tests
```

Tests that need Postgres **skip** rather than fail when `TEST_DATABASE_URL` is
unset, so the pure suite runs anywhere. Each gets its own database.

The ones that matter most, in order:

| Test | Why |
|---|---|
| `internal/postgres/invariants_test.go` | Asserts the database *refuses* things: a post-kickoff prediction, an update to a published row, a repriced slip, a reopened settlement, a second paid entitlement. These are failure assertions on purpose — a happy-path test would still pass with every trigger dropped. |
| `internal/settle/grade_test.go` | Exhaustive over every market × every scoreline. A bug here silently rewrites the published track record and nothing crashes. |
| `internal/model/parity_test.go` | Diffs the Go model against output generated from the TypeScript itself. A silent numerical divergence is very hard to find later. |
| `internal/model/engine_test.go` | Walk-forward: predicting from history that includes the target match must fail. |

## Deviations from the design documents

Recorded because a reviewer should see them without reading a diff.

- **Hand-written pgx instead of sqlc.** ARCHITECTURE.md specifies sqlc. Much of
  the query surface is dynamically filtered — `/feed` has four optional
  filters, `/predictions/settled` and `/slips` add cursor pagination — which
  sqlc handles badly enough that those queries end up hand-written anyway. The
  SQL still lives as reviewable statements, one file per aggregate. The schema
  remains sqlc-ready and nothing outside `internal/postgres` depends on the
  choice.
- **Double Chance stores `*_SIDE` outcomes.** SETTLEMENT.md grades DC to
  `HOME`/`DRAW`/`AWAY`; the frontend's `settledOutcomeLabel` already renders
  `HOME_SIDE`/`DRAW_SIDE`/`AWAY_SIDE` and would mislabel a plain `HOME` as
  "Away Win". `settle.Grade` follows the document's logic and stores the form
  the frontend reads. Correctness is decided by membership either way.
- **Four tables beyond DATA-MODEL.md**: `prediction_voids` (a void has no
  result row, so without this it is indistinguishable from pending and
  settlement re-examines cancelled matches forever), `correction_reviews`
  (API.md requires flagging affected predictions without re-grading),
  `refunds`, and `rate_limits` (API.md wants limiting in Postgres, not memory).
  Plus `slips.settled_odds`, `leagues.is_published`, and
  `provider_budget.throttled_at`.
- **Test harness uses `TEST_DATABASE_URL`, not testcontainers.** Same
  guarantee — real Postgres, fresh database per test — without the dependency
  tree. CI supplies a service container.
