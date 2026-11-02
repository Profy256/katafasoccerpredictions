/**
 * Betting markets. The MVP six, matching MarketCode in src/api/types.ts
 * exactly — these strings cross the API boundary verbatim.
 *
 * Port of backend/internal/domain/market.go. `MARKET_CODES` order is
 * load-bearing: the free shortlist walks markets in this sequence and the
 * per-fixture appearance cap depends on it.
 */

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

/**
 * Validates an incoming market string. An unknown code is an error rather than
 * a silent skip — API.md requires a 400 for a typo'd market rather than an
 * unfiltered response.
 */
export function parseMarketCode(s: string): MarketCode | null {
  return (MARKET_CODES as string[]).includes(s) ? (s as MarketCode) : null;
}

export interface MarketOutcome {
  value: string;
  label: string;
  shortLabel: string;
}

export interface MarketType {
  code: MarketCode;
  displayName: string;
  shortName: string;
  tabLabel: string;
  slug: string;
  outcomes: MarketOutcome[];
}
