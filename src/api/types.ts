/**
 * Domain types for the predictions platform.
 *
 * These mirror the data model in PRD section 7 so that when the Go API layer
 * lands, the frontend contract does not have to change: only the transport in
 * `client.ts` gets swapped from the local mock adapter to `fetch`.
 */

/** Market codes. MVP scope only — see PRD section 3.1. */
export type MarketCode =
  | 'ONE_X_TWO'
  | 'DOUBLE_CHANCE'
  | 'BTTS'
  | 'OVER_UNDER_1_5'
  | 'OVER_UNDER_2_5'
  | 'OVER_UNDER_3_5';

export const MARKET_CODES: MarketCode[] = [
  'ONE_X_TWO',
  'DOUBLE_CHANCE',
  'BTTS',
  'OVER_UNDER_1_5',
  'OVER_UNDER_2_5',
  'OVER_UNDER_3_5',
];

export interface MarketType {
  code: MarketCode;
  displayName: string;
  shortName: string;
  /** Tab label — shorter than displayName, clearer than shortName. */
  tabLabel: string;
  /** URL segment for this market's own tips page, e.g. /tips/btts. */
  slug: string;
  /** Every outcome this market can settle to, in display order. */
  outcomes: { value: string; label: string; shortLabel: string }[];
}

export type Region = 'europe' | 'east-africa' | 'africa' | 'americas';

export const REGIONS: { value: Region; label: string }[] = [
  { value: 'europe', label: 'Europe' },
  { value: 'east-africa', label: 'East Africa' },
  { value: 'africa', label: 'Rest of Africa' },
  { value: 'americas', label: 'Americas' },
];

export interface League {
  id: string;
  name: string;
  shortName: string;
  country: string;
  /** ISO-ish code used for the flag chip. */
  countryCode: string;
  tier: number;
  region: Region;
}

export interface Team {
  id: string;
  name: string;
  shortName: string;
  leagueId: string;
}

export type MatchStatus = 'scheduled' | 'finished';

export interface Match {
  id: string;
  leagueId: string;
  homeTeamId: string;
  awayTeamId: string;
  /** ISO 8601 timestamp. */
  kickoffAt: string;
  status: MatchStatus;
  homeScore: number | null;
  awayScore: number | null;
  round: number;
}

export interface Prediction {
  id: string;
  matchId: string;
  marketType: MarketCode;
  /** The selected outcome, e.g. 'HOME', 'YES', 'OVER'. */
  predictionValue: string;
  /**
   * The model's own probability for `predictionValue`, as a percentage.
   * Per PRD 5.4 this is never a separately fabricated number.
   */
  confidencePct: number;
  modelVersion: string;
  createdAt: string;
  /** Full probability distribution across the market, for the detail view. */
  distribution: { value: string; probability: number }[];
}

export interface PredictionResult {
  predictionId: string;
  actualOutcome: string;
  wasCorrect: boolean;
  settledAt: string;
}

/* ------------------------------------------------------------------ *
 * Read models returned by the API
 * ------------------------------------------------------------------ */

/** A fixture plus its predictions, as served to the feed (FR-1). */
export interface MatchWithPredictions {
  match: Match;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
  predictions: Prediction[];
  /** Present only once the match has been graded (FR-5). */
  results?: PredictionResult[];
}

export type FormChar = 'W' | 'D' | 'L';

