import type { AccuracyBucket } from '@/api/types';
import { formatCount, formatRate } from '@/lib/format';

/**
 * Calibration: does a stated confidence actually deliver that hit rate?
 *
 * Each row shows the band the model claimed (a shaded zone) and where the
 * results actually landed (a marker). A marker inside its own zone means the
 * confidence number is honest — which is the whole basis for showing
 * confidence to users at all (PRD 5.4).
 *
 * Drawn as zone + marker rather than two coloured series so nothing here
 * depends on telling two hues apart.
 */
export interface ConfidenceBandRange {
  key: string;
  min: number;
  max: number;
}

export function CalibrationChart({
  buckets,
  bands,
}: {
  buckets: AccuracyBucket[];
  bands: ConfidenceBandRange[];
}) {
  const rangeFor = (key: string) => bands.find((b) => b.key === key);

  return (
    <div>
      {/* Two visual elements are in play, so they get a key. */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-fg-muted">
        <span className="flex items-center gap-1.5">
          <span aria-hidden className="h-2.5 w-6 rounded-sm bg-fg-dim/35" />
          Confidence the model stated
        </span>
        <span className="flex items-center gap-1.5">
          <span
            aria-hidden
            className="h-2.5 w-2.5 rounded-full bg-brand ring-2 ring-surface"
          />
          Hit rate actually achieved
        </span>
      </div>

      <ul className="mt-4 space-y-3.5">
        {buckets.map((b) => {
          const range = rangeFor(b.key);
          const min = range?.min ?? 0;
          const max = Math.min(range?.max ?? 100, 100);
          const actual = b.hitRate * 100;
          const inBand = actual >= min && actual <= max;

          return (
            <li
              key={b.key}
              className="grid grid-cols-[4.5rem_1fr_auto] items-center gap-3"
              title={`Stated ${b.label}: actually hit ${formatRate(b.hitRate)} across ${b.total} predictions`}
            >
              <span className="text-xs tabular-nums text-fg-muted">{b.label}</span>

              <span className="relative block h-2.5 rounded-full bg-line-soft">
                <span
                  aria-hidden
                  className="absolute inset-y-0 rounded-sm bg-fg-dim/35"
                  style={{ left: `${min}%`, width: `${Math.max(max - min, 1)}%` }}
                />
                <span
                  aria-hidden
                  className="absolute top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full bg-brand ring-2 ring-surface"
                  style={{ left: `${Math.min(Math.max(actual, 0), 100)}%` }}
                />
              </span>

              <span className="text-right text-xs tabular-nums">
                <span className="font-semibold">{formatRate(b.hitRate)}</span>
                <span className="ml-1.5 text-fg-dim">n={formatCount(b.total)}</span>
                <span className="sr-only">
                  {inBand ? ' — within the stated band' : ' — outside the stated band'}
                </span>
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
