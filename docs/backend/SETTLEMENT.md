# Settlement

Settlement turns a final score into a graded record. It is the only part of the
system whose output is a claim about the past, so it is the part most worth
being strict about.

**Settlement never involves a model, a heuristic, or an LLM.** It reads
`home_score` and `away_score` from a `finished` match and applies a pure
function. If the score is unknown, nothing is graded — the pick stays pending.
There is no such thing as an inferred result.

## Grading functions

All in `internal/settle/grade.go`. Signature:

```go
// Grade returns the outcome the market settled to for the given final score.
// It is total over its domain: every (market, score) pair has an outcome.
func Grade(market domain.MarketCode, home, away int) (outcome string, err error)
```

Deliberately *not* taking the prediction. It computes what actually happened;
comparing that to the prediction is the caller's one-line job. Passing the
prediction in invites a grader that quietly agrees with the pick.

| Market | Outcome |
|---|---|
| `ONE_X_TWO` | `home > away` → `HOME`; `home < away` → `AWAY`; else `DRAW` |
| `DOUBLE_CHANCE` | `HOME`→`1X` and `12`; `DRAW`→`1X` and `X2`; `AWAY`→`12` and `X2`. A DC selection wins if the 1X2 result is in its pair |
| `BTTS` | `home > 0 && away > 0` → `YES` else `NO` |
| `OVER_UNDER_1_5` | `home+away >= 2` → `OVER` else `UNDER` |
| `OVER_UNDER_2_5` | `home+away >= 3` → `OVER` else `UNDER` |
| `OVER_UNDER_3_5` | `home+away >= 4` → `OVER` else `UNDER` |

Double Chance is the one that does not fit the "one outcome per market" shape —
its three selections overlap, so two of them win on every result. Grade it as
membership rather than equality:

```go
func doubleChanceWins(selection string, home, away int) bool {
    switch result := oneXTwo(home, away); selection {
    case "1X": return result == "HOME" || result == "DRAW"
    case "12": return result == "HOME" || result == "AWAY"
    case "X2": return result == "DRAW" || result == "AWAY"
    }
    return false
}
```

The over/under lines are half-goal, so a push is impossible and every selection
resolves to won or lost. Do not add push handling for these; add it only if a
whole-number line (Over 2.0, Asian handicap) is ever introduced.

**Goals are full-time only.** Extra time and penalties are excluded — a cup tie
level at 90 minutes settles as `DRAW` on 1X2. Providers report the 90-minute
score separately from aggregate; ingestion must store the 90-minute figure and
nothing else. Getting this wrong corrupts every goals market in knockout rounds.

## Void handling

A match that never produced a full-time score cannot be graded.

| Match status | Prediction | Tip on a slip |
|---|---|---|
| `postponed` | stays pending until the rescheduled fixture finishes | tip voids, odds removed from the accumulator |
| `cancelled` | voided, excluded from accuracy | tip voids, odds removed |
| `abandoned` | voided unless the competition awards a result | tip voids, odds removed |

Voiding is not the same as excluding a loss. A voided pick has no outcome to
report; a lost pick does. Non-negotiable 5 forbids the second, not the first.
Every void is written to `audit_log` with its reason, so the gap between
"predictions made" and "predictions settled" is always explainable.

Postponements are the common case and are handled by *waiting*, not voiding: the
provider reschedules the same fixture id, ingestion updates `kickoff_at`, and the
prediction — made before the original kickoff, therefore before the new one —
settles when the match is eventually played. A prediction is never re-made for a
rescheduled fixture, because that would be a post-hoc pick with fresher
information.

## Settling predictions

`settle_predictions` runs every 30 minutes after `sync_results`.

```sql
SELECT p.id, p.market_code, p.prediction_value, m.home_score, m.away_score
FROM predictions p
JOIN matches m ON m.id = p.match_id
LEFT JOIN prediction_results r ON r.prediction_id = p.id
WHERE m.status = 'finished'
  AND r.prediction_id IS NULL
FOR UPDATE OF p SKIP LOCKED
LIMIT 500;
```

