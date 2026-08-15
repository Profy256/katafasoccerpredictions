-- +goose Up

-- Computing a hit rate over every settled prediction on each page view is the
-- one query guaranteed to get slower every day the product succeeds. These are
-- refreshed after settlement, not per request.
--
-- No WHERE clause filters anything out. Every settled prediction is in here,
-- which is the point: accuracy is computed over the complete set, with no
-- exclusions, no "excluding postponed", and no cherry-picked windows.
CREATE MATERIALIZED VIEW accuracy_rollup AS
SELECT p.model_version,
       p.market_code,
       m.league_id,
       -- Matches CONFIDENCE_BANDS in src/api/client.ts: bucket 0 is under 50,
       -- then four 10-point bands, then 5 for 90+.
       width_bucket(p.confidence_pct, 50, 90, 4) AS confidence_band,
       date_trunc('day', r.settled_at)::date     AS settled_day,
       count(*)                                  AS total,
       count(*) FILTER (WHERE r.was_correct)     AS correct
FROM prediction_results r
JOIN predictions p ON p.id = r.prediction_id
JOIN matches     m ON m.id = p.match_id
GROUP BY 1,2,3,4,5;

-- Required for REFRESH MATERIALIZED VIEW CONCURRENTLY, which is what keeps the
-- accuracy page readable during a refresh.
CREATE UNIQUE INDEX ON accuracy_rollup
    (model_version, market_code, league_id, confidence_band, settled_day);

CREATE MATERIALIZED VIEW analyst_rollup AS
SELECT t.analyst_id,
       s.package_code,
       date_trunc('day', r.settled_at)::date  AS settled_day,
       count(*)                               AS total,
       count(*) FILTER (WHERE r.was_correct)  AS correct,
       sum(t.odds)                            AS odds_sum,
       -- Flat 1-unit stakes, matching the frontend's definition: a winner
       -- returns its odds less the stake, a loser -1.
       COALESCE(sum(t.odds - 1) FILTER (WHERE r.was_correct), 0)
         - count(*) FILTER (WHERE NOT r.was_correct)  AS profit_units
FROM tip_results r
JOIN tips  t ON t.id = r.tip_id
JOIN slips s ON s.id = t.slip_id
WHERE r.actual_outcome <> 'VOID'   -- a void has no outcome to report
GROUP BY 1,2,3;

CREATE UNIQUE INDEX ON analyst_rollup (analyst_id, package_code, settled_day);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS analyst_rollup;
DROP MATERIALIZED VIEW IF EXISTS accuracy_rollup;
