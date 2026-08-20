import { formatPct } from '@/lib/format';

/**
 * A single ratio against a limit — a meter, not a one-bar chart.
 * The unfilled track is a pale step of the fill's own hue so the whole bar
 * reads as one scale.
 */
export function ConfidenceMeter({
  pct,
  label,
  showValue = true,
}: {
  /** 0..100 */
  pct: number;
  /** Accessible name, e.g. "Confidence in Over 2.5". */
  label: string;
  showValue?: boolean;
}) {
  const clamped = Math.min(100, Math.max(0, pct));
  return (
    <div className="flex items-center gap-2">
      <div
        role="meter"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={label}
        className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-brand/15"
      >
        <div
          className="h-full rounded-r-[4px] bg-brand"
          style={{ width: `${clamped}%` }}
        />
      </div>
      {showValue && (
        <span className="shrink-0 text-xs tabular-nums text-fg-muted">
          {formatPct(clamped, 0)}
        </span>
      )}
    </div>
  );
}