For each row: `Grade`, compare, `INSERT INTO prediction_results`. The insert is
`ON CONFLICT (prediction_id) DO NOTHING` — two workers racing produce one row,
not two, and never a double-counted win.

The whole batch runs in one transaction with the River job completion, so a
crash mid-batch replays cleanly rather than leaving half a matchday graded.

## Settling slips

A slip settles only when every tip on it has resolved.

```
for each tip:
    auto-gradable (match_id + market_code + selection_value all present)
        → grade from the match score, same function as predictions
    free text
        → wait for an admin decision; never guess

slip settles when no tip is still pending:
    won_tips  = count of tips with was_correct
    status    = 'settled'
    settled_at = now()
```

Slip-level outcome follows accumulator convention: the slip wins only if every
non-void tip won. `won_tips` is stored so the frontend can render "4 of 5" on a
losing slip without recounting.

**Void tips reduce the accumulator.** If a tip voids, it is removed from
`total_odds` — the remaining tips still stand and the slip settles on them. This
is the standard bookmaker rule and the only one that is fair to a buyer whose
match was called off. Recompute `total_odds` as the product of surviving tips'
odds; because `slips.total_odds` is frozen after publication, write the adjusted
figure to a separate `settled_odds` column rather than mutating the published
one, and show both.

If *every* tip voids, the slip becomes `void` and the purchase is refunded — see
[PAYMENTS.md § Refunds](PAYMENTS.md#refunds).

## Admin settlement

Free-text tips need a human. The endpoint is
`POST /v1/admin/tips/{id}/settle` with `{was_correct, actual_outcome, reason}`.

Rules:

- `settled_by = 'admin'` and `settled_by_user` is mandatory.
- Writes `tip_results` **and** an `audit_log` row in one transaction.
- Cannot settle a tip whose `kickoff_at` is in the future.
- Cannot re-settle: `tip_results` is immutable and primary-keyed on `tip_id`.
- A genuine mistake is corrected by a compensating `audit_log` entry and a
  public correction note on the slip, not by editing the result.

That last rule will feel obnoxious the first time someone fat-fingers a grade.
It is the rule that makes the analyst leaderboard mean anything.

## Why the free shortlist is frozen

`getFreeTips` in `src/api/client.ts` selects the day's shortlist live: it filters
scheduled matches, applies the 1.25 odds floor, walks the confidence ladder, and
returns the result. That is correct for a demo and wrong for a backend.

Computed live, the shortlist is a function of *current* data. Once matches
finish they stop being `scheduled`, so yesterday's shortlist cannot be
reconstructed — and if it could, a model rerun or a late odds change would
reconstruct a *different* one. "Yesterday's tips won" would be a claim about a
list that no longer exists.

So `publish_free_tips` runs at 05:00, performs exactly the selection logic
`getFreeTips` implements, and writes the outcome to `free_tip_days` +
`free_tips`. After that the list is a fact. The API reads those rows; it never
re-derives them.

Port the selection constants verbatim — `MIN_PUBLISHABLE_ODDS = 1.25`,
`MAX_APPEARANCES_PER_FIXTURE = 2`, and the `CONFIDENCE_LADDER` bands — into
`internal/tips`. The frontend comments explain why each exists; keep those
comments in the Go port. When the backend is live, `getFreeTips` becomes a
`fetch` and the selection code is deleted from the frontend, not left to drift.

This also answers the question the free tier is built on: showing a user "we
went 4 from 5 yesterday" requires a row that says what yesterday's five were,
written before those matches kicked off.

## Accuracy

`accuracy_rollup` aggregates every row in `prediction_results`. No filters, no
exclusions, no date windows applied at write time — the API may window a *query*
over it for the timeline chart, but the underlying set is complete.

Calibration bands come from `width_bucket(confidence_pct, 50, 90, 4)`, matching
the frontend's `CONFIDENCE_BANDS`. The calibration chart is the one that catches
model rot: if the 70–80% band hits 55% of the time over a few hundred settled
picks, the model is overconfident and the published confidence figures are
misleading users. Alert on it rather than waiting to notice.
