import type { AccuracyBucket } from '@/api/types';
import { formatCount, formatRate } from '@/lib/format';

/**
 * Hit rate by category.
 *
 * One hue for every bar — length already encodes the value, so shading bars
 * darker-where-bigger would burn the colour channel on information the chart
 * shows twice. Bars run from a true zero baseline on a fixed 0–100% scale, so
 * two of these charts can be compared side by side.
 */
export function HitRateBars({ buckets }: { buckets: AccuracyBucket[] }) {
  if (buckets.length === 0) {
    return <p className="py-6 text-sm text-fg-muted">Nothing graded yet.</p>;
  }

  return (
    <ul className="space-y-3">
      {buckets.map((b) => (
        <li
          key={b.key}
          className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 sm:grid-cols-[9rem_1fr_5.5rem] sm:items-center"
          title={`${b.label}: ${b.correct} correct of ${b.total} graded (${formatRate(b.hitRate)})`}
        >
          <span className="truncate text-xs text-fg-muted">{b.label}</span>

          <span className="order-3 col-span-2 h-2.5 overflow-hidden rounded-full bg-line-soft sm:order-none sm:col-span-1">
            <span
              className="block h-full rounded-r-[4px] bg-brand"
              style={{ width: `${Math.max(b.hitRate * 100, 0.5)}%` }}
            />
          </span>

          <span className="text-right text-xs tabular-nums sm:text-left">
            <span className="font-semibold text-fg">{formatRate(b.hitRate)}</span>
            <span className="ml-1.5 text-fg-dim">n={formatCount(b.total)}</span>
          </span>
        </li>
      ))}
    </ul>
  );
}
