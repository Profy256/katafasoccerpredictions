import { query, queryOne } from '@/server/db';
import { loadConfig } from '@/server/config';
import { newAccuracyBucket } from '@/server/domain/accuracy';
import { AppError } from '@/server/domain/errors';
import type { AnalystRecord, PackageCode } from '@/server/domain/pro';
import { listSlips } from '@/server/api/slips';
import { analysts, packages } from '@/server/api/pro';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * The analyst leaderboard and the landing-page coverage counters. Port of
 * AnalystRecords/AnalystRecord in backend/internal/postgres/analysts.go and
 * CoverageStats in reference.go.
 */

/**
 * Flat-1-unit rounding, matching the frontend's Math.round(profitUnits * 100)
 * / 100 so the figure shown does not change at cutover. Go truncates toward
 * zero after adding a sign-corrected 0.5; Math.trunc is the same rule.
 */
function round2(v: number): number {
  const sign = v < 0 ? -0.5 : 0.5;
  return Math.trunc(v * 100 + sign) / 100;
}

interface Tally {
  total: number;
  correct: number;
  profitUnits: number;
  oddsSum: number;
}

function emptyTally(): Tally {
  return { total: 0, correct: 0, profitUnits: 0, oddsSum: 0 };
}

/**
 * Every active analyst's record, best hit rate first.
 *
 * An analyst's record is computed over every settled tip, with no exclusions.
 * Void legs are excluded from the rollup because a void has no outcome to
 * report — that is not the same as dropping a loss, which would be.
 */
export async function analystRecords(): Promise<AnalystRecord[]> {
  const roster = await analysts();
  if (roster.length === 0) return [];

  const pkgRows = await packages();
  const packageName = new Map<PackageCode, string>(pkgRows.map((p) => [p.code, p.name]));

  // Read the settled tips directly rather than the rollup: the record needs
  // per-tip odds for the flat-stake return, and the analyst set is small.
  const rows = await query<{
    analyst_id: string;
    package_code: PackageCode;
    was_correct: boolean;
    settled_at: string;
    odds: number;
  }>(
    `
    SELECT t.analyst_id, s.package_code, r.was_correct, r.settled_at, t.odds::float8
    FROM tip_results r
    JOIN tips  t ON t.id = r.tip_id
    JOIN slips s ON s.id = t.slip_id
    WHERE r.actual_outcome <> $1`,
    ['VOID'],
  );

  const overall = new Map<string, Tally>();
  const last30 = new Map<string, Tally>();
  const byPackage = new Map<string, Map<PackageCode, Tally>>();
  const get = (m: Map<string, Tally>, id: string): Tally => {
    let t = m.get(id);
    if (!t) {
      t = emptyTally();
      m.set(id, t);
    }
    return t;
  };

  const cutoffMs = Date.now() - 30 * 24 * 60 * 60 * 1000;

  for (const row of rows) {
    const o = get(overall, row.analyst_id);
    o.total++;
    o.oddsSum += row.odds;
    if (row.was_correct) {
      o.correct++;
      // Flat 1-unit stakes: a winner returns its odds less the stake, a
      // loser -1.
      o.profitUnits += row.odds - 1;
    } else {
      o.profitUnits--;
    }

    if (Date.parse(formatTimestamptz(row.settled_at)) > cutoffMs) {
      const r = get(last30, row.analyst_id);
      r.total++;
      if (row.was_correct) r.correct++;
    }

    let pkgMap = byPackage.get(row.analyst_id);
    if (!pkgMap) {
      pkgMap = new Map();
      byPackage.set(row.analyst_id, pkgMap);
    }
    let p = pkgMap.get(row.package_code);
    if (!p) {
      p = emptyTally();
      pkgMap.set(row.package_code, p);
    }
    p.total++;
    if (row.was_correct) p.correct++;
  }

  const out: AnalystRecord[] = [];
  for (const analyst of roster) {
    const o = overall.get(analyst.id) ?? emptyTally();
    const r = last30.get(analyst.id) ?? emptyTally();

    const record: AnalystRecord = {
      analyst,
      overall: newAccuracyBucket('overall', 'All tips', o.total, o.correct),
      last30Days: newAccuracyBucket('last30', 'Last 30 days', r.total, r.correct),
      byPackage: [],
      profitUnits: round2(o.profitUnits),
      roi: 0,
      averageOdds: 0,
      recentSlips: [],
    };
    if (o.total > 0) {
      record.roi = o.profitUnits / o.total;
      record.averageOdds = o.oddsSum / o.total;
    }

    const pkgMap = byPackage.get(analyst.id);
    if (pkgMap) {
      for (const code of ['ordinary', 'vip', 'akatambula'] as PackageCode[]) {
        const t = pkgMap.get(code);
        if (t && t.total > 0) {
          record.byPackage.push(
            newAccuracyBucket(code, packageName.get(code) ?? '', t.total, t.correct),
          );
        }
      }
    }

    record.recentSlips = await listSlips({
      packageCode: '',
      status: '',
      analystID: analyst.id,
      limit: 6,
      cursor: null,
    });
    out.push(record);
  }

  // Stable: equal hit rates keep the name order the roster came in (by name).
  out.sort((a, b) => b.overall.hitRate - a.overall.hitRate);
  return out;
}

export async function analystRecord(slug: string): Promise<AnalystRecord> {
  const all = await analystRecords();
  for (const record of all) {
    if (record.analyst.slug === slug) return record;
  }
  throw AppError.notFound();
}

export const handleAnalystLeaderboard: ApiHandler = async (ctx) =>
  jsonResponse(await analystRecords(), CACHE.accuracy, ctx.request);

export const handleAnalystRecord: ApiHandler = async (ctx) =>
  jsonResponse(await analystRecord(ctx.params.slug ?? ''), CACHE.accuracy, ctx.request);

export async function coverageStats(): Promise<{
  leagues: number;
  teams: number;
  upcomingFixtures: number;
  livePredictions: number;
  gradedPredictions: number;
  modelVersion: string;
}> {
  // Each subquery is aliased: PG would otherwise name all five output
  // columns "count", which node-pg collapses onto a single key.
  const row = await queryOne<{
    leagues: string;
    teams: string;
    upcoming: string;
    live: string;
    graded: string;
  }>(
    `
    SELECT
      (SELECT count(*) FROM leagues WHERE is_active)                       AS leagues,
      (SELECT count(*) FROM teams)                                         AS teams,
      (SELECT count(*) FROM matches WHERE status IN ('scheduled','in_play')) AS upcoming,
      -- Live predictions are those still awaiting a result. A voided pick is
      -- neither live nor graded, so it is in neither figure.
      (SELECT count(*) FROM predictions p
         LEFT JOIN prediction_results r ON r.prediction_id = p.id
         LEFT JOIN prediction_voids   v ON v.prediction_id = p.id
        WHERE r.prediction_id IS NULL AND v.prediction_id IS NULL)         AS live,
      (SELECT count(*) FROM prediction_results)                            AS graded`,
  );
  return {
    leagues: Number(row?.leagues ?? 0),
    teams: Number(row?.teams ?? 0),
    upcomingFixtures: Number(row?.upcoming ?? 0),
    livePredictions: Number(row?.live ?? 0),
    gradedPredictions: Number(row?.graded ?? 0),
    modelVersion: loadConfig().modelVersion,
  };
}

export const handleCoverage: ApiHandler = async (ctx) =>
  jsonResponse(await coverageStats(), CACHE.stats, ctx.request);
