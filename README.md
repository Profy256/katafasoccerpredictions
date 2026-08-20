# Katafa — Football Predictions

Football predictions platform: Next.js 16 (App Router) + TypeScript + Tailwind 4
on the front, Go + PostgreSQL behind it. Product scope is described in
`PRD-Football-Predictions-Platform.md`.

Two products on one dataset — a free daily shortlist from the Poisson model, and
hand-built slips from human analysts sold individually in UGX via MarzPay.

The only real asset is a **verifiable track record**, and the rules that protect
it are listed in [`AGENTS.md`](AGENTS.md). Read them before changing anything
that publishes, grades, or paywalls.

## Status

Frontend **and** Go backend. The frontend serves nothing of its own: every
figure on screen comes from the Go API over real, stored football data.

`src/` is the Next.js frontend. `backend/` is the Go API and workers. Postgres
is the only datastore — no Redis, no Python. Backend design lives in
[`docs/backend/`](docs/backend/README.md); read the relevant document before
changing code there, because the schema and the settlement rules are not
obvious from the frontend types alone.

```bash
# 1. Backend — see backend/README.md for the full set
docker run --rm -d --name katafa-pg -p 5433:5432 \
  -e POSTGRES_USER=katafa -e POSTGRES_PASSWORD=katafa -e POSTGRES_DB=katafa postgres:16

export DATABASE_URL='postgres://katafa:katafa@localhost:5433/katafa?sslmode=disable'
export MODEL_VERSION='poisson-1.2.0'
export PUBLIC_BASE_URL='http://localhost:8080'
export ALLOWED_ORIGINS='http://localhost:3000'

go -C backend run ./cmd/katafa migrate up
go -C backend run ./cmd/katafa jobs migrate
go -C backend run ./cmd/api      # :8080
go -C backend run ./cmd/worker   # jobs and cron

# 2. Frontend
npm install
export KATAFA_API_URL='http://localhost:8080'
npm run dev      # http://localhost:3000
npm run build
npm run lint
```

`KATAFA_API_URL` is required and has no default: a silent fallback to localhost
in production produces a site that renders empty rather than one that fails
loudly. It is deliberately not `NEXT_PUBLIC_` — every call is server-side, so
the API can live on an address the browser never resolves.

## The two tiers

**Free** — a curated daily shortlist of the model's strongest picks, 3-5 per
market (`/`). Not the whole fixture list; that lives at `/fixtures`.

The shortlist is **selected once at 05:00 and frozen** — `publish_free_tips`
writes it to `free_tip_days` / `free_tips` and the API only ever reads those
rows. It is never recomputed on read. Computed live it would be a function of
*current* data: once matches finish they stop being scheduled, so yesterday's
shortlist could not be reconstructed, and a model rerun would reconstruct a
different one. "We went 4 from 5 yesterday" needs a row saying what yesterday's
five were, written before those matches kicked off.

Selection lives in `backend/internal/tips`, and two rules there are
deliberate:

- an **odds floor** (1.25), because ranking on raw confidence surfaces things
  like Double Chance at 1.05 that nobody would bet;
- a **confidence ladder**, because odds here are derived from the model's own
  probability, so ranking purely by confidence returns four near-identical
  selections pinned to that floor. One pick per risk band gives a usable spread.

Each market has **its own route** — `/` is the default (Match Result), the rest
live at `/tips/[market]`. That is not cosmetic: an ad gate needs a navigation
boundary to sit on, which client-side tab state cannot provide.

**Pro** — hand-built slips from analysts, in three packages (Ordinary, VIP,
AKATAMBULA). Users buy **a single slip at a time**; there is no subscription.
The price therefore lives on the slip (`priceUgx`), set by the admin when the
slip is entered — the package's `typicalPriceUgx` is indicative only.

Once a slip settles its selections become public, which is what makes each
analyst's record auditable rather than a claim.

## Where things live

| Path | Role |
|---|---|
| `src/api/types.ts` | Domain types — the API returns these shapes verbatim |
| `src/api/client.ts` | The frontend's entire data surface. Transport only |
| `src/api/http.ts` | Fetch transport: caching policy, cookies, problem+json |
| `src/lib/poisson.ts` | Scoreline matrix + market derivation + grading |
| `src/lib/model.ts` | Team strength and expected-goals estimation |
| `src/app/actions.ts` | Sign-in, registration and MarzPay checkout actions |
| `src/app/` | Routes: free tips, pro, fixtures, accuracy, analysts |
| `src/components/charts/` | Accuracy timeline, hit-rate bars, calibration |
| `backend/` | Go API, workers, migrations — see `backend/README.md` |

