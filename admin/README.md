# Katafa admin

A second, separate Next.js app — not a section of `frontend/`. It's an
authenticated client of the same Go API, covering the `/v1/admin/*` surface
the public frontend never calls: creating and publishing Pro slips, settling
hand-graded tips, correcting a finished score, and reading revenue/payment
reports.

It carries no business logic. Every rule — kickoff cutoffs, immutability,
who's allowed to do this — is enforced by the API and the database; this app
only renders forms against it and relays whatever the API decides.

## Running it

```bash
npm install
cp .env.example .env.local   # point KATAFA_API_URL at the Go API
npm run dev                  # http://localhost:3001
```

Runs on port 3001 by default (`next dev -p 3001` / `next start -p 3001`) so it
can sit next to `frontend/` on 3000 without a port clash during local dev.

Signing in requires an account with `role = 'admin'`. There is no
self-registration and no bootstrap-admin flag — register a normal account
through the public frontend, then promote it directly in Postgres:

```sql
UPDATE users SET role = 'admin' WHERE email = 'you@yourdomain.com';
```

See the root [DEPLOYMENT.md](../DEPLOYMENT.md) for production deployment and
where every credential comes from.

## Layout

```
src/api/     http.ts (transport), client.ts (typed reads), types.ts
src/app/     one route per admin screen, actions.ts for every write
src/components/  Shell (nav + sign-out chrome), AddTipForm (the one screen
                 with real client-side interaction — cascading market →
                 valid-selection values)
```

## What's deliberately missing

- **No list of draft slips.** `GET /v1/slips` only returns published slips —
  a draft isn't a product yet, by design (see AGENTS.md's non-negotiables).
  Creating a slip here redirects straight to `/slips/[id]`; that URL is the
  record of it until it's published.
- **No list of tips awaiting hand-settlement**, and no list of finished
  matches to correct. Both screens (`/tips/[id]/settle`,
  `/matches/[id]/correct`) are opened directly by ID. Add a list view here if
  that becomes a frequent enough operation to be worth building.
- **No audit log viewer**, though every admin write is recorded in
  `audit_log` by the API. Query it directly in Postgres for now.
