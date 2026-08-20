import type { Scoreline } from '@/lib/poisson';
import { formatPct } from '@/lib/format';

/**
 * Most likely exact scorelines straight off the Poisson matrix.
 *
 * Correct Score is deliberately not an MVP market (PRD 3.2), so these are shown
 * as reasoning only — they are the same numbers every published market is
 * summed from.
 */
export function ScorelineList({
  scorelines,
  homeShort,
  awayShort,
}: {
  scorelines: Scoreline[];
  homeShort: string;
  awayShort: string;
}) {
  const max = Math.max(...scorelines.map((s) => s.probability), 0.0001);

  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <h3 className="text-sm font-semibold">Most likely scorelines</h3>
      <p className="mt-1 text-xs leading-relaxed text-fg-muted">
        {homeShort} first. Not a published market — this is the matrix every
        market above is summed from.
      </p>
      <ul className="mt-3 space-y-2">
        {scorelines.map((s) => (
          <li key={`${s.home}-${s.away}`} className="grid grid-cols-[3.5rem_1fr_3rem] items-center gap-3">
            <span className="font-mono text-xs tabular-nums text-fg">
              {s.home}–{s.away}
            </span>
            <span className="h-2 overflow-hidden rounded-full bg-line-soft">
              <span
                className="block h-full rounded-r-[4px] bg-brand/70"
                style={{ width: `${(s.probability / max) * 100}%` }}
              />
            </span>
            <span className="text-right text-xs tabular-nums text-fg-muted">
              {formatPct(s.probability * 100, 1)}
            </span>
          </li>
        ))}
      </ul>
      <p className="sr-only">
        Bars are scaled to the most likely scoreline, {awayShort} away.
      </p>
    </div>
  );
}