`src/lib/poisson.ts` and `src/lib/model.ts` stay even though the model now runs
in Go: the methodology page describes this model, and
`backend/tools/parity/` diffs the Go port against them. The moment the two
implementations are allowed to drift, the published methodology stops
describing the published numbers.

## Advertising

Not wired to any network yet, but the seam is built and works. All config lives
in `src/lib/ads.ts`:

```ts
export const ADS_ENABLED = false;              // master switch
export const AD_GATED_MARKETS: MarketCode[] = [];  // e.g. ['BTTS']
```

Two separate mechanisms:

- **Slots** (`<AdSlot id="feed-top" />`) are inline banners. While disabled they
  render *nothing* — an empty box labelled "advertisement" is worse than no box.
  Sizes are declared up front so switching them on does not reflow the page.
- **Gating** is the interstitial shown before a market's tips. Add a market code
  to `AD_GATED_MARKETS` and `/tips/<that market>` serves the gate instead.

Two rules the implementation depends on, both verified:

1. **The gate returns the ad *instead of* the data.** The tips are never in the
   HTML behind it. If the gate were only an overlay, the picks would be readable
   from view-source and the ad impression would be worth nothing.
2. **The landing market is never gated**, whatever the config says
   (`isMarketGated` enforces this). Gating the page people arrive on blocks the
   free tier before anyone has seen a single tip.

`acknowledgeAdGateAction` currently trusts the button click. Wire it to the ad
network's completion callback before going live, or the gate is skippable.

## Before launch

| Thing | State |
|---|---|
| Provider ingestion | Both clients written and fixture-tested, but no live provider call has ever been made |
| MarzPay | Never called. The webhook signature scheme is **assumed** — verify it against the live dashboard before real money moves |
| Analyst roster | Seeded by hand; swap for the real analysts |
| Ad network | Seam built, no network wired. `acknowledgeAdGateAction` still trusts the click |

See [`HANDOFF.md`](HANDOFF.md) for what is verified and what is not.

The paywall is enforced where it should be — as a `WHERE` clause in the API's
query, so an unpaid viewer receives zero tip rows *from the database*. There is
no filtering step that could be forgotten and no serialisation path where the
tips sit in memory next to a boolean. Do not "fix" anything here by sending
everything and hiding it in CSS.

## Model notes

The model follows PRD section 5.2 exactly: venue-split, recency-weighted attack
and defence strengths → expected goals → a Poisson scoreline matrix → every
market summed out of that one matrix. Because all six markets come from the same
matrix, they cannot contradict each other.

Two details worth knowing before touching this code:

- **Predictions are generated walk-forward.** The pick for round *N* is computed
  using only results from rounds before *N*, so the accuracy figures are a real
  out-of-sample backtest rather than the model grading itself on data it already
  saw. The backtest harness asserts this rather than trusting it — a
  walk-forward violation is self-grading and worthless.
- **Double Chance probabilities sum to 200%, not 100%.** Its three outcomes
  overlap by definition. Render them as independent bars — never as a stacked
  share-of-100% chart.

**Watch the calibration chart weekly.** If the 70–80% confidence band does not
hit near 70%, the published confidence figures are misleading readers and the
model needs recalibrating before more leagues are added. It is the earliest
honest signal that something is wrong, and it is visible to users at the same
time it is visible to you.

## Design constraints

The theme is a committed dark palette. Chart colours were validated (lightness
band, chroma, colour-vision-deficiency separation, WCAG contrast) against the
card surface rather than picked by eye. Two rules fall out of that and are load
bearing:

- **Magnitude charts use one hue.** Bar length already encodes the value;
  shading bars darker-where-bigger double-encodes it and fails the categorical
  checks.
- **Hit/miss never relies on colour.** The good and critical colours sit ~4.1 ΔE
  apart under deuteranopia, so `OutcomeBadge` always ships an icon *and* the
  word. Do not reduce it to a coloured dot.

## Not built (deliberately)

Push notifications, native apps, analyst self-service authoring (slips are
admin-entered until the record is established), bookmaker odds ingestion — odds
today are derived from model probability, which is honest but is not a real
price — and the deferred markets in PRD section 3.2 (correct score, HT/FT,
handicaps, cards, corners, goalscorers).
