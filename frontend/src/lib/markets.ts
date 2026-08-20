import type { MarketCode, MarketType } from '../api/types';

/**
 * Static market metadata. On the backend this is the `market_types` table
 * (PRD section 7); it is duplicated here so the UI can render labels without
 * a round trip.
 */
export const MARKETS: Record<MarketCode, MarketType> = {
  ONE_X_TWO: {
    code: 'ONE_X_TWO',
    displayName: 'Match Result (1X2)',
    shortName: '1X2',
    tabLabel: 'Match Result',
    slug: '1x2',
    outcomes: [
      { value: 'HOME', label: 'Home Win', shortLabel: '1' },
      { value: 'DRAW', label: 'Draw', shortLabel: 'X' },
      { value: 'AWAY', label: 'Away Win', shortLabel: '2' },
    ],
  },
  DOUBLE_CHANCE: {
    code: 'DOUBLE_CHANCE',
    displayName: 'Double Chance',
    shortName: 'DC',
    tabLabel: 'Double Chance',
    slug: 'double-chance',
    outcomes: [
      { value: '1X', label: 'Home or Draw', shortLabel: '1X' },
      { value: '12', label: 'Home or Away', shortLabel: '12' },
      { value: 'X2', label: 'Draw or Away', shortLabel: 'X2' },
    ],
  },
  BTTS: {
    code: 'BTTS',
    displayName: 'Both Teams To Score',
    shortName: 'BTTS',
    tabLabel: 'Both Teams Score',
    slug: 'btts',
    outcomes: [
      { value: 'YES', label: 'Yes', shortLabel: 'Yes' },
      { value: 'NO', label: 'No', shortLabel: 'No' },
    ],
  },
  OVER_UNDER_1_5: {
    code: 'OVER_UNDER_1_5',
    displayName: 'Over / Under 1.5 Goals',
    shortName: 'O/U 1.5',
    tabLabel: 'Over/Under 1.5',
    slug: 'over-under-1-5',
    outcomes: [
      { value: 'OVER', label: 'Over 1.5', shortLabel: 'O 1.5' },
      { value: 'UNDER', label: 'Under 1.5', shortLabel: 'U 1.5' },
    ],
  },
  OVER_UNDER_2_5: {
    code: 'OVER_UNDER_2_5',
    displayName: 'Over / Under 2.5 Goals',
    shortName: 'O/U 2.5',
    tabLabel: 'Over/Under 2.5',
    slug: 'over-under-2-5',
    outcomes: [
      { value: 'OVER', label: 'Over 2.5', shortLabel: 'O 2.5' },
      { value: 'UNDER', label: 'Under 2.5', shortLabel: 'U 2.5' },
    ],
  },
  OVER_UNDER_3_5: {
    code: 'OVER_UNDER_3_5',
    displayName: 'Over / Under 3.5 Goals',
    shortName: 'O/U 3.5',
    tabLabel: 'Over/Under 3.5',
    slug: 'over-under-3-5',
    outcomes: [
      { value: 'OVER', label: 'Over 3.5', shortLabel: 'O 3.5' },
      { value: 'UNDER', label: 'Under 3.5', shortLabel: 'U 3.5' },
    ],
  },
};

/**
 * The market shown on the landing page. It is the only one reachable without
 * navigating, so it is deliberately never ad-gated — see `lib/ads.ts`.
 */
export const DEFAULT_MARKET: MarketCode = 'ONE_X_TWO';

const BY_SLUG = new Map<string, MarketCode>(
  Object.values(MARKETS).map((m) => [m.slug, m.code]),
);

/** Resolves a `/tips/[market]` URL segment. Returns null for anything unknown. */
export function marketFromSlug(slug: string): MarketCode | null {
  return BY_SLUG.get(slug) ?? null;
}

/** Where a market's tips live. The default market owns the landing page. */
export function marketHref(code: MarketCode): string {
  return code === DEFAULT_MARKET ? '/' : `/tips/${MARKETS[code].slug}`;
}

/** Goal line for each Over/Under market, keyed by code. */
export const OVER_UNDER_LINES: Partial<Record<MarketCode, number>> = {
  OVER_UNDER_1_5: 1.5,
  OVER_UNDER_2_5: 2.5,
  OVER_UNDER_3_5: 3.5,
};

export function outcomeLabel(market: MarketCode, value: string): string {
  return MARKETS[market].outcomes.find((o) => o.value === value)?.label ?? value;
}

export function outcomeShortLabel(market: MarketCode, value: string): string {
  return MARKETS[market].outcomes.find((o) => o.value === value)?.shortLabel ?? value;
}
