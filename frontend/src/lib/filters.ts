import type { FeedFilters, MarketCode, Region } from '@/api/types';
import { MARKET_CODES, REGIONS } from '@/api/types';

/**
 * Feed filters live in the URL rather than in component state.
 *
 * That keeps a filtered view shareable and linkable, lets the server render the
 * already-filtered list, and means the back button behaves. FR-3 only asks for
 * the filters; putting them in the URL is what makes them useful.
 */

export type SearchParams = Record<string, string | string[] | undefined>;

const REGION_VALUES: Region[] = REGIONS.map((r) => r.value);

function readList(params: SearchParams, key: string): string[] {
  const raw = params[key];
  const joined = Array.isArray(raw) ? raw.join(',') : raw;
  if (!joined) return [];
  return joined
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

export function parseFeedFilters(params: SearchParams, validLeagueIds: string[]): FeedFilters {
  const leagueSet = new Set(validLeagueIds);
  const leagueIds = readList(params, 'league').filter((id) => leagueSet.has(id));

  const marketSet = new Set<string>(MARKET_CODES);
  const markets = readList(params, 'market').filter((m): m is MarketCode => marketSet.has(m));

  const rawMin = Array.isArray(params.minConf) ? params.minConf[0] : params.minConf;
  const parsedMin = rawMin ? Number(rawMin) : NaN;
  const minConfidence =
    Number.isFinite(parsedMin) && parsedMin > 0 ? Math.min(100, Math.max(0, parsedMin)) : undefined;

  const rawRegion = Array.isArray(params.region) ? params.region[0] : params.region;
  const region = REGION_VALUES.includes(rawRegion as Region) ? (rawRegion as Region) : 'all';

  return {
    leagueIds: leagueIds.length ? leagueIds : undefined,
    markets: markets.length ? markets : undefined,
    minConfidence,
    region,
  };
}

/** Serialises filters back to a query string, omitting anything at its default. */
export function buildFeedQuery(filters: FeedFilters): string {
  const params = new URLSearchParams();
  if (filters.leagueIds?.length) params.set('league', filters.leagueIds.join(','));
  if (filters.markets?.length) params.set('market', filters.markets.join(','));
  if (filters.minConfidence) params.set('minConf', String(filters.minConfidence));
  if (filters.region && filters.region !== 'all') params.set('region', filters.region);
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export function countActiveFilters(filters: FeedFilters): number {
  let n = 0;
  if (filters.leagueIds?.length) n++;
  if (filters.markets?.length) n++;
  if (filters.minConfidence) n++;
  if (filters.region && filters.region !== 'all') n++;
  return n;
}

/** Confidence steps offered in the filter UI. */
export const CONFIDENCE_STEPS = [0, 55, 65, 75, 85];
