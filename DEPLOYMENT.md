# Deploying Katafa

Five things run in production, each a separate deploy target:

| Process | What it is | Source |
|---|---|---|
| `api` | Stateless HTTP server — `/healthz`, `/readyz`, `/v1/…`. Scale horizontally. | `backend/cmd/api` |
| `worker` | River job runner — ingestion, prediction, settlement, payment reconciliation, cron. Run exactly one (River's `SELECT … FOR UPDATE SKIP LOCKED` allows more, but there's no reason to yet). | `backend/cmd/worker` |
| `katafa` | One-off admin CLI — migrations, seeding, backfills, manual settlement. Run it, don't host it. | `backend/cmd/katafa` |
| frontend | Next.js, public. Calls the API server-side only; no browser talks to it directly. | `frontend/` |
| admin | Next.js, ops-only. A second, separate client of the same API — creating/publishing slips, settling hand-graded tips, correcting scores, revenue/payment reports. Keep it off the public frontend's domain and out of search (see [admin/README.md](admin/README.md)); it has no self-registration, so exposure is a smaller risk than the payment webhook, but it's still where every write to money-adjacent tables happens. | `admin/` |

Plus one Postgres 16 instance. No Redis, no message broker, no Python —
Postgres is the only datastore, including the job queue (see
[docs/backend/README.md](docs/backend/README.md)).

There's no Dockerfile or IaC in this repo yet — pick a host, build the two Go
binaries (`go build ./cmd/api` and `go build ./cmd/worker`) or run them with
`go run`, and point each Next.js app's standard `next build && next start`
(`frontend/` and `admin/` independently) at whatever platform you use
(Vercel, Fly, Render, a VM — nothing here is platform-specific).

## 1. Provision Postgres

Any managed Postgres 16 works (RDS, Neon, Supabase, Fly Postgres, a VM you
run yourself). You need:

- A connection string with `sslmode=require` (or your provider's equivalent)
  in production — `sslmode=disable` is fine for local dev only.
- A role that can create tables/indexes/triggers (the `katafa` CLI runs the
  migrations; it needs DDL rights, not just `SELECT`/`INSERT`).

Once you have `DATABASE_URL`, run both migration sets from a machine that can
reach it:

```bash
go run ./backend/cmd/katafa migrate up     # domain schema
go run ./backend/cmd/katafa jobs migrate   # River's own tables
```

## 2. Collect credentials

Every credential below maps to an environment variable read by
[backend/internal/config/config.go](backend/internal/config/config.go).
`backend/.env.example` is the authoritative list — copy it to `backend/.env`
for local work; in production, inject these through your host's secret
manager (Fly secrets, Render env groups, AWS Secrets Manager, etc.), never a
committed file. **Only `.env.example` is meant to be in git — it must never
contain a real value**, only blanks or non-secret defaults.

### Football data providers

Two providers, routed per competition (see
[docs/backend/INGESTION.md](docs/backend/INGESTION.md)). Both are free-tier.
Get both before going to production — dropping either one means the leagues
it uniquely covers stop ingesting.

**`FOOTBALL_DATA_TOKEN`** — football-data.org (12 competitions, Europe +
Brasileirão, 10 req/min, no daily cap):
1. Go to https://www.football-data.org/client/register and register an
   account (email + password, free).
2. After confirming, the token is shown on your account dashboard at
   https://www.football-data.org/client/register — it's a single opaque
   string, no OAuth flow.
3. Set `FOOTBALL_DATA_TOKEN` to that value. Sent as header `X-Auth-Token`.

**`APIFOOTBALL_KEY`** — API-Football / api-sports.io (~1,200 leagues incl.
Uganda, CAF, Asia; 100 req/day, resets 00:00 UTC):
1. Go to https://dashboard.api-football.com/register (or via
   https://rapidapi.com/api-sports/api/api-football — either surface works,
   the direct dashboard is simpler for a plain API key).
2. Register, verify email, then open the dashboard — your key is listed
   under "My Access" / API Keys.
3. Set `APIFOOTBALL_KEY` to that value. Sent as header `x-apisports-key`.

