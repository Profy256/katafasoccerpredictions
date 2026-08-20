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
| 2 — Ingestion | **code complete + fixture-tested; no live provider call has ever been made** |
| 3 — Model | **done** — Go/TypeScript parity verified to 1e-12, regenerated in CI |
| 4 — Settlement + accuracy | **done** — grading, voids, rollups, endpoints |
| 5 — Free tier | **done** — selection, freeze, both endpoints |
| 6 — Accounts + Pro reads | **done** — entitlement is in SQL, and now proven by test |
| 7 — MarzPay | **code complete, sandbox-untested**; the lost-webhook guarantee is proven |
| 8 — Frontend cutover | **done** — nothing in `src/` generates data |

Verified in this build: `go build ./...`, `go vet ./...` and `gofmt -l .` clean;
`go test -race ./...` green including the database suite; the parity fixture
regenerated from the TypeScript model and byte-identical to the committed one;
`tsc --noEmit`, `eslint` and `next build` clean.

Verified end to end against a live API and a real Postgres, through the browser:

- every route renders from the API — `/`, `/accuracy`, `/fixtures`, `/pro`,
  `/analysts`, `/analysts/[slug]`, `/matches/[id]`, `/tips/[market]`, `/login`,
  `/pro/slips/[id]` — and unknown ids 404 rather than erroring;
- an anonymous viewer on an **open** slip gets no selections *in the HTML at
  all*, only the locked shell;
- the same viewer on a **settled** slip gets every selection, including the
  losing one;
- register → purchase → signed webhook → the slip unlocks for that buyer, while
  a second viewer is still locked out;
- the trace code the API returned matched the one Postgres generated.

### What is NOT done — read this before trusting anything

1. **No provider has ever been called.** Both clients now have fixture tests
   covering the parsing rules that matter — the 90-minute score on knockout
   ties, status vocabulary, the archive-before-parse guarantee — but the
   fixtures in `testdata/` were built to the *documented* response shapes, not
   captured from a live call. They cannot catch a field the documentation gets
   wrong. Replace them with real captured responses after the first live sync;
   `provider_payloads` archives every response before parsing precisely so that
   costs nothing extra. The assertions should survive the swap unchanged — if
   they do not, the shape differed, which is exactly the discovery worth making
   cheaply.
2. **MarzPay has never been called.** The webhook signature scheme is
   *assumed* to be HMAC-SHA256 over the raw body, read from
   `X-Marzpay-Signature` with `X-Signature` / `Signature` fallbacks. **Verify
   this against the live dashboard before going near real money.** If it is
   wrong, every callback fails verification, is recorded with
   `signature_valid = false`, and is never acted on — purchases would then
   complete only via `reconcile_payments`, silently and slowly. That path is now
   tested (`TestLostWebhookStillCompletesThePurchase`), so it does work; it is
   just slow and silent, which is the wrong way to find out the scheme is wrong.
3. **`sync_competitions` is a stub.** It logs and returns. Fixture responses
   carry team ids, so teams are created on demand by `TeamIDBySource`; the
   roster job only ever existed to keep names fresh.
4. **The ad gate still trusts the click.** `acknowledgeAdGateAction` records the
   acknowledgement on submit rather than on an ad-network completion callback.
   Harmless today — `ADS_ENABLED` is false, so no market is gated and the action
   is unreachable — but it must be wired before ads go live or the gate is
   skippable.
5. **No live data.** The database holds whatever you seed. Nothing is published
   until a league has `model.MinHistoryPerTeam` (40) matches per team and is
   flipped to `is_published`.

### Tests written to close phases 6 and 7

- `internal/postgres/entitlement_test.go` — ROADMAP phase 6's "done when". An
  unpaid viewer receives zero tip rows, asserted through the same `db.Slip`
  call the handler makes, for anonymous / signed-in-never-bought / pending /
  failed / refunded. The load-bearing part is that it first asserts the tips
  *are* in the table, so zero rows back is the query filtering rather than an
  empty fixture.
- `internal/pay/service_test.go` — ROADMAP phase 7's "done when". A lost
  webhook still completes the purchase, proven by a test that never constructs
  one and then asserts `payment_webhook_events` is empty. Plus: duplicate
  callbacks grant once, webhook-and-reconciliation do not double-grant, a
  forged signature is recorded but never acted on, a late `failed` does not
  revoke a paid purchase, and a collection that never reached the gateway is
  recoverable and expires after 24 hours.
