import { getDataset } from '@/data/generate';
import { PACKAGES, oddsFromProbability } from '@/data/tipsters';
import { MARKETS } from '@/lib/markets';
import type {
  AccuracyBucket,
  AccuracyPoint,
  AccuracySummary,
  Analyst,
  AnalystRecord,
  FeedFilters,
  FreeTip,
  FreeTipGroup,
  FreeTipsDay,
  League,
  LedgerFilters,
  MatchDetail,
  MatchWithPredictions,
  PackageCode,
  Prediction,
  SettledPrediction,
  Slip,
  SlipWithTips,
  TipPackage,
  TipResult,
} from './types';
import { MARKET_CODES, PACKAGE_CODES } from './types';

/**
 * The frontend's entire data surface.
 *
 * Right now every call is served from the locally generated demo dataset. When
 * the Go API from PRD section 6 exists, only the bodies of these functions
 * change — they are already async and already return the shapes the API will
 * serve, so no calling code has to move.
 */

export async function getLeagues(): Promise<League[]> {
  return getDataset().leagues;
}

function matchesRegion(league: League, region: FeedFilters['region']): boolean {
  return !region || region === 'all' || league.region === region;
}

/** Applies market and confidence filters to a match's predictions. */
function filterPredictions(predictions: Prediction[], filters: FeedFilters): Prediction[] {
  let out = predictions;
  if (filters.markets?.length) {
    const wanted = new Set(filters.markets);
    out = out.filter((p) => wanted.has(p.marketType));
  }
  if (filters.minConfidence !== undefined && filters.minConfidence > 0) {
    out = out.filter((p) => p.confidencePct >= filters.minConfidence!);
  }
  return out;
}

/** Upcoming fixtures with their published predictions (FR-1, FR-3). */
export async function getFeed(filters: FeedFilters = {}): Promise<MatchWithPredictions[]> {
  const db = getDataset();
  const leagueIds = filters.leagueIds?.length ? new Set(filters.leagueIds) : null;

  const out: MatchWithPredictions[] = [];
  for (const match of db.matches) {
    if (match.status !== 'scheduled') continue;
    if (leagueIds && !leagueIds.has(match.leagueId)) continue;

    const league = db.leaguesById.get(match.leagueId)!;
    if (!matchesRegion(league, filters.region)) continue;

    const predictions = filterPredictions(db.predictionsByMatch.get(match.id) ?? [], filters);
    // A fixture with nothing left to show after filtering is not a result.
    if (predictions.length === 0) continue;

    out.push({
      match,
      league,
      homeTeam: db.teamsById.get(match.homeTeamId)!,
      awayTeam: db.teamsById.get(match.awayTeamId)!,
      predictions,
    });
  }
  return out;
}

/** A single fixture with the full model reasoning behind it (FR-4). */
export async function getMatchDetail(matchId: string): Promise<MatchDetail | null> {
  const db = getDataset();
  const match = db.matchesById.get(matchId);
  if (!match) return null;

  const reasoning = db.reasoningByMatch.get(matchId);
  if (!reasoning) return null;

  const predictions = db.predictionsByMatch.get(matchId) ?? [];
  const results = predictions
    .map((p) => db.resultsByPrediction.get(p.id))
    .filter((r): r is NonNullable<typeof r> => Boolean(r));

  return {
    match,
    league: db.leaguesById.get(match.leagueId)!,
    homeTeam: db.teamsById.get(match.homeTeamId)!,
    awayTeam: db.teamsById.get(match.awayTeamId)!,
    predictions,
    results: results.length ? results : undefined,
    reasoning,
  };
}

/* ------------------------------------------------------------------ *
 * Accuracy (FR-5, FR-6)
 * ------------------------------------------------------------------ */

function bucket(key: string, label: string, rows: { wasCorrect: boolean }[]): AccuracyBucket {
  const correct = rows.reduce((n, r) => n + (r.wasCorrect ? 1 : 0), 0);
  return {
    key,
    label,
    total: rows.length,
    correct,
    hitRate: rows.length ? correct / rows.length : 0,
  };
}

/** Exported so the calibration chart can draw each band's stated range. */
export const CONFIDENCE_BANDS: { key: string; label: string; min: number; max: number }[] = [
  { key: 'lt50', label: 'Under 50%', min: 0, max: 50 },
  { key: '50-60', label: '50–60%', min: 50, max: 60 },
  { key: '60-70', label: '60–70%', min: 60, max: 70 },
  { key: '70-80', label: '70–80%', min: 70, max: 80 },
  { key: '80-90', label: '80–90%', min: 80, max: 90 },
  { key: 'gte90', label: '90%+', min: 90, max: 100.01 },
];

