import { query, queryOne } from '@/server/db';
import { AppError } from '@/server/domain/errors';
import type { League, MatchRow, PredictionRow, Team } from '@/server/domain/readmodel';
import { toAPIMatch, toAPIPrediction } from '@/server/domain/readmodel';
import type { MarketCode } from '@/server/domain/market';
import type { APIPredictionResult } from '@/server/domain/readmodel';
import { leagues, teams } from '@/server/api/reference';
import { parseDateOnly } from '@/server/api/query';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * The daily free shortlist and its history. Port of
 * backend/internal/postgres/freetips.go.
 *
 * Everything here reads free_tips. Nothing re-derives the shortlist: the
 * selection ran once, at publish time, and was frozen. Re-deriving it on read
 * would make "yesterday's tips went 4 from 5" a claim about a list that no
 * longer exists.
 */

interface FreeTipGroup {
  market: MarketCode;
  tips: FreeTip[];
}

interface FreeTip {
  match: ReturnType<typeof toAPIMatch>;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
  prediction: ReturnType<typeof toAPIPrediction>;
  /** Indicative, derived from the model price — decimal, but a number on the wire. */
  odds: number;
  /** Present once the pick has been graded, so history can show frozen results. */
  result?: APIPredictionResult;
}

export interface FreeTipsDay {
  date: string;
  groups: FreeTipGroup[];
  totalTips: number;
}

export interface FreeTipsHistoryDay {
  date: string;
  totalTips: number;
  settled: number;
  correct: number;
  hitRate: number;
  /** Missing groups serialise as JSON null in Go — a nil map lookup. */
  groups: FreeTipGroup[] | null;
}

/**
 * The most recently published shortlist date, or null when nothing has been
 * published yet.
 */
async function latestFreeTipsDay(): Promise<string | null> {
  const row = await queryOne<{ day: string }>(
    `SELECT day FROM free_tip_days ORDER BY day DESC LIMIT 1`,
  );
  if (!row) throw AppError.notFound();
  return row.day;
}

interface FreeTipRow {
  day: string;
  market_code: MarketCode;
  odds: number;
  rank: number;
  p_id: string;
  p_match_id: string;
  p_market_code: MarketCode;
  p_prediction_value: string;
  p_confidence_pct: number;
  p_distribution: PredictionRow['distribution'];
  p_model_version: string;
  p_created_at: string;
  m_id: string;
  m_league_id: string;
  m_season_id: string;
  m_home_team_id: string;
  m_away_team_id: string;
  m_kickoff_at: string;
  m_status: string;
  m_home_score: number | null;
  m_away_score: number | null;
  m_round: number | null;
  r_actual_outcome: string | null;
  r_was_correct: boolean | null;
  r_settled_at: string | null;
}

/**
 * Loads the frozen tips for a date range, with results attached where the
 * pick has been graded. Groups are keyed by date; `settled`/`correct` count
 * graded tips per day for the history view.
 */
async function freeTipGroups(from: string, to: string): Promise<{
  groups: Map<string, FreeTipGroup[]>;
  settled: Map<string, number>;
  correct: Map<string, number>;
}> {
  // Every column is aliased: node-pg keys rows by column name, and p.id
  // plus m.id would otherwise collapse onto one key — Go's positional Scan
  // has no such collision, so the aliases exist purely to preserve its view
  // of the row.
  const rows = await query<FreeTipRow>(
    `
    SELECT ft.day, ft.market_code, ft.odds::float8 AS odds, ft.rank,
           p.id AS p_id, p.match_id AS p_match_id, p.market_code AS p_market_code,
           p.prediction_value AS p_prediction_value,
           p.confidence_pct::float8 AS p_confidence_pct,
           p.distribution AS p_distribution, p.model_version AS p_model_version,
           p.created_at AS p_created_at,
           m.id AS m_id, m.league_id AS m_league_id, m.season_id AS m_season_id,
           m.home_team_id AS m_home_team_id, m.away_team_id AS m_away_team_id,
           m.kickoff_at AS m_kickoff_at, m.status AS m_status,
           m.home_score AS m_home_score, m.away_score AS m_away_score,
           m.round AS m_round,
           r.actual_outcome AS r_actual_outcome, r.was_correct AS r_was_correct,
           r.settled_at AS r_settled_at
    FROM free_tips ft
    JOIN predictions p ON p.id = ft.prediction_id
    JOIN matches     m ON m.id = p.match_id
    JOIN market_types mt ON mt.code = ft.market_code
    LEFT JOIN prediction_results r ON r.prediction_id = p.id
    WHERE ft.day BETWEEN $1 AND $2
    ORDER BY ft.day DESC, mt.sort_order, ft.rank`,
    [from, to],
  );

  const [leagueRows, teamRows] = await Promise.all([leagues(), teams()]);
  const leagueByID = new Map(leagueRows.map((l) => [l.id, l]));
  const teamByID = new Map(teamRows.map((t) => [t.id, t]));

  const groups = new Map<string, FreeTipGroup[]>();
  const settled = new Map<string, number>();
  const correct = new Map<string, number>();

  for (const row of rows) {
    const key = row.day;

    const match: MatchRow = {
      id: row.m_id,
      league_id: row.m_league_id,
      season_id: row.m_season_id,
      home_team_id: row.m_home_team_id,
      away_team_id: row.m_away_team_id,
      kickoff_at: row.m_kickoff_at,
      status: row.m_status,
      home_score: row.m_home_score,
      away_score: row.m_away_score,
      round: row.m_round,
    };
    const predictionRow: PredictionRow = {
      id: row.p_id,
      match_id: row.p_match_id,
      market_code: row.p_market_code,
      prediction_value: row.p_prediction_value,
      confidence_pct: row.p_confidence_pct,
      distribution: row.p_distribution,
      model_version: row.p_model_version,
      created_at: row.p_created_at,
    };

    const tip: FreeTip = {
      match: toAPIMatch(match),
      league: leagueByID.get(row.m_league_id) ?? EMPTY_LEAGUE,
      homeTeam: teamByID.get(row.m_home_team_id) ?? EMPTY_TEAM,
      awayTeam: teamByID.get(row.m_away_team_id) ?? EMPTY_TEAM,
      prediction: toAPIPrediction(predictionRow),
      odds: row.odds,
    };
    if (row.r_was_correct !== null && row.r_actual_outcome !== null && row.r_settled_at !== null) {
      tip.result = {
        predictionId: row.p_id,
        actualOutcome: row.r_actual_outcome,
        wasCorrect: row.r_was_correct,
        settledAt: formatTimestamptz(row.r_settled_at),
      };
      settled.set(key, (settled.get(key) ?? 0) + 1);
      if (row.r_was_correct) correct.set(key, (correct.get(key) ?? 0) + 1);
    }

    // Rows arrive ordered by market, so the last group is the right one
    // until the market changes.
    const list = groups.get(key) ?? [];
    if (list.length === 0 || list[list.length - 1].market !== row.market_code) {
      list.push({ market: row.market_code, tips: [] });
    }
    list[list.length - 1].tips.push(tip);
    groups.set(key, list);
  }

  return { groups, settled, correct };
}

