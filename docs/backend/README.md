# Katafa backend

Go + PostgreSQL. No Redis, no Python, no message broker.

The frontend at `src/api/client.ts` is the contract this backend serves. Every
function there is already `async` and already returns the response shape the API
must produce — treat that file as the spec and do not change its signatures.

## Read in this order

| Document | What it settles |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Binaries, package layout, library choices, request and job flow |
| [DATA-MODEL.md](DATA-MODEL.md) | Full schema, constraints, and the triggers that enforce the invariants |
| [INGESTION.md](INGESTION.md) | Multi-provider fixture/result sync, rate budgets, history accumulation |
| [SETTLEMENT.md](SETTLEMENT.md) | Grading rules per market, void handling, slip settlement, idempotency |
| [PAYMENTS.md](PAYMENTS.md) | MarzPay collection flow, webhooks, entitlement, reconciliation |
| [API.md](API.md) | HTTP surface mapped one-to-one onto `client.ts` |
| [ROADMAP.md](ROADMAP.md) | Build order, with the reasoning for the sequence |

## Decisions already made

These were chosen deliberately; reopen them only with a reason.

- **All Go.** The PRD proposed Python for ingestion and the prediction engine.
  The Poisson model is a few hundred lines of numerics with no ML dependency,
  so a second language and a second deploy target buys nothing. `internal/model`
  sits behind an interface, so a separate service can replace it later without
  the API noticing.
- **Postgres does the queueing.** [River](https://riverqueue.com) runs jobs on
  `SELECT … FOR UPDATE SKIP LOCKED` with built-in cron. One datastore to back
  up, one to restore, and jobs commit in the same transaction as the rows they
  produce — which is what makes settlement exactly-once rather than
  approximately-once.
- **Multi-provider ingestion from day one.** Coverage spans Europe, East
  Africa, the rest of Africa, the Americas and Asia, and no single free tier
  covers that. Competitions are routed to whichever provider carries them.
- **MarzPay in v1.** The Pro tier is the revenue, so it ships with the first
  backend rather than after it.

## The thing most likely to be got wrong

Free API tiers give you *fixtures going forward*, not *history going back*.
football-data.org's free plan covers 12 competitions with no deep archive;
API-Football's free plan reaches all leagues but caps historical seasons.

The Poisson model needs history to estimate attack and defence strength. So the
history is **yours to accumulate**: every result ever fetched is written to
Postgres permanently, and `internal/model` reads only from Postgres. The
provider is a tap you turn on daily, never a database you query at request time.

A cold start with no history is a real state the system must handle honestly —
see [INGESTION.md § Cold start](INGESTION.md#cold-start-and-the-honesty-problem).
