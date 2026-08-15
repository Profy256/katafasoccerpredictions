# Katafa — Football Predictions Frontend

Frontend for the football predictions platform described in
`PRD-Football-Predictions-Platform.md`. Next.js 16 (App Router) + TypeScript +
Tailwind 4.

## Status

This is the **frontend only**. The Python ingestion workers, the Python
prediction engine, PostgreSQL, Redis and the Go API from PRD section 6 do not
exist yet.

To build a real UI without them, the Poisson model itself is implemented in
TypeScript and run against a **simulated season**. That means the numbers on
screen are internally consistent and the accuracy dashboard shows genuinely
computed figures — but the football is not real, and the site says so in a
banner on every page. Remove `src/components/DemoDataBanner.tsx` when a live
data provider is connected.

```bash
npm install
npm run dev      # http://localhost:3000
npm run build
npm run lint
```

## The two tiers

**Free** — a curated daily shortlist of the model's strongest picks, 3-5 per
market (`/`). Not the whole fixture list; that lives at `/fixtures`. Selection
logic is in `getFreeTips`, and two rules there are deliberate:

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
| `src/api/types.ts` | Domain types, mirroring the PRD section 7 schema |
| `src/api/client.ts` | **The swap point** — the frontend's entire data surface |
| `src/lib/poisson.ts` | Scoreline matrix + market derivation + grading |
| `src/lib/model.ts` | Team strength and expected-goals estimation |
| `src/lib/session.ts` | **Stub auth + stub purchases** — cookies, nothing verified |
| `src/app/actions.ts` | Stub sign-in and stub checkout server actions |
| `src/data/` | Season simulator standing in for ingestion + engine |
| `src/data/tipsters.ts` | Analysts, packages, slips, and tip grading |
| `src/app/` | Routes: free tips, pro, fixtures, accuracy, analysts |
| `src/components/charts/` | Accuracy timeline, hit-rate bars, calibration |

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

## Not yet real — must be replaced before launch

| Thing | State |
|---|---|
| Sign-in | Any email is accepted, password ignored, session is an unsigned cookie |
| Checkout | "Unlock" writes a cookie. **No money moves.** MarzPay is not integrated |
| Payments | MarzPay collection belongs on the Go API, not the browser |
| Analyst roster | Placeholder personas — swap for the real analysts |

The paywall itself is enforced where it should be: `getSlip` refuses to return
the tips for an unpaid open slip, so the selections never reach the browser.
That boundary must survive the move to the real API — do not "fix" it by
sending everything and hiding it in CSS.

### Swapping in the real API

Every function in `src/api/client.ts` is already `async` and already returns the
shape the Go API is expected to serve. Replacing the mock adapter means changing
those function bodies to `fetch` calls — no calling code moves, and no component
needs to change.

Delete `src/data/` at that point; nothing outside `client.ts` imports it.

## Model notes

The model follows PRD section 5.2 exactly: venue-split, recency-weighted attack
and defence strengths → expected goals → a Poisson scoreline matrix → every
market summed out of that one matrix. Because all six markets come from the same
matrix, they cannot contradict each other.

Two details worth knowing before touching this code:

- **Predictions are generated walk-forward.** The pick for round *N* is computed
  using only results from rounds before *N*, so the accuracy figures are a real
  out-of-sample backtest rather than the model grading itself on data it already
  saw. `src/data/generate.ts` is careful about this; keep it that way.
- **Double Chance probabilities sum to 200%, not 100%.** Its three outcomes
  overlap by definition. Render them as independent bars — never as a stacked
  share-of-100% chart.

Because the simulated results are drawn from the same Poisson family the model
assumes, the model is better calibrated against this data than it will be
against real football. Treat the demo hit rates as a UI fixture, not evidence.

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

Out of MVP scope per PRD section 4.5: user accounts, payments, push
notifications, native apps, and the deferred markets in section 3.2 (correct
score, HT/FT, handicaps, cards, corners, goalscorers).
