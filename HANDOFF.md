# Handoff — Katafa backend build

Working notes for whoever (or whichever session) picks this up next. Update it
as you go; it is the only place that records *why* something is the way it is.

**Read first:** [`AGENTS.md`](AGENTS.md) non-negotiables, then
[`docs/backend/README.md`](docs/backend/README.md) and the documents it lists in
order. This file records progress against
[`docs/backend/ROADMAP.md`](docs/backend/ROADMAP.md) — it does not replace it.
Operational detail (how to run it, how to seed a league) is in
[`backend/README.md`](backend/README.md).

---

## Where the build is

"Done" means written **and** verified by something that runs. Anything weaker is
called out.

| Phase | State |
|---|---|
| 0 — Foundations | **done** — migrations apply to a clean DB, `api` serves `/readyz` |
| 1 — Schema + invariants | **done** — every table and trigger, proven by failure tests |
| 2 — Ingestion | **code complete, not exercised against a live provider** |
| 3 — Model | **done** — Go/TypeScript parity verified to 1e-12 |
| 4 — Settlement + accuracy | **done** — grading, voids, rollups, endpoints |
| 5 — Free tier | **done** — selection, freeze, both endpoints |
| 6 — Accounts + Pro reads | **code complete** — entitlement is in SQL; needs its own test |
| 7 — MarzPay | **code complete, sandbox-untested** |
| 8 — Frontend cutover | **not started** (deliberately last) |

Verified in this build: `go build ./...` and `go vet ./...` clean; `go test ./...`
green including the database suite; a fresh database migrated, River tables
installed, a league seeded, and the API serving `/healthz`, `/readyz`,
`/v1/leagues`, `/v1/markets`, `/v1/packages`, `/v1/tips/free`, `/v1/accuracy`,
`/v1/stats/coverage`, plus the correct 400 / 404 / 401 on bad input, an unknown
slip, and an unauthenticated `/v1/auth/me`.

~13,000 lines of Go across 66 files, 11 migrations.

### What is NOT done — read this before trusting anything

1. **No provider has ever been called.** Both clients are written against the
   documented response shapes and are unit-untested. ROADMAP Phase 2 wants
   recorded-fixture tests; there are none. Expect the first live sync to
   surface field-shape surprises. `provider_payloads` archives every response
   before parsing precisely so this is debuggable without burning budget.
2. **MarzPay has never been called.** The webhook signature scheme is
   *assumed* to be HMAC-SHA256 over the raw body, read from
   `X-Marzpay-Signature` with `X-Signature` / `Signature` fallbacks. **Verify
   this against the live dashboard before going near real money.** If it is
   wrong, every callback fails verification, is recorded with
   `signature_valid = false`, and is never acted on — purchases would then
   complete only via `reconcile_payments`, silently and slowly.
3. **The tests ROADMAP names for phases 6 and 7 do not exist yet**: the
   entitlement test that asserts an unpaid viewer receives *zero tip rows from
   the database*, and the reconciliation test where the webhook is never
   delivered. The code is written to satisfy both; nothing proves it.
4. **`sync_competitions` is a stub.** It logs and returns. Fixture responses
   carry team ids, so teams are created on demand by `TeamIDBySource`; the
   roster job only ever existed to keep names fresh.
5. **No CI.** The commands are in `backend/README.md`.

### Suggested order from here

