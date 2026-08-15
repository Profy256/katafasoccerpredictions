import type { MarketCode } from '@/api/types';
import { MARKETS } from '@/lib/markets';
import { acknowledgeAdGateAction } from '@/app/actions';

/**
 * The interstitial shown before a gated market's tips.
 *
 * The tips are not rendered behind this — the page returns this instead of the
 * data, so the selections are never in the HTML. If the gate were only an
 * overlay, anyone could read the picks out of view-source and the ad would be
 * worth nothing.
 *
 * The continue button currently just records the acknowledgement. Wire it to
 * the ad network's completion callback so it cannot be clicked past.
 */
export function MarketAdGate({
  market,
  next,
}: {
  market: MarketCode;
  next: string;
}) {
  return (
    <section className="mt-6 rounded-xl border border-line bg-surface p-6 text-center">
      <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
        Sponsored
      </p>
      <h2 className="mt-2 text-lg font-semibold tracking-tight">
        {MARKETS[market].displayName} tips are free — after a short ad
      </h2>
      <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-fg-muted">
        Ads keep these tips free to read. This takes a few seconds and only
        happens once for this market.
      </p>

      <div className="mx-auto mt-6 flex h-52 max-w-lg items-center justify-center rounded-xl border border-line-soft bg-canvas/60">
        <span className="text-[10px] uppercase tracking-wider text-fg-dim">
          Ad unit renders here
        </span>
      </div>

      <form action={acknowledgeAdGateAction} className="mt-6">
        <input type="hidden" name="market" value={market} />
        <input type="hidden" name="next" value={next} />
        <button
          type="submit"
          className="rounded-lg bg-brand px-4 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Continue to tips
        </button>
      </form>
    </section>
  );
}