Both are read only in `ENV=production` — `config.Load` requires them then.
Development runs against recorded fixtures in
`backend/internal/ingest/*/testdata/` and needs neither.

### MarzPay (Pro tier payments)

Mobile money collection in UGX, verified against
https://wallet.wearemarz.com/documentation/collections (see
[docs/backend/PAYMENTS.md](docs/backend/PAYMENTS.md)).

1. Sign up for a MarzPay merchant account — go to
   https://wallet.wearemarz.com and register as a business/merchant (MarzPay
   is Uganda-focused; expect KYC — business registration or a national ID,
   depending on their current onboarding requirements).
2. Once approved, the merchant dashboard exposes:
   - **`MARZPAY_API_USER`** and **`MARZPAY_API_KEY`** — the HTTP Basic auth
     pair for `/collect-money`, `/send-money`, and status lookups. Usually
     under an "API" or "Developer" settings page.
   - **A webhook/callback signing secret** → `MARZPAY_WEBHOOK_SECRET`. Look
     for "Webhooks" or "Callback settings" in the dashboard.
3. Set `MARZPAY_BASE_URL` (defaults to
   `https://wallet.wearemarz.com/api/v1`, which is almost certainly correct
   — only override for a sandbox environment if MarzPay gives you one).
4. **Before real money moves**, verify the callback signature scheme against
   the live dashboard. `backend/internal/pay/marzpay/client.go` currently
   *assumes* HMAC-SHA256 over the raw webhook body, signature in the
   `X-Marzpay-Signature` header — this was never confirmed against MarzPay
   directly. If it's wrong, every webhook fails signature verification
   (recorded with `signature_valid = false`) and purchases only complete via
   the slower `reconcile_payments` job, not instantly. Ask MarzPay support to
   confirm the scheme, or trigger one real sandbox/live payment and diff the
   received signature against what the code computes.
5. `PUBLIC_BASE_URL` must be the API's real internet-reachable origin
   (`https://api.yourdomain.com`, no trailing slash) — it's used to build the
   callback URL MarzPay POSTs to, so it cannot be `localhost` in production.

Without `MARZPAY_API_USER`/`MARZPAY_API_KEY` set, the app wires a fake
payment provider automatically (`Config.MarzPayConfigured()` returns false) —
that's fine for staging a demo, but Pro purchases will never actually charge
anyone.

### Everything else

- **`DATABASE_URL`** — from step 1, include `sslmode=require` in production.
- **`ENV`** — `production`. This is what makes the four secrets above
  mandatory at boot; `config.Load` refuses to start without them once set.
- **`MODEL_VERSION`** — not a credential, but required with no default (e.g.
  `poisson-1.2.0`). Stamped on every prediction; bump it when the model
  changes so the accuracy dashboard can attribute results correctly.
- **`ALLOWED_ORIGINS`** — comma-separated, the real frontend origin(s)
  (`https://katafa.app`). No wildcard fallback by design — CORS defaulting to
  `*` in production is exactly the failure this guards against.
- **`SESSION_COOKIE_DOMAIN`** — e.g. `.katafa.app`. Required in production so
  session cookies work across subdomains if you split API/frontend hosts.
- **`KATAFA_API_URL`** (frontend) — the API's origin, reachable from wherever
  Next.js runs server-side. Not `NEXT_PUBLIC_` — it's never sent to the
  browser, so it can point at a private/internal address if API and frontend
  share a network.
- **`NEXT_PUBLIC_SITE_URL`** (frontend) — the frontend's real public origin,
  used to build absolute Open Graph/Twitter image URLs. Must be a real
  address crawlers can resolve, not `localhost`.
- **`KATAFA_API_URL`** (admin) — same variable, same rule, in `admin/`. It's a
  separate app with its own env file (`admin/.env.example`), not a section of
  the frontend one.

## 3. Deploy order

1. Apply migrations against the production database (step 1).
2. Deploy `worker` with `ENV=production` and every secret set. It will
   refuse to boot otherwise — `config.Load` collects and reports every
   missing variable in one pass, so check its startup logs first if it
   exits immediately.
