import { query } from '@/server/db';
import { AppError } from '@/server/domain/errors';
import type {
  APIPredictionResult,
  League,
  MatchDetail,
  MatchReasoning,
  MatchRow,
  MatchWithPredictions,
  PredictionRow,
  Team,
} from '@/server/domain/readmodel';
import { toAPIMatch, toAPIPrediction } from '@/server/domain/readmodel';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';
import { leagues, teams } from '@/server/api/reference';
import { parseFeedFilters, parseUUID } from '@/server/api/query';

/**
 * The feed and match detail. Port of backend/internal/postgres/matches.go and
 * handlers_public.go — including the rule that only leagues cleared for
 * publication appear: a league still accumulating history runs in shadow mode,
 * its predictions generated and settled internally but never shown.
 */

async function scanMatches(sql: string, params: unknown[]): Promise<MatchRow[]> {
  return query<MatchRow>(sql, params);
}

async function predictionsForMatches(
  matchIDs: string[],
  markets: string[],
  minConfidence: number,
): Promise<Map<string, PredictionRow[]>> {
  const args: unknown[] = [matchIDs];
  const where = ['p.match_id = ANY($1)'];

  if (markets.length > 0) {
    args.push(markets);
    where.push(`p.market_code = ANY($${args.length})`);
  }
  if (minConfidence > 0) {
    args.push(minConfidence);
    where.push(`p.confidence_pct >= $${args.length}`);
  }

  const rows = await query<PredictionRow>(
    `
    SELECT p.id, p.match_id, p.market_code, p.prediction_value,
           p.confidence_pct::float8, p.distribution, p.model_version, p.created_at
    FROM predictions p
    JOIN market_types mt ON mt.code = p.market_code
    WHERE ${where.join(' AND ')}
    ORDER BY p.match_id, mt.sort_order, p.created_at`,
    args,
  );

  const out = new Map<string, PredictionRow[]>();
  for (const row of rows) {
    const list = out.get(row.match_id);
    if (list) list.push(row);
    else out.set(row.match_id, [row]);
  }
  return out;
}

async function resultsForMatch(matchID: string): Promise<APIPredictionResult[]> {
  const rows = await query<{
    prediction_id: string;
    actual_outcome: string;
    was_correct: boolean;
    settled_at: string;
  }>(
    `
    SELECT r.prediction_id, r.actual_outcome, r.was_correct, r.settled_at
    FROM prediction_results r
    JOIN predictions p ON p.id = r.prediction_id
    WHERE p.match_id = $1
    ORDER BY r.settled_at`,
    [matchID],
  );
  return rows.map((row) => ({
    predictionId: row.prediction_id,
    actualOutcome: row.actual_outcome,
    wasCorrect: row.was_correct,
    settledAt: formatTimestamptz(row.settled_at),
  }));
}

/**
 * Loads leagues and teams keyed by id. Both tables are small and
 * slow-changing, so one round trip each beats joining them onto every row.
 */
async function referenceMaps(): Promise<{
  leagueByID: Map<string, League>;
  teamByID: Map<string, Team>;
}> {
  const [leagueRows, teamRows] = await Promise.all([leagues(), teams()]);
  return {
    leagueByID: new Map(leagueRows.map((l) => [l.id, l])),
    teamByID: new Map(teamRows.map((t) => [t.id, t])),
  };
}

function assemble(
  match: MatchRow,
  predictions: PredictionRow[] | undefined,
  leagueByID: Map<string, League>,
  teamByID: Map<string, Team>,
): MatchWithPredictions {
  const league = leagueByID.get(match.league_id);
  if (!league) {
    throw new Error(`match ${match.id} references unknown league ${match.league_id}`);
  }
  const home = teamByID.get(match.home_team_id);
  if (!home) {
    throw new Error(`match ${match.id} references unknown team ${match.home_team_id}`);
  }
  const away = teamByID.get(match.away_team_id);
  if (!away) {
    throw new Error(`match ${match.id} references unknown team ${match.away_team_id}`);
  }

  return {
    match: toAPIMatch(match),
    league,
    homeTeam: home,
    awayTeam: away,
    predictions: (predictions ?? []).map(toAPIPrediction),
  };
}

export interface FeedFiltersInput {
  leagueIDs: string[];
  markets: string[];
  minConfidence: number;
  region: string;
}

export async function feed(f: FeedFiltersInput): Promise<MatchWithPredictions[]> {
  const args: unknown[] = [];
  const where = [
    `m.status IN ('scheduled','in_play')`,
    'l.is_active',
    'l.is_published',
  ];

  if (f.leagueIDs.length > 0) {
    args.push(f.leagueIDs);
    where.push(`m.league_id = ANY($${args.length})`);
  }
  if (f.region !== '' && f.region !== 'all') {
    args.push(f.region);
    where.push(`l.region = $${args.length}`);
  }

  const matches = await scanMatches(
    `
    SELECT m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
           m.kickoff_at, m.status, m.home_score, m.away_score, m.round
    FROM matches m
    JOIN leagues l ON l.id = m.league_id
    WHERE ${where.join(' AND ')}
    ORDER BY m.kickoff_at, m.id`,
    args,
  );
  if (matches.length === 0) return [];

  const predictions = await predictionsForMatches(
    matches.map((m) => m.id),
    f.markets,
    f.minConfidence,
  );
  const { leagueByID, teamByID } = await referenceMaps();

  const out: MatchWithPredictions[] = [];
  for (const match of matches) {
    const picks = predictions.get(match.id);
    // A fixture with nothing left to show after filtering is not a result.
    // Results are not fetched here at all — the feed shows upcoming fixtures,
    // and graded results belong to match detail and the settled ledger.
    if (!picks || picks.length === 0) continue;
    out.push(assemble(match, picks, leagueByID, teamByID));
  }
  return out;
}

