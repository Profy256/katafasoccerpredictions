import type { MarketCode } from '@/server/domain/market';
import { formatTimestamptz } from '@/server/http/time';

/**
 * The read models the API serves. These are the interfaces in
 * src/api/types.ts, field for field. JSON keys are camelCase and timestamps
 * serialise as RFC3339 UTC — port of backend/internal/domain/readmodel.go and
 * match.go, including the struct field order that determines key order.
 */

export type Region = 'europe' | 'east-africa' | 'africa' | 'americas' | 'asia';

export const REGIONS: Region[] = ['europe', 'east-africa', 'africa', 'americas', 'asia'];

export function parseRegion(s: string): Region | null {
  return (REGIONS as string[]).includes(s) ? (s as Region) : null;
}

/** Internal status vocabulary; the wire only sees the two APIStatus values. */
export type InternalMatchStatus =
  | 'scheduled'
  | 'in_play'
  | 'finished'
  | 'postponed'
  | 'cancelled'
  | 'abandoned';

/** in_play presents as scheduled (no final score yet); non-completions too. */
export function apiStatus(status: InternalMatchStatus): 'scheduled' | 'finished' {
  return status === 'finished' ? 'finished' : 'scheduled';
}

export interface League {
  id: string;
  name: string;
  shortName: string;
  country: string;
  countryCode: string;
  tier: number;
  region: Region | string;
}

export interface Team {
  id: string;
  leagueId: string;
  name: string;
  shortName: string;
}

/** APIMatch — Match in src/api/types.ts. Round is non-null; null becomes 0. */
export interface APIMatch {
  id: string;
  leagueId: string;
  homeTeamId: string;
  awayTeamId: string;
  kickoffAt: string;
  status: 'scheduled' | 'finished';
  homeScore: number | null;
  awayScore: number | null;
  round: number;
}

export interface OutcomeProbability {
  value: string;
  probability: number;
}

export interface APIPrediction {
  id: string;
  matchId: string;
  marketType: MarketCode;
  predictionValue: string;
  confidencePct: number;
  modelVersion: string;
  createdAt: string;
  distribution: OutcomeProbability[];
}

export interface APIPredictionResult {
  predictionId: string;
  actualOutcome: string;
  wasCorrect: boolean;
  settledAt: string;
}

export interface MatchWithPredictions {
  match: APIMatch;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
  predictions: APIPrediction[];
  results?: APIPredictionResult[];
}

export type FormChar = 'W' | 'D' | 'L';

export interface FormSummary {
  teamId: string;
  venue: 'home' | 'away';
  played: number;
  wins: number;
  draws: number;
  losses: number;
  goalsFor: number;
  goalsAgainst: number;
  recent: FormChar[];
  /** Multiples of the league average, where 1.0 is exactly average. */
  attackStrength: number;
  defenseStrength: number;
}

export interface HeadToHeadMatch {
  matchId: string;
  kickoffAt: string;
  homeTeamId: string;
  awayTeamId: string;
  homeScore: number;
  awayScore: number;
}

export interface Scoreline {
  home: number;
  away: number;
  probability: number;
}

export interface SampleSize {
  home: number;
  away: number;
}

/** A snapshot of what the model saw when it decided — never a recomputation. */
export interface MatchReasoning {
  matchId: string;
  xgHome: number;
  xgAway: number;
  homeForm: FormSummary;
  awayForm: FormSummary;
  headToHead: HeadToHeadMatch[];
  topScorelines: Scoreline[];
  modelVersion: string;
  sampleSize: SampleSize;
}

export interface MatchDetail extends MatchWithPredictions {
  reasoning: MatchReasoning;
}

/**
 * One row of the unfiltered graded ledger — wins and losses alike. Port of
 * SettledPrediction in backend/internal/domain/readmodel.go.
 */
export interface SettledPrediction {
  prediction: APIPrediction;
  result: APIPredictionResult;
  match: APIMatch;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
}

interface MatchRow {
  id: string;
  league_id: string;
  season_id: string;
  home_team_id: string;
  away_team_id: string;
  kickoff_at: string;
  status: string;
  home_score: number | null;
  away_score: number | null;
  round: number | null;
}

/** Narrows a matches row for the wire — port of Match.ToAPI. */
export function toAPIMatch(row: MatchRow): APIMatch {
  return {
    id: row.id,
    leagueId: row.league_id,
    homeTeamId: row.home_team_id,
    awayTeamId: row.away_team_id,
    kickoffAt: formatTimestamptz(row.kickoff_at),
    status: apiStatus(row.status as InternalMatchStatus),
    homeScore: row.home_score,
    awayScore: row.away_score,
    round: row.round ?? 0,
  };
}

export type { MatchRow };

interface PredictionRow {
  id: string;
  match_id: string;
  market_code: MarketCode;
  prediction_value: string;
  confidence_pct: number;
  distribution: OutcomeProbability[] | null;
  model_version: string;
  created_at: string;
}

/** Port of Prediction.ToAPI: nil distribution becomes []. */
export function toAPIPrediction(row: PredictionRow): APIPrediction {
  const raw = row.distribution ?? [];
  return {
    id: row.id,
    matchId: row.match_id,
    marketType: row.market_code,
    predictionValue: row.prediction_value,
    confidencePct: row.confidence_pct,
    modelVersion: row.model_version,
    createdAt: formatTimestamptz(row.created_at),
    distribution: raw.map((entry) => ({
      value: entry.value,
      probability: entry.probability,
    })),
  };
}

export type { PredictionRow };
