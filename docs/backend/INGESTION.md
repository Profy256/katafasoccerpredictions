# Ingestion

Two providers, routed per competition, both on free tiers with hard daily
ceilings. The job of this layer is to spend a small request budget well and to
never lose a result once seen.

## Providers

| | football-data.org | API-Football (api-sports.io) |
|---|---|---|
| Free limit | 10 requests/minute, no daily cap | 100 requests/day, resets 00:00 UTC |
| Coverage | 12 competitions, Europe + Brasileirão | ~1,200 leagues incl. Uganda, CAF, Asia |
| History | Shallow on free | All endpoints, **historical seasons capped** |
| Auth | `X-Auth-Token: <token>` | `x-apisports-key: <key>` |
| Scores | Delayed on free; fine for settlement | Delayed on free |
| Base URL | `https://api.football-data.org/v4` | `https://v3.football.api-sports.io` |

Neither is a fallback for the other. They are assigned per competition:
football-data.org where it has the competition (better data quality, and its
per-minute limit means bulk backfills are practical), API-Football for
everything else — Ugandan Premier League, CAF, the Asian leagues, and any
second-tier competition.

```sql
CREATE TABLE competition_sources (
    league_id            UUID NOT NULL REFERENCES leagues(id),
    provider             TEXT NOT NULL,
    provider_competition_id TEXT NOT NULL,
    is_primary           BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (league_id, provider)
);
CREATE UNIQUE INDEX ON competition_sources (league_id) WHERE is_primary;
```

## The Provider interface

```go
type Provider interface {
    Name() string
    Competitions(ctx context.Context) ([]RawCompetition, error)
    Teams(ctx context.Context, comp string, season int) ([]RawTeam, error)
    Fixtures(ctx context.Context, comp string, from, to time.Time) ([]RawFixture, error)
    Results(ctx context.Context, comp string, from, to time.Time) ([]RawFixture, error)
}

type RawFixture struct {
    ProviderID   string
    CompetitionID string
    HomeName, AwayName string
    HomeProviderID, AwayProviderID string
    KickoffAt    time.Time   // always UTC
    Status       domain.MatchStatus
    HomeScore, AwayScore *int  // full-time, 90 minutes only
    Round        *int
}
```

`RawFixture` is the seam. Provider packages translate their JSON into it and
know nothing about the schema; `ingest/sync.go` knows the schema and nothing
about provider JSON. Status vocabularies differ wildly between the two — map
them inside each provider package, and map anything unrecognised to an error
rather than silently to `scheduled`.

**Full-time scores only.** API-Football returns `fulltime`, `extratime` and
`penalty` separately; football-data returns `fullTime` alongside `regularTime`
in knockouts. Take the 90-minute figure. See
[SETTLEMENT.md](SETTLEMENT.md#grading-functions) for why this is load-bearing.

## Budget accounting

Every outbound request passes through `budget.Acquire(ctx, provider)`, which:

1. Blocks on a `golang.org/x/time/rate` limiter sized to the provider
   (football-data: 10/min; API-Football: 100/day smoothed to ~4/hour).
2. Increments `provider_budget (provider, day)` in the same transaction as the
   job, so a crash cannot lose the count and over-spend tomorrow's allowance.
3. Returns `ErrBudgetExhausted` when the day's allocation is gone.

Budget exhaustion is a normal condition, not an error to page on. The job logs,
records what it did not fetch, and exits successfully — tomorrow's run picks up
where it stopped. What *is* worth paging on: predictions due within 6 hours for
fixtures that never got ingested.

Allocation of API-Football's 100 daily requests, by priority:

| Purpose | Budget |
|---|---|
| Results for matches kicked off ≥2h ago | 40 |
| Fixtures for the next 48h | 25 |
| Fixtures for days 3–14 | 20 |
| Team/competition roster refresh | 5 |
| Reserve for manual admin backfill | 10 |

Results outrank fixtures. A missing fixture costs one day's tips on one league;
a missing result leaves published picks ungraded, which is the visible failure.

## Matching and upsert

Provider ids are the join key, never team names. Names differ across providers
and across seasons ("Man United" / "Manchester United FC" / "Manchester Utd").

```
for each RawFixture:
    league  := lookup by (provider, provider_competition_id) → competition_sources
    home    := upsert team by (provider, provider_team_id) → team_sources
    away    := same
    match   := lookup by (provider, ProviderID) → match_sources
               if absent, insert matches + match_sources
    update  matches SET kickoff_at, status, scores, round
            WHERE id = match.id AND (status <> 'finished' OR scores IS NULL)
```

The final guard is important: **a finished match with a score is never
overwritten.** Providers do occasionally re-emit corrected scores, and accepting
a late correction after settlement would change history under an already-graded
prediction. Corrections go through the admin path with an `audit_log` entry, so
somebody owns the decision.

Team upserts fuzzy-match on normalised name *only* when creating a new team in a
league that already has a roster, and log every fuzzy match for review. Silent
duplicate teams poison the strength model — two half-strength "Arsenal" rows
each with half the history.

## Cold start and the honesty problem

Free tiers cap historical seasons, so on day one there is close to no history,
and the Poisson model's strength estimates are near-worthless. Three responses,
all of which should ship:

1. **Seed Europe from CSV.** [football-data.co.uk](https://www.football-data.co.uk/data.php)
   publishes decades of results with closing odds as free CSVs for the major
   European leagues. Load them once through the same upsert path via
   `katafa backfill --csv`. This is not an API and has no rate limit.
2. **Accumulate from day one.** Every result ever fetched stays in Postgres
   permanently. History that free tiers will not sell you is history you grow.
3. **Gate publication on sample size.** A league with fewer than ~40 matches of
   history per team does not get published tips. `match_reasoning.sample_home`
   and `sample_away` already exist to expose this; the frontend renders sample
   size, so the honest state is representable.

Point 3 is the one that will be tempting to skip in order to launch with more
leagues on the page. Publishing confidently-labelled picks from a model with
eight matches of history is exactly the behaviour the accuracy dashboard exists
to expose, and it will expose it — publicly, and permanently.

For East African and Asian leagues there is no CSV archive to seed from, so
those launch in shadow mode: predictions generated and settled internally,
nothing published, until the calibration chart says the model is sane there.

## Reschedules

`sync_fixtures_near` re-pulls the next 36 hours hourly, precisely to catch
kickoff changes. When `kickoff_at` moves:

- Move it. The match row is mutable until it finishes.
- Do **not** touch existing predictions. They were made before the original
  kickoff and therefore before the new one; they remain valid.
- If the new kickoff is *earlier* and now precedes a prediction's `created_at`,
  the trigger's invariant is broken retroactively. Void that prediction, log it,
  and exclude it from accuracy. This is rare and must never be papered over.

That last case is the one nobody anticipates. Write the test for it.

## Failure handling

- Retries: exponential backoff, max 5 attempts, on 5xx and network errors only.
- `429`: honour `Retry-After`, halve the local limiter rate for the rest of the
  day, and record it in `provider_budget`.
- `4xx` other than 429: do not retry. Log, archive the payload, fail the job.
- Every response — success or failure — is archived to `provider_payloads`
  before parsing, so a parser bug is replayable without spending budget.