export const handleFeed: ApiHandler = async (ctx) => {
  const filters = parseFeedFilters(ctx.url);
  return jsonResponse(await feed(filters), CACHE.feed, ctx.request);
};

async function matchReasoning(matchID: string): Promise<MatchReasoning> {
  interface FormRaw {
    teamId: string;
    venue: 'home' | 'away';
    played: number;
    wins: number;
    draws: number;
    losses: number;
    goalsFor: number;
    goalsAgainst: number;
    recent: MatchReasoning['homeForm']['recent'];
    attackStrength: number;
    defenseStrength: number;
  }
  interface H2hRaw {
    matchId: string;
    kickoffAt: string;
    homeTeamId: string;
    awayTeamId: string;
    homeScore: number;
    awayScore: number;
  }
  interface ScorelineRaw {
    home: number;
    away: number;
    probability: number;
  }

  const row = await query<{
    match_id: string;
    xg_home: number;
    xg_away: number;
    home_form: FormRaw;
    away_form: FormRaw;
    head_to_head: H2hRaw[];
    top_scorelines: ScorelineRaw[];
    sample_home: number;
    sample_away: number;
    model_version: string;
  }>(
    `
    SELECT match_id, xg_home::float8, xg_away::float8, home_form, away_form,
           head_to_head, top_scorelines, sample_home, sample_away, model_version
    FROM match_reasoning WHERE match_id = $1`,
    [matchID],
  );
  const r = row[0];
  if (!r) throw AppError.notFound();

  // jsonb stores keys in its own canonical order; Go decodes into structs and
  // re-marshals, so the response order is the struct's. Map every nested
  // object explicitly for the same reason outcomes and distributions are.
  const toForm = (raw: FormRaw): MatchReasoning['homeForm'] => ({
    teamId: raw.teamId,
    venue: raw.venue,
    played: raw.played,
    wins: raw.wins,
    draws: raw.draws,
    losses: raw.losses,
    goalsFor: raw.goalsFor,
    goalsAgainst: raw.goalsAgainst,
    recent: raw.recent ?? [],
    attackStrength: raw.attackStrength,
    defenseStrength: raw.defenseStrength,
  });

  return {
    matchId: r.match_id,
    xgHome: r.xg_home,
    xgAway: r.xg_away,
    homeForm: toForm(r.home_form),
    awayForm: toForm(r.away_form),
    headToHead: (r.head_to_head ?? []).map((h) => ({
      matchId: h.matchId,
      kickoffAt: formatTimestamptz(h.kickoffAt),
      homeTeamId: h.homeTeamId,
      awayTeamId: h.awayTeamId,
      homeScore: h.homeScore,
      awayScore: h.awayScore,
    })),
    topScorelines: (r.top_scorelines ?? []).map((s) => ({
      home: s.home,
      away: s.away,
      probability: s.probability,
    })),
    modelVersion: r.model_version,
    sampleSize: { home: r.sample_home, away: r.sample_away },
  };
}

/**
 * One fixture with its predictions, any results, and the reasoning snapshot
 * taken when the model decided.
 *
 * The detail page exists to show *why* a pick was made. Without the snapshot
 * there is nothing to show, and recomputing it now would show a different,
 * flattering picture — hence ErrNotFound rather than a fallback.
 */
export async function matchDetail(matchID: string): Promise<MatchDetail> {
  const matches = await scanMatches(
    `
    SELECT m.id, m.league_id, m.season_id, m.home_team_id, m.away_team_id,
           m.kickoff_at, m.status, m.home_score, m.away_score, m.round
    FROM matches m WHERE m.id = $1`,
    [matchID],
  );
  if (matches.length === 0) throw AppError.notFound();
  const match = matches[0];

  const [predictions, reference, results, reasoning] = await Promise.all([
    predictionsForMatches([match.id], [], 0),
    referenceMaps(),
    resultsForMatch(match.id),
    matchReasoning(match.id),
  ]);

  const base = assemble(match, predictions.get(match.id), reference.leagueByID, reference.teamByID);
  const detail: MatchDetail = { ...base, reasoning };
  if (results.length > 0) (detail as { results?: APIPredictionResult[] }).results = results;
  return detail;
}

export const handleMatchDetail: ApiHandler = async (ctx) => {
  const id = parseUUID(ctx.params.id ?? '', 'match id');
  return jsonResponse(await matchDetail(id), CACHE.feed, ctx.request);
};