/** Rolling form for one team at one venue, as used by the model. */
export interface FormSummary {
  teamId: string;
  /** Which venue the strengths and record below are measured at. */
  venue: 'home' | 'away';
  /** Matches in the rolling window (at this venue only). */
  played: number;
  wins: number;
  draws: number;
  losses: number;
  goalsFor: number;
  goalsAgainst: number;
  /** Last few results across both venues, most recent first. */
  recent: FormChar[];
  /** Venue-split model inputs, 1.0 == league average. */
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

/**
 * Everything the match detail view needs to show *why* a pick was made
 * (FR-4). Nothing here is decorative — each field is a real model input
 * or a direct output of the scoreline matrix.
 */
export interface MatchReasoning {
  matchId: string;
  xgHome: number;
  xgAway: number;
  homeForm: FormSummary;
  awayForm: FormSummary;
  headToHead: HeadToHeadMatch[];
  /** Highest-probability exact scorelines from the Poisson matrix. */
  topScorelines: { home: number; away: number; probability: number }[];
  modelVersion: string;
  /** How much history the model had to work with, for honesty about sample size. */
  sampleSize: { home: number; away: number };
}

export interface MatchDetail extends MatchWithPredictions {
  reasoning: MatchReasoning;
}

/* ------------------------------------------------------------------ *
 * Accuracy tracking (FR-5, FR-6, FR-7)
 * ------------------------------------------------------------------ */

export interface AccuracyBucket {
  key: string;
  label: string;
  total: number;
  correct: number;
  /** 0..1 */
  hitRate: number;
}

export interface AccuracyPoint {
  date: string;
  /** Cumulative hit rate up to and including this date, 0..1. */
  cumulativeHitRate: number;
  /** Hit rate for this date alone, 0..1. */
  dailyHitRate: number;
  settled: number;
}

export interface AccuracySummary {
  overall: AccuracyBucket;
  byMarket: AccuracyBucket[];
  byLeague: AccuracyBucket[];
  /** Calibration check: does 70%-confidence actually hit ~70% of the time? */
  byConfidenceBand: AccuracyBucket[];
  timeline: AccuracyPoint[];
  modelVersion: string;
  /** Window covered by the graded set. */
  firstSettledAt: string;
  lastSettledAt: string;
}

/** One graded pick, for the unfiltered "no hiding misses" ledger (FR-7). */
export interface SettledPrediction {
  prediction: Prediction;
  result: PredictionResult;
  match: Match;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
}

/* ------------------------------------------------------------------ *
 * Query parameters
 * ------------------------------------------------------------------ */

/* ------------------------------------------------------------------ *
 * Free daily tips
 *
 * The free tier is a curated shortlist rather than the whole fixture list:
 * 3-5 of the strongest model picks per market, published daily.
 * ------------------------------------------------------------------ */

export interface FreeTip {
  match: Match;
  league: League;
  homeTeam: Team;
  awayTeam: Team;
  prediction: Prediction;
  /** Indicative bookmaker odds implied by the model price. */
  odds: number;
}

export interface FreeTipGroup {
  market: MarketCode;
  tips: FreeTip[];
}

export interface FreeTipsDay {
  /** UTC day the shortlist covers. */
  date: string;
  groups: FreeTipGroup[];
  totalTips: number;
}

/* ------------------------------------------------------------------ *
 * Pro tier — human analysts, packages, and per-slip purchases
 *
 * Users buy individual slips, not monthly subscriptions. The price of each
 * slip is set by the admin at the time the slip is entered, so it lives on
 * the slip rather than on the package.
 * ------------------------------------------------------------------ */

export type PackageCode = 'ordinary' | 'vip' | 'akatambula';

export const PACKAGE_CODES: PackageCode[] = ['ordinary', 'vip', 'akatambula'];

export interface TipPackage {
  code: PackageCode;
  name: string;
  tagline: string;
  description: string;
  /** Indicative only — the real figure is whatever the admin set per slip. */
  typicalPriceUgx: number;
  highlights: string[];
}

export interface Analyst {
  id: string;
  slug: string;
  name: string;
  handle: string;
  initials: string;
  bio: string;
  packages: PackageCode[];
  joinedAt: string;
}

export interface Tip {
  id: string;
  slipId: string;
  analystId: string;
  /** Set when the tip maps onto a fixture we track; null for anything else. */
  matchId: string | null;
  /** Free text, because analysts may tip markets outside the MVP six. */
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  /** Structured form, present only when the tip can be auto-graded. */
  marketType: MarketCode | null;
  selectionValue: string | null;
  odds: number;
  kickoffAt: string;
  note?: string;
}

export interface TipResult {
  tipId: string;
  wasCorrect: boolean;
  actualOutcome: string;
  settledAt: string;
  /** Auto where the tip mapped to a tracked market; admin otherwise. */
  settledBy: 'auto' | 'admin';
}

export interface Slip {
  id: string;
  packageCode: PackageCode;
  title: string;
  analystIds: string[];
  publishedAt: string;
  /** Whatever the admin charged for this specific slip, in UGX. */
  priceUgx: number;
  status: 'open' | 'settled';
  tipCount: number;
  /** Accumulated odds across every tip on the slip. */
  totalOdds: number;
  /** Present once settled. */
  wonTips?: number;
}

export interface SlipWithTips extends Slip {
  tips: Tip[];
  results?: TipResult[];
  analysts: Analyst[];
  package: TipPackage;
  /** Whether the current viewer has paid for this slip. */
  unlocked: boolean;
}

export interface AnalystRecord {
  analyst: Analyst;
  overall: AccuracyBucket;
  last30Days: AccuracyBucket;
  byPackage: AccuracyBucket[];
  /** Return from flat 1-unit stakes across every settled tip. */
  profitUnits: number;
  roi: number;
  averageOdds: number;
  recentSlips: Slip[];
}

export interface FeedFilters {
  leagueIds?: string[];
  markets?: MarketCode[];
  /** Minimum confidence percentage, 0..100. */
  minConfidence?: number;
  region?: Region | 'all';
}

export interface LedgerFilters {
  leagueIds?: string[];
  markets?: MarketCode[];
  outcome?: 'all' | 'hit' | 'miss';
  limit?: number;
}
