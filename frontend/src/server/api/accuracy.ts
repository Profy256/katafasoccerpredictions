import { query } from '@/server/db';
import { loadConfig } from '@/server/config';
import { newAccuracyBucket, type AccuracyPoint, type AccuracySummary } from '@/server/domain/accuracy';
import type { MarketCode } from '@/server/domain/market';
import { leagues, marketTypes } from '@/server/api/reference';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatRFC3339 } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * The public accuracy dashboard. Port of AccuracySummary in
 * backend/internal/postgres/accuracy.go.
 *
 * Served from accuracy_rollup, not from a live aggregate over
 * prediction_results — computing a hit rate over every settled prediction on
 * each page view is the one query guaranteed to get slower every day the
 * product succeeds.
 *
 * Nothing here filters the underlying set. Every settled prediction is in the
 * rollup — no exclusions, no "excluding postponed", no cherry-picked windows.
 */

/**
 * Mirror of CONFIDENCE_BANDS in src/api/client.ts, indexed by the
 * width_bucket(confidence_pct, 50, 90, 4) value the rollup stores.
 */
const CONFIDENCE_BANDS: { bucket: number; key: string; label: string }[] = [
  { bucket: 0, key: 'lt50', label: 'Under 50%' },
  { bucket: 1, key: '50-60', label: '50–60%' },
  { bucket: 2, key: '60-70', label: '60–70%' },
  { bucket: 3, key: '70-80', label: '70–80%' },
  { bucket: 4, key: '80-90', label: '80–90%' },
  { bucket: 5, key: 'gte90', label: '90%+' },
];

interface Tally {
  total: number;
  correct: number;
}

function add(t: Tally | undefined, total: number, correct: number): Tally {
  return { total: (t?.total ?? 0) + total, correct: (t?.correct ?? 0) + correct };
}

export async function accuracySummary(): Promise<AccuracySummary> {
  const rows = await query<{
    model_version: string;
    market_code: MarketCode;
    league_id: string;
    confidence_band: number;
    settled_day: string;
    total: string;
    correct: string;
  }>(
    `
    SELECT model_version, market_code, league_id, confidence_band, settled_day, total, correct
    FROM accuracy_rollup
    ORDER BY settled_day`,
  );

  let overall: Tally = { total: 0, correct: 0 };
  const byMarket = new Map<string, Tally>();
  const byLeague = new Map<string, Tally>();
  const byBand = new Map<number, Tally>();
  const byDay = new Map<string, Tally>();

  for (const row of rows) {
    const total = Number(row.total);
    const correct = Number(row.correct);
    overall = add(overall, total, correct);
    byMarket.set(row.market_code, add(byMarket.get(row.market_code), total, correct));
    byLeague.set(row.league_id, add(byLeague.get(row.league_id), total, correct));
    byBand.set(row.confidence_band, add(byBand.get(row.confidence_band), total, correct));
    // settled_day is a DATE column and arrives as YYYY-MM-DD — already the
    // key Go's time.DateOnly formatting would produce.
    byDay.set(row.settled_day, add(byDay.get(row.settled_day), total, correct));
  }

  const [marketRows, leagueRows] = await Promise.all([marketTypes(), leagues()]);

  const summary: AccuracySummary = {
    overall: newAccuracyBucket('overall', 'All predictions', overall.total, overall.correct),
    // Every list starts empty rather than null so it serialises as [] — the
    // frontend types all four as arrays and calls .map on them.
    byMarket: [],
    byLeague: [],
    byConfidenceBand: [],
    timeline: [],
    modelVersion: loadConfig().modelVersion,
    firstSettledAt: '',
    lastSettledAt: '',
  };

  // Empty buckets are dropped, matching the frontend's .filter(b => b.total > 0):
  // a market with no settled picks is absent rather than shown at 0%.
  for (const m of marketRows) {
    const k = byMarket.get(m.code);
    if (k && k.total > 0) {
      summary.byMarket.push(newAccuracyBucket(m.code, m.shortName, k.total, k.correct));
    }
  }
  for (const l of leagueRows) {
    const k = byLeague.get(l.id);
    if (k && k.total > 0) {
      summary.byLeague.push(newAccuracyBucket(l.id, l.name, k.total, k.correct));
    }
  }
  // The calibration bands are the ones worth watching weekly: if the 70–80%
  // band does not hit near 70%, the published confidence figures are
  // misleading users and the model needs recalibrating.
  for (const band of CONFIDENCE_BANDS) {
    const k = byBand.get(band.bucket);
    if (k && k.total > 0) {
      summary.byConfidenceBand.push(newAccuracyBucket(band.key, band.label, k.total, k.correct));
    }
  }

  const days = [...byDay.keys()].sort();
  let runningTotal = 0;
  let runningCorrect = 0;
  for (const day of days) {
    const k = byDay.get(day) as Tally;
    runningTotal += k.total;
    runningCorrect += k.correct;
    const point: AccuracyPoint = { date: day, cumulativeHitRate: 0, dailyHitRate: 0, settled: k.total };
    if (runningTotal > 0) point.cumulativeHitRate = runningCorrect / runningTotal;
    if (k.total > 0) point.dailyHitRate = k.correct / k.total;
    summary.timeline.push(point);
  }

  // The window the graded set covers, at full timestamp precision — the
  // rollup only carries days. Go formats these as RFC3339, dropping any
  // fraction; empty stays "" when nothing has been graded yet.
  const window = await query<{ first: string | null; last: string | null }>(
    `SELECT min(settled_at) AS first, max(settled_at) AS last FROM prediction_results`,
  );
  const w = window[0];
  if (w?.first) summary.firstSettledAt = formatRFC3339(w.first);
  if (w?.last) summary.lastSettledAt = formatRFC3339(w.last);

  return summary;
}

export const handleAccuracy: ApiHandler = async (ctx) =>
  jsonResponse(await accuracySummary(), CACHE.accuracy, ctx.request);
