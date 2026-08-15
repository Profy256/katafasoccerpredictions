<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->

# Katafa

Football predictions platform. Two products on one dataset:

- **Free tier** — a curated daily shortlist from the Poisson model, published per market.
- **Pro tier** — hand-built slips from human analysts, sold individually in UGX via MarzPay.

`src/` is the Next.js frontend. `backend/` is the Go API and workers. Postgres is
the only datastore — no Redis, no Python.

Backend design lives in [`docs/backend/`](docs/backend/README.md). Read the
relevant document before writing backend code; the schema and the settlement
rules are not obvious from the frontend types alone.

## Non-negotiables

These exist because the product's only real asset is a **verifiable track
record**. Every rule below protects it. Breaking one silently turns published
history into fiction, which is worse than a crash — nobody notices.

1. **Published predictions are immutable.** Never `UPDATE` a row in
   `predictions`, `free_tips`, `tips`, or `slips` once published. Corrections
   are new rows plus an `audit_log` entry.

2. **A prediction must exist before kickoff.** Enforced by trigger, not by
   convention. A pick written after the whistle is not a prediction.

3. **Settlement is deterministic and never involves an LLM.** Grading reads a
   final score and applies a pure function. No model, no heuristic, no
   "reasonable inference" about what probably happened.

4. **The daily free shortlist is materialised at publish time**, never
   recomputed on read. `getFreeTips` in the frontend mock derives it live from
   current data; the backend must not. See [SETTLEMENT.md](docs/backend/SETTLEMENT.md#why-the-free-shortlist-is-frozen).

5. **Accuracy is computed over every settled prediction, no exclusions.** No
   filtering out losses, no "excluding postponed", no cherry-picked windows.

6. **The paywall is a SQL boundary.** An unpaid viewer's query must not return
   tip rows at all. Never fetch-then-hide, in any layer.

7. **Settled slips become public.** That is what makes an analyst's record
   auditable rather than a claim. Do not paywall history.

8. **Walk-forward only.** A prediction for match *M* may use only results from
   matches that kicked off before *M*. Backtests that violate this are
   self-grading and worthless.

9. **The database is the system of record for football history, not the
   provider.** Free API tiers cap historical seasons; you accumulate history by
   storing every result you ever fetch. The model reads from Postgres, never
   from a provider.

10. **UGX is a zero-decimal currency.** Money is `int64` shillings and
    `BIGINT` columns. Never float, never "cents". Odds are `NUMERIC(7,3)` and
    `decimal.Decimal` — also never float.

11. **All timestamps are `TIMESTAMPTZ` in UTC.** Kickoff times drive settlement
    windows; a naive timestamp is a settlement bug waiting to happen.