interface GradedRow {
  wasCorrect: boolean;
  settledAt: string;
  marketType: (typeof MARKET_CODES)[number];
  leagueId: string;
  confidencePct: number;
}

function gradedRows(): GradedRow[] {
  const db = getDataset();
  const rows: GradedRow[] = [];
  for (const result of db.results) {
    const prediction = db.predictionsById.get(result.predictionId);
    if (!prediction) continue;
    const match = db.matchesById.get(prediction.matchId);
    if (!match) continue;
    rows.push({
      wasCorrect: result.wasCorrect,
      settledAt: result.settledAt,
      marketType: prediction.marketType,
      leagueId: match.leagueId,
      confidencePct: prediction.confidencePct,
    });
  }
  return rows.sort((a, b) => Date.parse(a.settledAt) - Date.parse(b.settledAt));
}

/**
 * The public accuracy dashboard payload. Deliberately computed over *every*
 * graded prediction with no exclusions — see FR-6.
 */
export async function getAccuracySummary(): Promise<AccuracySummary> {
  const db = getDataset();
  const rows = gradedRows();

  const byMarket = MARKET_CODES.map((code) =>
    bucket(code, MARKETS[code].shortName, rows.filter((r) => r.marketType === code)),
  ).filter((b) => b.total > 0);

  const byLeague = db.leagues
    .map((l) => bucket(l.id, l.name, rows.filter((r) => r.leagueId === l.id)))
    .filter((b) => b.total > 0);

  const byConfidenceBand = CONFIDENCE_BANDS.map((band) =>
    bucket(
      band.key,
      band.label,
      rows.filter((r) => r.confidencePct >= band.min && r.confidencePct < band.max),
    ),
  ).filter((b) => b.total > 0);

  // Cumulative hit rate over time, one point per settlement day.
  const byDay = new Map<string, { total: number; correct: number }>();
  for (const r of rows) {
    const day = r.settledAt.slice(0, 10);
    const entry = byDay.get(day) ?? { total: 0, correct: 0 };
    entry.total++;
    if (r.wasCorrect) entry.correct++;
    byDay.set(day, entry);
  }

  let runningTotal = 0;
  let runningCorrect = 0;
  const timeline: AccuracyPoint[] = [...byDay.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, entry]) => {
      runningTotal += entry.total;
      runningCorrect += entry.correct;
      return {
        date,
        cumulativeHitRate: runningCorrect / runningTotal,
        dailyHitRate: entry.correct / entry.total,
        settled: entry.total,
      };
    });

  return {
    overall: bucket('overall', 'All predictions', rows),
    byMarket,
    byLeague,
    byConfidenceBand,
    timeline,
    modelVersion: db.modelVersion,
    firstSettledAt: rows[0]?.settledAt ?? '',
    lastSettledAt: rows[rows.length - 1]?.settledAt ?? '',
  };
}

/**
 * The raw graded ledger — every settled pick, wins and losses alike.
 * This is the receipt behind the headline numbers (FR-7).
 */
export async function getSettledPredictions(
  filters: LedgerFilters = {},
): Promise<SettledPrediction[]> {
  const db = getDataset();
  const leagueIds = filters.leagueIds?.length ? new Set(filters.leagueIds) : null;
  const markets = filters.markets?.length ? new Set(filters.markets) : null;

  const out: SettledPrediction[] = [];
  for (const result of db.results) {
    const prediction = db.predictionsById.get(result.predictionId);
    if (!prediction) continue;
    if (markets && !markets.has(prediction.marketType)) continue;
    if (filters.outcome === 'hit' && !result.wasCorrect) continue;
    if (filters.outcome === 'miss' && result.wasCorrect) continue;

    const match = db.matchesById.get(prediction.matchId);
    if (!match) continue;
    if (leagueIds && !leagueIds.has(match.leagueId)) continue;

    out.push({
      prediction,
      result,
      match,
      league: db.leaguesById.get(match.leagueId)!,
      homeTeam: db.teamsById.get(match.homeTeamId)!,
      awayTeam: db.teamsById.get(match.awayTeamId)!,
    });
  }

  out.sort((a, b) => Date.parse(b.result.settledAt) - Date.parse(a.result.settledAt));
  return filters.limit ? out.slice(0, filters.limit) : out;
}

/* ------------------------------------------------------------------ *
 * Free daily tips
 * ------------------------------------------------------------------ */