3. Deploy `api` the same way.
4. Deploy the frontend with `KATAFA_API_URL` pointed at the live API and
   `NEXT_PUBLIC_SITE_URL` set to the live frontend origin.
5. Deploy `admin/` the same way, with `KATAFA_API_URL` pointed at the same
   live API. Put it on its own subdomain (e.g. `admin.katafa.app`), not a
   path under the public site — it shares the API's session cookie
   (`SESSION_COOKIE_DOMAIN`) but is otherwise a completely separate deploy,
   and there's no reason for it to be linked from, or discoverable via, the
   public frontend.
6. Seed and publish at least one league before announcing anything —
   leagues start unpublished (shadow mode) until there's enough history for
   the strength estimates to mean something:

   ```bash
   go run ./backend/cmd/katafa seed leagues \
     -slug=eng-premier-league -name="Premier League" -short-name=EPL \
     -country=England -country-code=ENG -region=europe \
     -provider=football-data -provider-id=2021

   go run ./backend/cmd/katafa backfill -csv=E0.csv -league=eng-premier-league
   ```

   Then publish deliberately:

   ```bash
   go run ./backend/cmd/katafa leagues list
   go run ./backend/cmd/katafa leagues publish eng-premier-league \
     -by=you@yourdomain.com -reason="seeded 25 seasons from E0.csv"
   ```

   This used to be a hand-written `UPDATE leagues SET is_published = TRUE`,
   on the reasoning that publishing is a one-way door and should not be one
   flag away from an accidental default. The door is still one-way; the raw
   UPDATE was just a bad way to guard it. It recorded no reason, no operator,
   and — worst — checked nothing, so it could not tell you that the league you
   were about to publish had eight matches of history.

   `leagues publish` keeps the deliberateness and adds the check: `-by` must
   resolve to a real `role = 'admin'` user, `-reason` is mandatory, both land
   in `audit_log` alongside the sample-size evidence, and it **refuses** a
   league where no upcoming fixture clears the `MinHistoryPerTeam` floor
   unless you pass `-force`. `leagues list` shows every league against that
   gate, which is also the fastest answer to "why is the site so empty?" — a
   live league with `READY 0` publishes no tips.

   `leagues unpublish` returns a league to shadow mode. It does not retract
   anything already published: free tips are immutable, and settled history
   stays public.

## 4. Promoting an admin/analyst user

There is no bootstrap-admin flag or back door by design. Register a normal
account through the app, then promote it directly in Postgres:

```sql
UPDATE users SET role = 'admin' WHERE email = 'you@yourdomain.com';
-- or 'analyst' for someone who builds Pro slips but shouldn't hit /v1/admin/*
```

Register the account through the public frontend first (`/login?mode=register`),
then run the query above, then sign in at the admin app with the same
credentials.

## 5. Verify before real traffic

- `GET /healthz` and `/readyz` on the deployed API.
- Trigger one real MarzPay collection end-to-end and confirm the webhook
  lands with `signature_valid = true` (see the warning in step 2) — this is
  the one integration that touches real money and was never live-verified
  before this deploy.
- Confirm `go test ./...` with `TEST_DATABASE_URL` set passes against a
  throwaway database before trusting migrations against production — CI
  already does this on every push (`.github/workflows/ci.yml`), but it's
  worth re-running locally if you've patched anything post-merge.
- Sign in at the deployed admin app with a promoted account and create one
  throwaway draft slip end to end (create → add a tip → delete, without
  publishing) — confirms the admin cookie, CORS, and `/v1/admin/*` are all
  actually wired together, not just individually reachable.

## A note on `backend/.env.example`

This file is intentionally tracked in git (unlike `.env`/`.env.local`,
which are gitignored) — it's the only record of which environment variables
exist. It must **only** ever contain variable names with blank or clearly
fake values. If you ever find a real key, token, or secret sitting in it,
treat it as compromised: rotate that credential with the provider
immediately, then scrub it from the file — and check `git log -p --all --
backend/.env.example` to see whether it ever reached a pushed commit, in
which case rotation is mandatory, not optional, since removing it later does
not remove it from history.
