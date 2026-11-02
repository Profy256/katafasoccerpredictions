import type { AccuracyBucket } from '@/server/domain/accuracy';

/**
 * Pro-tier types: packages, analysts, slips, tips — port of
 * backend/internal/domain/pro.go read models. `UGX` is int64 shillings,
 * serialised as a plain number; never float, never cents.
 */

export type PackageCode = 'ordinary' | 'vip' | 'akatambula';

export const PACKAGE_CODES: PackageCode[] = ['ordinary', 'vip', 'akatambula'];

export function parsePackageCode(s: string): PackageCode | null {
  return (PACKAGE_CODES as string[]).includes(s) ? (s as PackageCode) : null;
}

export interface Package {
  code: PackageCode;
  name: string;
  tagline: string;
  description: string;
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
  /**
   * The packages this analyst has actually published slips in. Go only ever
   * appends to a nil slice, so an analyst with no published packages
   * serialises as `null` — mirrored here rather than "fixed": the Go↔TS
   * harness compares bytes, and this is the shape it would produce.
   */
  packages: PackageCode[] | null;
  joinedAt: string;
}

export type SettledBy = 'auto' | 'admin';

/** Slip in src/api/types.ts: status is 'open' | 'settled' only. */
export interface APISlip {
  id: string;
  packageCode: PackageCode;
  title: string;
  analystIds: string[];
  publishedAt: string;
  priceUgx: number;
  status: string;
  tipCount: number;
  totalOdds: number;
  /** Pointer field with omitempty in Go: absent when the column is null. */
  wonTips?: number;
  /**
   * The accumulator after void legs were removed. Shown alongside totalOdds
   * rather than replacing it: buyers were advertised the published price and
   * it must keep showing. Absent when null — omitempty, like Go.
   */
  settledOdds?: number;
}

export interface APITip {
  id: string;
  slipId: string;
  analystId: string;
  matchId: string | null;
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  marketType: string | null;
  selectionValue: string | null;
  odds: number;
  kickoffAt: string;
  note?: string | null;
}

export interface APITipResult {
  tipId: string;
  wasCorrect: boolean;
  actualOutcome: string;
  settledAt: string;
  settledBy: SettledBy;
}

/**
 * A slip and, if the viewer is entitled to them, its picks. Tips is empty for
 * an unpaid viewer because the *database returned no rows* — the entitlement
 * is folded into the query's WHERE clause.
 */
export interface SlipWithTips extends APISlip {
  tips: APITip[];
  results?: APITipResult[];
  analysts: Analyst[];
  package: Package;
  unlocked: boolean;
}

export interface AnalystRecord {
  analyst: Analyst;
  overall: AccuracyBucket;
  last30Days: AccuracyBucket;
  byPackage: AccuracyBucket[];
  profitUnits: number;
  roi: number;
  averageOdds: number;
  recentSlips: APISlip[];
}