/**
 * Below this, a selection is not worth publishing as a tip.
 *
 * Ranking purely on model confidence surfaces things like Double Chance 1X at
 * 1.05 — technically the most likely outcome on the card, and useless to
 * anyone. The floor keeps the shortlist to selections that actually pay.
 */
const MIN_PUBLISHABLE_ODDS = 1.25;

/**
 * Cap on how many times one fixture can carry the shortlist. Without it a
 * single low-scoring match wins every goals market and the page reads as four
 * versions of the same tip.
 */
const MAX_APPEARANCES_PER_FIXTURE = 2;

/**
 * Confidence bands the shortlist is drawn from, safest first.
 *
 * Odds here are derived from the model's own probability, so confidence and
 * price move in lockstep — ranking a market purely by confidence returns four
 * near-identical selections sitting on the odds floor. Taking the best pick out
 * of each band instead gives a ladder from banker to value, which is both more
 * useful to read and more honest about what the model is offering.
 */
const CONFIDENCE_LADDER: [number, number][] = [
  [75, 101],
  [68, 75],
  [60, 68],
  [50, 60],
  [0, 50],
];

/**
 * The free shortlist: the strongest model picks per market for the next
 * matchday. This is what the free tier publishes instead of the whole fixture
 * list — the full feed still lives at /fixtures.
 */
export async function getFreeTips(perMarket = 4): Promise<FreeTipsDay> {
  const db = getDataset();

  const upcoming = db.matches
    .filter((m) => m.status === 'scheduled')
    .sort((a, b) => Date.parse(a.kickoffAt) - Date.parse(b.kickoffAt));

  if (upcoming.length === 0) {
    return { date: '', groups: [], totalTips: 0 };
  }

  // The shortlist covers one matchday: the next one with fixtures on it.
  const day = upcoming[0].kickoffAt.slice(0, 10);
  const dayMatches = upcoming.filter((m) => m.kickoffAt.slice(0, 10) === day);

  const appearances = new Map<string, number>();

  const groups: FreeTipGroup[] = [];
  for (const market of MARKET_CODES) {
    const candidates: FreeTip[] = [];
    for (const match of dayMatches) {
      const prediction = db.predictionsByMatch
        .get(match.id)
        ?.find((p) => p.marketType === market);
      if (!prediction) continue;

      const odds = oddsFromProbability(prediction.confidencePct / 100);
      if (odds < MIN_PUBLISHABLE_ODDS) continue;

      candidates.push({
        match,
        league: db.leaguesById.get(match.leagueId)!,
        homeTeam: db.teamsById.get(match.homeTeamId)!,
        awayTeam: db.teamsById.get(match.awayTeamId)!,
        prediction,
        odds,
      });
    }

    candidates.sort((a, b) => b.prediction.confidencePct - a.prediction.confidencePct);

    const tips: FreeTip[] = [];
    const taken = new Set<string>();

    const claim = (candidate: FreeTip): boolean => {
      const used = appearances.get(candidate.match.id) ?? 0;
      if (used >= MAX_APPEARANCES_PER_FIXTURE) return false;
      appearances.set(candidate.match.id, used + 1);
      taken.add(candidate.prediction.id);
      tips.push(candidate);
      return true;
    };

    // Best available pick from each band, safest band first.
    for (const [min, max] of CONFIDENCE_LADDER) {
      if (tips.length >= perMarket) break;
      const pick = candidates.find(
        (c) =>
          !taken.has(c.prediction.id) &&
          c.prediction.confidencePct >= min &&
          c.prediction.confidencePct < max,
      );
      if (pick) claim(pick);
    }

    // If some bands were empty, backfill on confidence so the market still
    // shows a full shortlist.
    for (const candidate of candidates) {
      if (tips.length >= perMarket) break;
      if (taken.has(candidate.prediction.id)) continue;
      claim(candidate);
    }

    tips.sort((a, b) => b.prediction.confidencePct - a.prediction.confidencePct);
    if (tips.length) groups.push({ market, tips });
  }

  return {
    date: day,
    groups,
    totalTips: groups.reduce((n, g) => n + g.tips.length, 0),
  };
}

/* ------------------------------------------------------------------ *
 * Pro tier
 * ------------------------------------------------------------------ */

export async function getPackages(): Promise<TipPackage[]> {
  return PACKAGE_CODES.map((code) => PACKAGES[code]);
}

export interface SlipQuery {
  packageCode?: PackageCode;
  status?: 'open' | 'settled';
  analystId?: string;
  limit?: number;
}

