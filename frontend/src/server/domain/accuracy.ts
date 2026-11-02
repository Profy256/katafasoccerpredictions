/**
 * The accuracy dashboard shapes — port of backend/internal/domain readmodel
 * accuracy types.
 */

export interface AccuracyBucket {
  key: string;
  label: string;
  total: number;
  correct: number;
  /** 0..1 */
  hitRate: number;
}

/**
 * Keeps the zero-total case at a hit rate of 0 rather than NaN, matching the
 * frontend's `rows.length ? correct / rows.length : 0`.
 */
export function newAccuracyBucket(
  key: string,
  label: string,
  total: number,
  correct: number,
): AccuracyBucket {
  return {
    key,
    label,
    total,
    correct,
    hitRate: total > 0 ? correct / total : 0,
  };
}

export interface AccuracyPoint {
  date: string;
  cumulativeHitRate: number;
  dailyHitRate: number;
  settled: number;
}

export interface AccuracySummary {
  overall: AccuracyBucket;
  byMarket: AccuracyBucket[];
  byLeague: AccuracyBucket[];
  byConfidenceBand: AccuracyBucket[];
  timeline: AccuracyPoint[];
  modelVersion: string;
  firstSettledAt: string;
  lastSettledAt: string;
}
