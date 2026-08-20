# HTTP API

Base path `/v1`. JSON in, JSON out. Errors are
[problem+json](https://datatracker.ietf.org/doc/html/rfc9457).

The contract is `src/api/client.ts`. Each endpoint below names the client
function it serves; response bodies are the types in `src/api/types.ts`
verbatim, camelCase, so the frontend swap is a change of transport only.

## Mapping

| Client function | Endpoint | Auth | Cache |
|---|---|---|---|
| `getLeagues` | `GET /leagues` | — | `max-age=3600` |
| `getFeed` | `GET /feed` | — | `max-age=300` |
| `getMatchDetail` | `GET /matches/{id}` | — | `max-age=300` |
| `getAccuracySummary` | `GET /accuracy` | — | `max-age=900` |
| `getSettledPredictions` | `GET /predictions/settled` | — | `max-age=900` |
| `getFreeTips` | `GET /tips/free` | — | `max-age=600` |
| `getFreeTipsHistory` | `GET /tips/free/history` | — | `max-age=3600` |
| `getPackages` | `GET /packages` | — | `max-age=3600` |
| `getSlips` | `GET /slips` | optional | `no-store` |
| `getSlip` | `GET /slips/{id}` | optional | `private, no-store` |
| `getSession` | `GET /auth/me` | required | `no-store` |
| `getPurchases` | `GET /purchases` | required | `no-store` |
| `getAnalysts` | `GET /analysts` | — | `max-age=3600` |
| `getAnalystRecord` | `GET /analysts/{slug}` | — | `max-age=900` |
| `getAnalystLeaderboard` | `GET /analysts/leaderboard` | — | `max-age=900` |
| `getCoverageStats` | `GET /stats/coverage` | — | `max-age=600` |

`getSlips` is `no-store` despite returning only metadata, because the response
varies by whether the viewer owns each slip.

## Query parameters

```
GET /feed?leagues=<uuid,uuid>&markets=<CODE,CODE>&min_confidence=<0-100>&region=<region|all>
GET /predictions/settled?leagues=&markets=&outcome=all|hit|miss&limit=&cursor=
GET /tips/free?date=YYYY-MM-DD           omit for the current published day
GET /tips/free/history?from=&to=
GET /slips?package=&status=open|settled&analyst=&limit=&cursor=
```

Multi-value parameters are comma-separated, not repeated keys. Unknown values
are a `400`, not silently ignored — a typo'd market code returning the unfiltered
feed is worse than an error.

Pagination is cursor-based (opaque, encodes `(sort_key, id)`) on the two
endpoints that grow without bound: `/predictions/settled` and `/slips`. Offset
pagination over a table that gains rows daily shows duplicates across pages.

## The free-tips history endpoint

```
GET /tips/free/history?from=2026-08-01&to=2026-08-13
→ { days: [ { date, totalTips, settled, correct, hitRate,
              groups: [ { market, tips: [ …FreeTip + result ] } ] } ] }
```

Serving "yesterday's free tips went 4 from 5" needs the frozen shortlist joined
to `prediction_results`. It reads `free_tips`, never a live re-selection — see
[SETTLEMENT.md](SETTLEMENT.md#why-the-free-shortlist-is-frozen). `client.ts`
exposes it as `getFreeTipsHistory`.

## Auth

```
POST /auth/register   { email, password, name, phone? } → 201 + session cookie
POST /auth/login      { email, password }               → 200 + session cookie
POST /auth/logout                                       → 204
GET  /auth/me                                           → { user }
```

Session is an opaque 32-byte token in an `HttpOnly; Secure; SameSite=Lax`
cookie, sha256-hashed in `sessions`. Login is rate-limited per email *and* per
IP — per IP alone lets a botnet spray one account, per email alone lets one IP
enumerate accounts.

This replaced `src/lib/session.ts` entirely — that file's cookies were forgeable
by anyone and said so, and it was deleted rather than adapted. `getSession()` in
`client.ts` calls `GET /auth/me`; the browser never talks to this API directly,
so `relaySessionCookie` in `src/app/actions.ts` copies the token across the
server-to-server hop.

## Purchases

```
POST /slips/{id}/purchase   { phone_number } → 202 { purchaseId, status }
GET  /purchases/{id}                         → { status, slipId, priceUgx, paidAt? }
GET  /purchases                              → the viewer's purchases
POST /webhooks/marzpay                       → 200 (public, signature-verified)
```

Flow detail in [PAYMENTS.md](PAYMENTS.md#purchase-flow).

## Admin

All under `/admin`, all require `role = 'admin'`, all write `audit_log`.

```
POST   /admin/slips                    create a draft
POST   /admin/slips/{id}/tips          add a tip to a draft
POST   /admin/slips/{id}/publish       draft → open; validates every kickoff is future
DELETE /admin/slips/{id}               only while draft
POST   /admin/tips/{id}/settle         { wasCorrect, actualOutcome, reason }
POST   /admin/matches/{id}/correct     { homeScore, awayScore, reason }
POST   /admin/predictions/publish      force a generation run
GET    /admin/ingest/status            per-provider budget, last sync, gaps
```

`POST /admin/slips/{id}/publish` refuses if any tip's `kickoff_at` has passed.
Publishing a slip containing a started match is the exact shape of the fraud the
whole design is meant to prevent, and it is an easy accident at 3pm on a
Saturday.

`POST /admin/matches/{id}/correct` is the only path that may change a finished
score. It requires a reason, writes `audit_log`, and — because
`prediction_results` is immutable — does **not** silently re-grade. It flags
affected predictions for review and surfaces them in the admin UI as a
correction the team must publish.

## Errors

```json
{ "type": "https://katafa.app/errors/slip-not-purchasable",
  "title": "Slip is not open for purchase",
  "status": 409,
  "detail": "Slip 8f3e… settled at 2026-08-13T19:04:00Z",
  "instance": "/v1/slips/8f3e…/purchase" }
```

| Status | Used for |
|---|---|
| 400 | Malformed body, unknown enum value, bad cursor |
| 401 | No session where one is required |
| 403 | Authenticated but wrong role |
| 404 | Unknown id — **also** returned for a draft slip to a non-admin |
| 409 | State conflict: already purchased, slip closed, already settled |
| 422 | Well-formed but rejected: phone outside MarzPay's range |
| 429 | Rate limited, with `Retry-After` |

A draft slip returns `404` rather than `403` so that unpublished slip ids are
not discoverable.

## Cross-cutting

- **CORS**: the Next.js origin only. Most reads are server-side, so this is
  narrow by default.
- **Rate limiting**: token bucket per IP for public reads, per user for writes.
  In Postgres, not in memory, so it survives a restart and works across
  replicas.
- **Request id** on every request, echoed as `X-Request-Id` and attached to
  every log line.
- **`GET /healthz`** (process up) and **`GET /readyz`** (Postgres reachable,
  migrations current) outside `/v1`.
- **Timestamps** serialise as RFC3339 UTC with `Z`, matching the ISO strings the
  frontend types already expect.
