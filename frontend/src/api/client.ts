import { ApiError, REVALIDATE, apiGet, apiGetPrivate, apiGetPrivateOrNull, query } from './http';
import type {
  AccuracySummary,
  Analyst,
  AnalystRecord,
  FeedFilters,
  FreeTipsDay,
  FreeTipsHistoryDay,
  League,
  LedgerFilters,
  MatchDetail,
  MatchWithPredictions,
  PackageCode,
  Purchase,
  SessionUser,
  SettledPrediction,
  Slip,
  SlipWithTips,
  TipPackage,
} from './types';

/**
 * The frontend's entire data surface, served by the Go API.
 *
 * Every function here is transport only — no filtering, no selection, no
 * derivation. That is the whole point of this file after the cutover: the
 * shortlist is chosen and frozen at publish time, entitlement is decided in
 * SQL, and accuracy is aggregated in Postgres. Anything recomputed here would
 * be a second implementation of a published number, free to disagree with the
 * one on the record.
 *
 * See docs/backend/API.md for the endpoint mapping.
 */

export async function getLeagues(): Promise<League[]> {
  return apiGet<League[]>('/v1/leagues', { revalidate: REVALIDATE.reference });
}

/** Upcoming fixtures with their published predictions (FR-1, FR-3). */
export async function getFeed(filters: FeedFilters = {}): Promise<MatchWithPredictions[]> {
  const qs = query({
    leagues: filters.leagueIds,
    markets: filters.markets,
    min_confidence: filters.minConfidence,
    region: filters.region === 'all' ? undefined : filters.region,
  });
  return apiGet<MatchWithPredictions[]>(`/v1/feed${qs}`, { revalidate: REVALIDATE.feed });
}

/** A single fixture with the full model reasoning behind it (FR-4). */
export async function getMatchDetail(matchId: string): Promise<MatchDetail | null> {
  return orNullOn404(
    apiGet<MatchDetail>(`/v1/matches/${encodeURIComponent(matchId)}`, {
      revalidate: REVALIDATE.feed,
    }),
  );
}

/* ------------------------------------------------------------------ *
 * Accuracy (FR-5, FR-6)
 * ------------------------------------------------------------------ */

/**
 * Exported so the calibration chart can draw each band's stated range.
 *
 * These mirror the `confidenceBands` the API aggregates over. They are
 * presentation metadata — the bucketing itself happens in Postgres, and the
 * keys here have to match the ones it returns.
 */
export const CONFIDENCE_BANDS: { key: string; label: string; min: number; max: number }[] = [
  { key: 'lt50', label: 'Under 50%', min: 0, max: 50 },
  { key: '50-60', label: '50–60%', min: 50, max: 60 },
  { key: '60-70', label: '60–70%', min: 60, max: 70 },
  { key: '70-80', label: '70–80%', min: 70, max: 80 },
  { key: '80-90', label: '80–90%', min: 80, max: 90 },
  { key: 'gte90', label: '90%+', min: 90, max: 100.01 },
];

/**
 * The public accuracy dashboard payload, computed over *every* settled
 * prediction with no exclusions (FR-6). Read from `accuracy_rollup`, never
 * aggregated live.
 */
export async function getAccuracySummary(): Promise<AccuracySummary> {
  return apiGet<AccuracySummary>('/v1/accuracy', { revalidate: REVALIDATE.accuracy });
}

/**
 * The raw graded ledger — every settled pick, wins and losses alike. This is
 * the receipt behind the headline numbers (FR-7).
 *
 * The `outcome` filter narrows what a *reader* asked to see. It never narrows
 * what the headline figures are computed from.
 */
export async function getSettledPredictions(
  filters: LedgerFilters = {},
): Promise<SettledPrediction[]> {
  const qs = query({
    leagues: filters.leagueIds,
    markets: filters.markets,
    outcome: filters.outcome,
    limit: filters.limit,
  });
  const page = await apiGet<{ items: SettledPrediction[]; nextCursor?: string }>(
    `/v1/predictions/settled${qs}`,
    { revalidate: REVALIDATE.accuracy },
  );
  return page.items;
}

/* ------------------------------------------------------------------ *
 * Free daily tips
 * ------------------------------------------------------------------ */

/**
 * The free shortlist for a published day.
 *
 * Read from the frozen `free_tip_days` / `free_tips` rows and never
 * re-selected. The selection logic that used to live here has moved to the
 * backend's `publish_free_tips` job, which runs once at 05:00 and writes the
 * result down — see SETTLEMENT.md § Why the free shortlist is frozen. A
 * shortlist recomputed on read would quietly change after kickoff, and the
 * published record would stop matching what readers were actually shown.
 *
 * @param date UTC day as YYYY-MM-DD. Omit for the current published day.
 */