export async function getSlips(query: SlipQuery = {}): Promise<Slip[]> {
  const { slips } = getDataset().tipsters;
  let out = slips;
  if (query.packageCode) out = out.filter((s) => s.packageCode === query.packageCode);
  if (query.status) out = out.filter((s) => s.status === query.status);
  if (query.analystId) out = out.filter((s) => s.analystIds.includes(query.analystId!));
  return query.limit ? out.slice(0, query.limit) : out;
}

/**
 * A slip and its tips. `ownedSlipIds` decides whether the selections are
 * returned at all — an unpaid viewer must never receive the picks, only the
 * metadata needed to decide whether to buy.
 */
export async function getSlip(
  slipId: string,
  ownedSlipIds: Set<string> = new Set(),
): Promise<SlipWithTips | null> {
  const db = getDataset();
  const { slipsById, tipsBySlip, tipResultsByTip, analystsById } = db.tipsters;
  const slip = slipsById.get(slipId);
  if (!slip) return null;

  // A settled slip's picks are public — that is what makes the analyst record
  // verifiable. Only slips still open are paywalled.
  const unlocked = ownedSlipIds.has(slipId) || slip.status === 'settled';
  const tips = tipsBySlip.get(slipId) ?? [];
  const results = tips
    .map((t) => tipResultsByTip.get(t.id))
    .filter((r): r is NonNullable<typeof r> => Boolean(r));

  return {
    ...slip,
    tips: unlocked ? tips : [],
    results: results.length ? results : undefined,
    analysts: slip.analystIds
      .map((id) => analystsById.get(id))
      .filter((a): a is NonNullable<typeof a> => Boolean(a)),
    package: PACKAGES[slip.packageCode],
    unlocked,
  };
}

export async function getAnalysts(): Promise<Analyst[]> {
  return getDataset().tipsters.analysts;
}

function analystRecordFor(analyst: Analyst): AnalystRecord {
  const db = getDataset();
  const { tipsByAnalyst, tipResultsByTip, slipsById, slips } = db.tipsters;
  const tips = tipsByAnalyst.get(analyst.id) ?? [];

  const settled = tips
    .map((tip) => ({ tip, result: tipResultsByTip.get(tip.id) }))
    .filter((row): row is { tip: (typeof tips)[number]; result: TipResult } =>
      Boolean(row.result),
    );

  const cutoff = Date.now() - 30 * 86_400_000;
  const recent = settled.filter((r) => Date.parse(r.result.settledAt) >= cutoff);

  // Flat 1-unit stakes: a winner returns its odds less the stake, a loser -1.
  let profitUnits = 0;
  let oddsSum = 0;
  for (const { tip, result } of settled) {
    profitUnits += result.wasCorrect ? tip.odds - 1 : -1;
    oddsSum += tip.odds;
  }

  const byPackage = PACKAGE_CODES.map((code) => {
    const rows = settled.filter(
      ({ tip }) => slipsById.get(tip.slipId)?.packageCode === code,
    );
    return bucket(code, PACKAGES[code].name, rows.map((r) => r.result));
  }).filter((b) => b.total > 0);

  return {
    analyst,
    overall: bucket('overall', 'All tips', settled.map((r) => r.result)),
    last30Days: bucket('last30', 'Last 30 days', recent.map((r) => r.result)),
    byPackage,
    profitUnits: Math.round(profitUnits * 100) / 100,
    roi: settled.length ? profitUnits / settled.length : 0,
    averageOdds: settled.length ? oddsSum / settled.length : 0,
    recentSlips: slips.filter((s) => s.analystIds.includes(analyst.id)).slice(0, 6),
  };
}

export async function getAnalystRecord(slug: string): Promise<AnalystRecord | null> {
  const analyst = getDataset().tipsters.analystsBySlug.get(slug);
  return analyst ? analystRecordFor(analyst) : null;
}

/** Every analyst's record, best hit rate first. */
export async function getAnalystLeaderboard(): Promise<AnalystRecord[]> {
  return getDataset()
    .tipsters.analysts.map(analystRecordFor)
    .sort((a, b) => b.overall.hitRate - a.overall.hitRate);
}

export interface CoverageStats {
  leagues: number;
  teams: number;
  upcomingFixtures: number;
  livePredictions: number;
  gradedPredictions: number;
  modelVersion: string;
}

/** Headline counts for the landing strip. */
export async function getCoverageStats(): Promise<CoverageStats> {
  const db = getDataset();
  const upcoming = db.matches.filter((m) => m.status === 'scheduled');
  const graded = db.results.length;
  return {
    leagues: db.leagues.length,
    teams: db.teams.length,
    upcomingFixtures: upcoming.length,
    livePredictions: db.predictions.length - graded,
    gradedPredictions: graded,
    modelVersion: db.modelVersion,
  };
}
