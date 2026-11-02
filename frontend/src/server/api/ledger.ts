import { query } from '@/server/db';
import type {
  League,
  MatchRow,
  PredictionRow,
  SettledPrediction,
  Team,
} from '@/server/domain/readmodel';
import type { APIPredictionResult } from '@/server/domain/readmodel';
import { toAPIMatch, toAPIPrediction } from '@/server/domain/readmodel';
import { leagues, teams } from '@/server/api/reference';
import { encodeLedgerCursor, parseLedgerFilters, type LedgerCursor } from '@/server/api/query';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * The unfiltered graded ledger — every settled pick, wins and losses alike.
 * Port of SettledPredictions in backend/internal/postgres/predictions.go.
 *
 * The outcome filter narrows what a *reader* asked for; it never narrows what
 * the accuracy figures are computed over.
 */

const NIL_UUID = '00000000-0000-0000-0000-000000000000';

const EMPTY_LEAGUE: League = {
  id: NIL_UUID,
  name: '',
  shortName: '',
  country: '',
  countryCode: '',
  tier: 0,
  region: '',
};

const EMPTY_TEAM: Team = {
  id: NIL_UUID,
  leagueId: NIL_UUID,
  name: '',
  shortName: '',
};

interface SettledRow extends PredictionRow {
  r_actual_outcome: string;
  r_was_correct: boolean;
  r_settled_at: string;
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
}

export async function settledPredictions(f: ReturnType<typeof parseLedgerFilters>): Promise<{
  items: SettledPrediction[];
  next: LedgerCursor | null;
}> {
  let limit = f.limit;
  if (limit <= 0 || limit > 200) limit = 50;

  const args: unknown[] = [];
  const where = ['TRUE'];

  if (f.leagueIDs.length > 0) {
    args.push(f.leagueIDs);
    where.push(`m.league_id = ANY($${args.length})`);
  }
  if (f.markets.length > 0) {
    args.push(f.markets);
    where.push(`p.market_code = ANY($${args.length})`);
  }
  if (f.outcome === 'hit') where.push('r.was_correct');
  else if (f.outcome === 'miss') where.push('NOT r.was_correct');

  if (f.cursor !== null) {
    args.push(f.cursor.settledAt, f.cursor.id);
    where.push(`(r.settled_at, p.id) < ($${args.length - 1}, $${args.length})`);
  }
  args.push(limit + 1);

  // Aliased p_/m_/r_: node-pg keys rows by column name, and p.id plus m.id
  // would collapse onto one key — Go's positional Scan has no such collision.
  const rows = await query<SettledRow>(
    `
    SELECT p.id, p.match_id, p.market_code, p.prediction_value,
           p.confidence_pct::float8, p.distribution, p.model_version, p.created_at,
           r.actual_outcome AS r_actual_outcome, r.was_correct AS r_was_correct,
           r.settled_at AS r_settled_at,
           m.id AS m_id, m.league_id AS m_league_id, m.season_id AS m_season_id,
           m.home_team_id AS m_home_team_id, m.away_team_id AS m_away_team_id,
           m.kickoff_at AS m_kickoff_at, m.status AS m_status,
           m.home_score AS m_home_score, m.away_score AS m_away_score,
           m.round AS m_round
    FROM prediction_results r
    JOIN predictions p ON p.id = r.prediction_id
    JOIN matches     m ON m.id = p.match_id
    WHERE ${where.join(' AND ')}
    ORDER BY r.settled_at DESC, p.id DESC
    LIMIT $${args.length}`,
    args,
  );

  const [leagueRows, teamRows] = await Promise.all([leagues(), teams()]);
  const leagueByID = new Map(leagueRows.map((l) => [l.id, l]));
  const teamByID = new Map(teamRows.map((t) => [t.id, t]));

  const items: SettledPrediction[] = [];
  let next: LedgerCursor | null = null;

  for (const row of rows) {
    // The extra row proves there is another page without a second query.
    if (items.length === limit) {
      next = { settledAt: formatTimestamptz(row.r_settled_at), id: row.id };
      break;
    }

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
    const result: APIPredictionResult = {
      predictionId: row.id,
      actualOutcome: row.r_actual_outcome,
      wasCorrect: row.r_was_correct,
      settledAt: formatTimestamptz(row.r_settled_at),
    };

    items.push({
      prediction: toAPIPrediction(row),
      result,
      match: toAPIMatch(match),
      league: leagueByID.get(row.m_league_id) ?? EMPTY_LEAGUE,
      homeTeam: teamByID.get(row.m_home_team_id) ?? EMPTY_TEAM,
      awayTeam: teamByID.get(row.m_away_team_id) ?? EMPTY_TEAM,
    });
  }

  // The cursor points at the last row *returned*, not the lookahead row.
  if (next !== null && items.length > 0) {
    const last = items[items.length - 1];
    next = { settledAt: last.result.settledAt, id: last.prediction.id };
  }
  return { items, next };
}

export const handleSettledPredictions: ApiHandler = async (ctx) => {
  const filters = parseLedgerFilters(ctx.url);
  const { items, next } = await settledPredictions(filters);
  const body: { items: SettledPrediction[]; nextCursor?: string } = { items };
  if (next !== null) body.nextCursor = encodeLedgerCursor(next);
  return jsonResponse(body, CACHE.accuracy, ctx.request);
};