1. Recorded-fixture tests for both provider clients (Phase 2's "done when").
   Capture real responses into `testdata/` first — one free call each.
2. The entitlement test. It is the shortest test that protects revenue.
3. MarzPay sandbox end-to-end, then a live low-value transaction.
4. Phase 8 cutover: rewrite `src/api/client.ts` bodies as `fetch`, delete
   `src/data/` and `src/lib/session.ts`, remove `DemoDataBanner`.

---

## Decisions taken during the build

Things a fresh session would otherwise re-litigate or get wrong.

### Module path is the repo path

`github.com/Profy256/katafasoccerpredictions/backend`. The Go code is a
subdirectory module of the repo, so this is what `go get` resolves. Do not
rename it back to a short placeholder.

### Data access is hand-written pgx, not sqlc

ARCHITECTURE.md specifies sqlc. Deviating, because a large share of the query
surface is dynamically filtered — `/feed` has four optional filters,
`/predictions/settled` and `/slips` have optional filters *plus* cursor
pagination — and sqlc handles dynamic `WHERE` badly enough that those queries
end up hand-written anyway. SQL still lives as reviewable statements in
`internal/postgres`, one file per aggregate.

**This is reversible** and the schema is sqlc-ready. Nothing outside
`internal/postgres` depends on the choice.

### Double Chance settles to a *side*, not to a selection

SETTLEMENT.md's table says DC grades to `HOME`/`DRAW`/`AWAY`, with wins decided
by membership. The frontend's `settledOutcomeLabel` in `src/lib/model.ts`
already renders `HOME_SIDE` / `DRAW_SIDE` / `AWAY_SIDE`, and would label a plain
`HOME` as **"Away Win"** — a real bug at cutover, not a cosmetic one.

`settle.Grade` follows the document's logic; the value **stored** in
`actual_outcome` for DC is the `*_SIDE` form. Correctness is membership either
way, and `settle.Matches` refuses a 1X2 selection on a DC market.

### A contradiction in the design documents, resolved — worth reading

DATA-MODEL.md gives `purchases` this constraint:

```sql
CHECK ((status = 'paid') = (paid_at IS NOT NULL))
```

PAYMENTS.md § Refunds then sets `status = 'refunded'` on a paid purchase. That
row still carries the `paid_at` it was given when the money arrived, so the
check fails — **every refund would have errored at runtime.** A test caught it;
nothing else would have until the first refund.

Migration `00013` relaxes it to:

```sql
CHECK ((status IN ('paid','refunded')) = (paid_at IS NOT NULL))
```

The fix is deliberately *not* to clear `paid_at`. When the money arrived is part
of the record, and a refund does not un-happen the payment — PAYMENTS.md itself
says the history stays. Nulling the column to satisfy a constraint would destroy
exactly what the constraint exists to protect.

**`docs/backend/DATA-MODEL.md` still shows the original, broken constraint.**
It was left alone rather than edited unprompted; correct it there when
convenient, or the next person will reintroduce it.

### Payment tracing

Collections carry a short trace code (`KTF-3F9A2B7C`) derived from the
reference UUID, leading the description so it survives truncation into the
MarzPay statement and the payer's SMS. It is a **generated column** in
`payment_transactions`, computed by Postgres from the same reference Go derives
the description from, and a test asserts the two agree — a divergence would
silently break statement lookups for payments made after the change, and only
those.

`GET /v1/admin/revenue`, `GET /v1/admin/payments{,/{traceCode}}`, and the
`katafa revenue` / `katafa payment` commands read it. See
[`backend/README.md`](backend/README.md) § Tracking money.

### Tables added beyond DATA-MODEL.md

- **`prediction_voids`** — a voided prediction has no outcome, so it gets no
  `prediction_results` row. Without this it is indistinguishable from a pending
  one, the "made vs settled" gap is unexplainable in SQL, and
  `settle_predictions` re-examines cancelled matches every 30 minutes forever.
- **`correction_reviews`** — backs API.md's requirement that
  `POST /admin/matches/{id}/correct` flags affected predictions for review
  *without* re-grading.
- **`refunds`**, **`rate_limits`** (API.md wants limiting in Postgres, not
  memory), `slips.settled_odds` (SETTLEMENT.md asks for it by name),
  `leagues.is_published` (the shadow-mode gate INGESTION.md § Cold start
  requires), and `provider_budget.throttled_at` (so a 429's halved rate
  survives a worker restart).

### `tips.Select` is deterministic where the frontend was incidentally so

JavaScript's stable sort over kickoff-ordered matches gave the frontend an
implicit tie-break that Postgres does not promise. `Select` sorts by
(confidence desc, kickoff asc, prediction id) throughout. A republished day that
differed from the frozen one purely by query plan would be very hard to explain
and worse to discover.

### Test harness is `TEST_DATABASE_URL`, not testcontainers

ARCHITECTURE.md says testcontainers; its dependency tree would not fetch in a
reasonable time here. Same guarantee — real Postgres, a fresh database per test,
dropped afterwards — via `internal/testdb`. Tests **skip** when the variable is
unset so the pure suite runs anywhere.

### Secrets are only required in production

`config.Load` demands provider and MarzPay credentials when `ENV=production`
only. Demanding live credentials to boot locally is what pushes people to paste
real ones into a `.env` they then commit.

---

## Traps found while reading, worth not rediscovering

- **`getFreeTips` in `src/api/client.ts` must never be ported as a read path.**
  It re-derives the shortlist live. The backend selects once at 05:00 and
  freezes to `free_tip_days` / `free_tips`; the API only reads those rows.
- **`domain.MarketCodes` order is load-bearing.** The shortlist walks markets in
  that order and the shared per-fixture appearance cap makes the output depend
  on it. Reordering the slice changes which tips get published.
- **Scores are the 90-minute figure only.** football-data's `regularTime`
  overrides `fullTime` on knockout ties; API-Football's `fulltime` excludes
  `extratime` and `penalty`. Both clients already do this. Getting it wrong
  corrupts every goals market in knockout rounds.
- **A finished match with a score is never overwritten by ingestion** — the
  guard is in `UpsertMatch`'s `WHERE`. Corrections go through the admin path.
- **Reschedule edge case is handled but rare:**
  `PredictionsInvalidatedByReschedule` finds picks whose match moved *earlier*
  than the pick's own `created_at`, and voids them. INGESTION.md says to write
  the test for it. Still unwritten.
- **`region` gains `'asia'`.** `Region` and `REGIONS` in `src/api/types.ts` need
  it at cutover.
- **A draft slip returns 404, not 403**, so unpublished slip ids stay
  undiscoverable.
- **An invalid webhook signature returns 200, not 401** — never let an attacker
  distinguish a rejected forgery from an accepted one, and never make the
  provider retry a forgery.
- **`/v1/slips` is `no-store`** even though it returns only metadata: the
  response varies by whether the viewer owns each slip.

---

## Not to be done quietly

From AGENTS.md, repeated because these are the ones under time pressure:

- Never `UPDATE` a published prediction, free tip, tip, or slip row.
- Settlement never involves a model, a heuristic, or an LLM.
- Accuracy is computed over every settled prediction, no exclusions. Voiding is
  not the same as excluding a loss — a void has no outcome, a loss does.
- The paywall is a SQL boundary. Never fetch-then-hide, in any layer.
- Settled slips are public. Do not paywall history.
- Publication is gated on sample size (`model.MinHistoryPerTeam`, 40). This is
  the rule that will be tempting to skip in order to launch with more leagues on
  the page, and the accuracy dashboard would expose it publicly and permanently.
