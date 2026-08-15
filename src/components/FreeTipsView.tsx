import Link from 'next/link';
import type { MarketCode } from '@/api/types';
import { getAccuracySummary, getFreeTips, getPackages, getSlips } from '@/api/client';
import { MARKETS, marketHref } from '@/lib/markets';
import { shouldShowGate } from '@/lib/ads';
import { getSeenAdGates } from '@/lib/session';
import { formatCount, formatDayHeading, formatRate, formatUgx } from '@/lib/format';
import { AdSlot } from './AdSlot';
import { FreeTipsSection } from './FreeTipsSection';
import { MarketAdGate } from './MarketAdGate';
import { MarketTabs } from './MarketTabs';

const TIPS_PER_MARKET = 4;

/**
 * One market's free tips, with the market selector above it.
 *
 * Both the landing page and every /tips/[market] route render this, so each
 * market page carries the same tab bar, ad placements and pro upsell rather
 * than the landing page being the only monetised surface.
 */
export async function FreeTipsView({ market }: { market: MarketCode }) {
  const [tips, accuracy, packages, openSlips, seenGates] = await Promise.all([
    getFreeTips(TIPS_PER_MARKET),
    getAccuracySummary(),
    getPackages(),
    getSlips({ status: 'open' }),
    getSeenAdGates(),
  ]);

  const group = tips.groups.find((g) => g.market === market);
  const marketAccuracy = accuracy.byMarket.find((b) => b.key === market);
  const gated = shouldShowGate(market, seenGates);

  return (
    <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6">
      <section className="max-w-2xl">
        <p className="text-xs font-medium uppercase tracking-wider text-brand">
          Free daily tips
        </p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
          {tips.date ? formatDayHeading(tips.date) : 'No fixtures scheduled'}
          &rsquo;s selections
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          {tips.totalTips} model picks across six markets, free and open to
          everyone. Every one is graded automatically after the final whistle and
          added to the{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            public record
          </Link>{' '}
          — currently {formatRate(accuracy.overall.hitRate)} across{' '}
          {formatCount(accuracy.overall.total)} graded picks.
        </p>
      </section>

      <AdSlot id="feed-top" className="mt-6" />

      <div className="mt-6">
        <MarketTabs active={market} />
      </div>

      {gated ? (
        <MarketAdGate market={market} next={marketHref(market)} />
      ) : (
        <>
          {/* Per-market accuracy sits beside the picks so the reader can weigh
              them without leaving for the dashboard. */}
          {marketAccuracy && marketAccuracy.total > 0 && (
            <p className="mt-4 text-xs leading-relaxed text-fg-muted">
              The model has hit{' '}
              <span className="font-semibold text-fg tabular-nums">
                {formatRate(marketAccuracy.hitRate)}
              </span>{' '}
              on {MARKETS[market].displayName} across{' '}
              {formatCount(marketAccuracy.total)} graded picks.{' '}
              <Link href="/accuracy" className="text-brand hover:underline">
                Full record
              </Link>
            </p>
          )}

          <div className="mt-4">
            {group ? (
              <FreeTipsSection group={group} />
            ) : (
              <p className="rounded-xl border border-dashed border-line bg-surface/50 p-10 text-center text-sm text-fg-muted">
                No {MARKETS[market].displayName} tips clear our publishing
                threshold for this matchday.
              </p>
            )}
          </div>
        </>
      )}

      <AdSlot id="feed-inline" className="mt-6" />

      {/* Pro tier teaser */}
      <section className="mt-8 rounded-xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-xl">
            <h2 className="text-sm font-semibold">
              Want the analysts&rsquo; slips as well?
            </h2>
            <p className="mt-1.5 text-sm leading-relaxed text-fg-muted">
              Our Ugandan analysts publish hand-built slips in three packages.
              You pay per slip, not by subscription — and each analyst&rsquo;s
              full record is public before you spend anything.
            </p>
          </div>
          <Link
            href="/pro"
            className="shrink-0 rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90"
          >
            View packages
          </Link>
        </div>

        <div className="mt-4 grid gap-2 sm:grid-cols-3">
          {packages.map((pkg) => {
            const live = openSlips.filter((s) => s.packageCode === pkg.code);
            const cheapest = live.length ? Math.min(...live.map((s) => s.priceUgx)) : null;
            return (
              <Link
                key={pkg.code}
                href={`/pro#${pkg.code}`}
                className="rounded-lg border border-line-soft bg-canvas/40 px-3 py-2.5 transition-colors hover:border-brand/40"
              >
                <p className="text-sm font-semibold">{pkg.name}</p>
                <p className="mt-0.5 text-xs text-fg-muted">{pkg.tagline}</p>
                <p className="mt-1.5 text-xs tabular-nums text-fg-dim">
                  {cheapest !== null
                    ? `${live.length} live · from ${formatUgx(cheapest)}`
                    : 'No open slip today'}
                </p>
              </Link>
            );
          })}
        </div>
      </section>

      <div className="mt-6 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-surface px-5 py-4">
        <p className="text-sm text-fg-muted">
          This is a shortlist. We price every fixture in all six markets.
        </p>
        <Link
          href="/fixtures"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand/40 hover:text-brand"
        >
          Browse all fixtures →
        </Link>
      </div>

      <p className="mt-4 text-xs leading-relaxed text-fg-dim">
        Odds shown are indicative, derived from the model&rsquo;s own probability
        with a typical margin applied. They are not live bookmaker prices and
        will differ from what you are offered.
      </p>
    </div>
  );
}