- `internal/settle/settle_db_test.go` — the reschedule case INGESTION.md asked
  for and nothing covered: a fixture moved *earlier* than the prediction that
  was written for it is voided, with an audit entry, and is not re-voided on
  the next pass. Also that accuracy counts every loss and no voids.
- `internal/ingest/{footballdata,apifootball}/client_test.go` — phase 2's
  recorded-fixture tests, with the caveat in point 1 above.

The mutation check is worth repeating if you touch reconciliation: breaking
`applyStatus` makes `TestLostWebhookStillCompletesThePurchase` fail with
"purchase status after reconciliation = pending", which is the failure a real
user would experience as paying and receiving nothing.

### Suggested order from here

1. **Verify the MarzPay webhook signature scheme against the live dashboard.**
   This is the single highest-value unknown left, and everything about revenue
   depends on the guess being right.
2. Capture real provider responses into `testdata/` — one free call each — and
   replace the hand-built fixtures.
3. MarzPay sandbox end to end, then a live low-value transaction.
4. Seed and backfill real leagues, let history accumulate to 40 matches per
   team, then publish deliberately.

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

`docs/backend/DATA-MODEL.md` showed the original, broken constraint for a
while. It has since been corrected to match migration `00013`, with the reason
inline, so the next person does not reintroduce it.

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

### The cutover: what changed shape in `src/`

ROADMAP said "signatures unchanged". Two could not stay, and both for the same
reason — they encoded decisions that have moved into the database:

- **`getSlip(slipId)` lost its `ownedSlipIds` argument.** Entitlement is now a
  `WHERE` clause in the API's query. Passing a set of owned ids from the caller
  would mean the frontend deciding what to show, which is the fetch-then-hide
  the paywall exists to prevent. `unlocked` now *reports* what the query
  returned rather than deciding it.
- **`getFreeTips(perMarket)` became `getFreeTips(date?)`.** See the traps
  section.

`getOwnedSlipIds` survives, but it now reads `GET /v1/purchases` and is only
used to badge the slips *list*. It must never gate rendering of tips.

### Where the session lives now

`src/lib/session.ts` is deleted, as API.md instructed. Its three jobs went to
three different places, deliberately:

- sign-in and purchases → the API, because those cookies granted something and
  were forgeable;
- the ad-gate cookie → `src/lib/ad-gates.ts`, because it grants nothing. The
  worst a forged value does is skip an advert;
- reading the session → `getSession()` in `client.ts`, which calls
  `GET /v1/auth/me`.

The browser never talks to the API directly. `relaySessionCookie` in
`actions.ts` copies the token across the server-to-server hop and re-states the
cookie attributes rather than parsing them out of the `Set-Cookie` header.

### `apiGet` versus `apiGetPrivate` is load-bearing

`apiGet` never touches cookies, which is what keeps a route statically
cacheable — reading cookies in a server component opts the whole route into
dynamic rendering. `apiGetPrivate` forwards the session cookie and is always
`no-store`.

Getting that the wrong way round is not a style question: a cached response
from a private endpoint is one viewer's paid slip served to another.

### `not-found.tsx` is `force-dynamic`, on purpose

The root layout reads the session to decide what the header shows. A statically
prerendered 404 bakes in whatever that was at build time — "signed out", for
everyone, forever. Forcing it dynamic also means `next build` no longer needs
the API reachable just to emit that one page, which is what lets CI build
without standing up a backend.

### `KATAFA_API_URL` has no default

A silent fallback to localhost in production produces a site that renders empty
rather than one that fails loudly. It is deliberately not `NEXT_PUBLIC_`: every
call is server-side, so the API can live on an address the browser never
resolves.

### Secrets are only required in production

`config.Load` demands provider and MarzPay credentials when `ENV=production`
only. Demanding live credentials to boot locally is what pushes people to paste
real ones into a `.env` they then commit.

---

## Traps found while reading, worth not rediscovering

- **`getFreeTips` must never go back to deriving the shortlist.** It is now a
  plain read of `GET /v1/tips/free`, which reads the frozen `free_tip_days` /
  `free_tips` rows. It also no longer takes a `perMarket` argument: how many
  tips a market carries was decided by the publish job when it froze the day,
  and asking for a different number on read is re-selection wearing a
  parameter.
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
  than the pick's own `created_at`, and voids them. Now covered by
  `TestRescheduleEarlierThanThePredictionVoidsIt`.
- **A published slip cannot be demoted to draft.** `slips_guard_update` refuses
  it, which is correct and caught a test of mine that took the shortcut. Seed a
  second, genuinely-draft slip instead of unpublishing one.
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