const EMPTY_LEAGUE: League = {
  id: '00000000-0000-0000-0000-000000000000',
  name: '',
  shortName: '',
  country: '',
  countryCode: '',
  tier: 0,
  region: '',
};

const EMPTY_TEAM: Team = {
  id: '00000000-0000-0000-0000-000000000000',
  leagueId: '00000000-0000-0000-0000-000000000000',
  name: '',
  shortName: '',
};

/** One published day, grouped by market in display order. */
async function freeTips(day: string): Promise<FreeTipsDay> {
  const dayRow = await queryOne<{ total_tips: number }>(
    `SELECT total_tips FROM free_tip_days WHERE day = $1`,
    [day],
  );
  if (!dayRow) throw AppError.notFound();

  const { groups } = await freeTipGroups(day, day);
  return {
    date: day,
    groups: groups.get(day) ?? [],
    totalTips: dayRow.total_tips,
  };
}

export const handleFreeTips: ApiHandler = async (ctx) => {
  const rawDate = ctx.url.searchParams.get('date') ?? '';

  let day: string;
  if (rawDate !== '') {
    const parsed = parseDateOnly(rawDate);
    if (parsed === null) throw AppError.invalid('date must be YYYY-MM-DD');
    day = parsed;
  } else {
    // Omitted means the current published day. The latest frozen row is the
    // answer; there is no live selection to fall back on.
    try {
      day = (await latestFreeTipsDay()) as string;
    } catch (error) {
      if (error instanceof AppError && error.kind === 'not-found') {
        // No day published yet: an empty shortlist, not a 404.
        return jsonResponse(
          { date: '', groups: [], totalTips: 0 },
          CACHE.freeTips,
          ctx.request,
        );
      }
      throw error;
    }
  }

  return jsonResponse(await freeTips(day), CACHE.freeTips, ctx.request);
};

/**
 * "Yesterday's free tips went 4 from 5": the frozen shortlist joined to its
 * results. The default window is the last 14 days ending today.
 */
export const handleFreeTipsHistory: ApiHandler = async (ctx) => {
  const query0 = ctx.url.searchParams;

  const now = new Date();
  let to = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, '0')}-${String(now.getUTCDate()).padStart(2, '0')}`;
  let from = shiftDays(to, -13);

  const rawFrom = query0.get('from') ?? '';
  if (rawFrom !== '') {
    const parsed = parseDateOnly(rawFrom);
    if (parsed === null) throw AppError.invalid('from must be YYYY-MM-DD');
    from = parsed;
  }
  const rawTo = query0.get('to') ?? '';
  if (rawTo !== '') {
    const parsed = parseDateOnly(rawTo);
    if (parsed === null) throw AppError.invalid('to must be YYYY-MM-DD');
    to = parsed;
  }
  if (to < from) throw AppError.invalid('to is before from');

  const days = await freeTipsHistory(from, to);
  return jsonResponse({ days }, CACHE.reference, ctx.request);
};

function shiftDays(date: string, delta: number): string {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + delta);
  return d.toISOString().slice(0, 10);
}

async function freeTipsHistory(from: string, to: string): Promise<FreeTipsHistoryDay[]> {
  const rows = await query<{ day: string; total_tips: number }>(
    `
    SELECT day, total_tips FROM free_tip_days
    WHERE day BETWEEN $1 AND $2
    ORDER BY day DESC`,
    [from, to],
  );
  if (rows.length === 0) return [];

  const { groups, settled, correct } = await freeTipGroups(from, to);

  const out: FreeTipsHistoryDay[] = [];
  for (const row of rows) {
    const key = row.day;
    const entry: FreeTipsHistoryDay = {
      date: key,
      totalTips: row.total_tips,
      settled: settled.get(key) ?? 0,
      correct: correct.get(key) ?? 0,
      hitRate: 0,
      // A day frozen without graded tips yet keeps a nil groups in Go —
      // a map miss, not an empty array.
      groups: groups.get(key) ?? null,
    };
    if (entry.settled > 0) {
      entry.hitRate = entry.correct / entry.settled;
    }
    out.push(entry);
  }
  return out;
}