export async function getFreeTips(date?: string): Promise<FreeTipsDay> {
  return apiGet<FreeTipsDay>(`/v1/tips/free${query({ date })}`, {
    revalidate: REVALIDATE.freeTips,
  });
}

/** Past shortlists with their results — "yesterday went 4 from 5". */
export async function getFreeTipsHistory(
  range: { from?: string; to?: string } = {},
): Promise<FreeTipsHistoryDay[]> {
  const page = await apiGet<{ days: FreeTipsHistoryDay[] }>(
    `/v1/tips/free/history${query(range)}`,
    { revalidate: REVALIDATE.reference },
  );
  return page.days;
}

/* ------------------------------------------------------------------ *
 * Pro tier
 * ------------------------------------------------------------------ */

export async function getPackages(): Promise<TipPackage[]> {
  return apiGet<TipPackage[]>('/v1/packages', { revalidate: REVALIDATE.reference });
}

export interface SlipQuery {
  packageCode?: PackageCode;
  status?: 'open' | 'settled';
  analystId?: string;
  limit?: number;
}

/**
 * Published slips. Drafts never appear — their ids must not be discoverable.
 *
 * Uncached: the response is metadata only, but which slips a viewer owns
 * varies per request, and a shared cache would serve one viewer's view to
 * another.
 */
export async function getSlips(q: SlipQuery = {}): Promise<Slip[]> {
  const qs = query({
    package: q.packageCode,
    status: q.status,
    analyst: q.analystId,
    limit: q.limit,
  });
  const page = await apiGetPrivate<{ items: Slip[]; nextCursor?: string }>(`/v1/slips${qs}`);
  return page.items;
}

/**
 * A slip and, if the viewer is entitled to them, its tips.
 *
 * There is no `ownedSlipIds` argument any more, and that is the point: the
 * entitlement is a WHERE clause in the API's query, so an unpaid viewer gets
 * zero tip rows *from the database*. Nothing here decides what to show —
 * `unlocked` reports what the query already returned.
 *
 * A draft slip is a 404 rather than a 403, so unpublished ids stay
 * undiscoverable by probing.
 */
export async function getSlip(slipId: string): Promise<SlipWithTips | null> {
  return orNullOn404(
    apiGetPrivate<SlipWithTips>(`/v1/slips/${encodeURIComponent(slipId)}`),
  );
}

export async function getAnalysts(): Promise<Analyst[]> {
  return apiGet<Analyst[]>('/v1/analysts', { revalidate: REVALIDATE.reference });
}

export async function getAnalystRecord(slug: string): Promise<AnalystRecord | null> {
  return orNullOn404(
    apiGet<AnalystRecord>(`/v1/analysts/${encodeURIComponent(slug)}`, {
      revalidate: REVALIDATE.accuracy,
    }),
  );
}

/** Every analyst's record, best hit rate first. */
export async function getAnalystLeaderboard(): Promise<AnalystRecord[]> {
  return apiGet<AnalystRecord[]>('/v1/analysts/leaderboard', {
    revalidate: REVALIDATE.accuracy,
  });
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
  return apiGet<CoverageStats>('/v1/stats/coverage', { revalidate: REVALIDATE.stats });
}

/* ------------------------------------------------------------------ *
 * Session and purchases
 *
 * Replaces src/lib/session.ts, whose cookies were forgeable by anyone and
 * said so. The session is now an opaque token the API issues and verifies.
 * ------------------------------------------------------------------ */

/** The signed-in user, or null. Not being signed in is not an error. */
export async function getSession(): Promise<SessionUser | null> {
  const body = await apiGetPrivateOrNull<{ user: SessionUser }>('/v1/auth/me');
  return body?.user ?? null;
}

/** The viewer's purchases, newest first. Empty when signed out. */
export async function getPurchases(): Promise<Purchase[]> {
  return (await apiGetPrivateOrNull<Purchase[]>('/v1/purchases')) ?? [];
}

/**
 * Slip ids the viewer has paid for.
 *
 * Used only to badge the slips *list* — it must never be used to decide
 * whether to render tips. That decision belongs to the API's entitlement
 * query, and duplicating it here would be exactly the fetch-then-hide the
 * paywall is designed to avoid.
 *
 * A refunded purchase is not included: refunding revokes access.
 */
export async function getOwnedSlipIds(): Promise<Set<string>> {
  const purchases = await getPurchases();
  return new Set(purchases.filter((p) => p.status === 'paid').map((p) => p.slipId));
}

/**
 * Turns a 404 into null. Reserved for "this id does not exist", which pages
 * render as `notFound()`; every other status still throws.
 */
async function orNullOn404<T>(pending: Promise<T>): Promise<T | null> {
  try {
    return await pending;
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}
