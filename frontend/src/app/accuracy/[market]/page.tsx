import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import {
  CONFIDENCE_BANDS,
  getAccuracySummary,
  getSettledPredictions,
} from '@/api/client';
import type { MarketCode } from '@/api/types';
import { AccuracyTimeline } from '@/components/charts/AccuracyTimeline';
import { CalibrationChart } from '@/components/charts/CalibrationChart';
import { HitRateBars } from '@/components/charts/HitRateBars';
import { OutcomeBadge } from '@/components/OutcomeBadge';
import { MARKETS, outcomeLabel } from '@/lib/markets';
import { formatCount, formatRate } from '@/lib/format';

export const dynamic = 'force-dynamic';

/** Every market gets an accuracy landing page — the set is fixed and tiny. */
export function generateStaticParams() {
  return Object.values(MARKETS).map((m) => ({ market: m.slug }));
}

const SLUG_TO_CODE = new Map<string, MarketCode>(
  Object.values(MARKETS).map((m) => [m.slug, m.code]),
);

/**
 * Per-market search-intent copy. Unique text per market, not a template —
 * each answers what a searcher typing e.g. "double chance accuracy" actually
 * wants to know about that specific market.
 */
const MARKET_COPY: Record<MarketCode, string> = {
  ONE_X_TWO:
    'Match result is the hardest of our markets: three outcomes, no safety net, draws included. It is also the market people mean when they ask how good a football prediction model really is.',
  DOUBLE_CHANCE:
    'Double Chance covers two of the three possible results, so it hits far more often than straight match-result picks — and pays less. Judge it against its own odds, never against a 1X2 hit rate.',
  BTTS:
    'Both Teams To Score ignores who wins entirely: it resolves purely on whether both sides find the net. Attacking strength and defensive weakness drive it more than league position does.',
  OVER_UNDER_1_5:
    'The 1.5 goal line is the most forgiving goals market — it misses only when a match finishes 1–0 or 0–0. High hit rate, low reward, and a useful sanity check on the model’s goal expectations.',
  OVER_UNDER_2_5:
    'Over/Under 2.5 is the classic goals line: three or more goals wins Over, two or fewer wins Under. It splits roughly down the middle across football, so edges here say the most about the model.',
  OVER_UNDER_3_5:
    'The 3.5 line needs four goals for Over to land. It is the lowest-frequency market we publish, which makes its record the thinnest — read it alongside the sample size, not just the percentage.',
};

export async function generateMetadata({
  params,
}: PageProps<'/accuracy/[market]'>): Promise<Metadata> {
  const { market: slug } = await params;
  const code = SLUG_TO_CODE.get(slug);
  if (!code) return { title: 'Market not found' };
  return {
    title: `${MARKETS[code].displayName} Prediction Accuracy`,
    description: `Katafa's complete ${MARKETS[code].displayName.toLowerCase()} prediction record — every graded pick counted, wins and losses alike — plus the model's hit rate over time and by confidence band.`,
    alternates: { canonical: `/accuracy/${MARKETS[code].slug}` },
  };
}

export default async function MarketAccuracyPage({
  params,
}: PageProps<'/accuracy/[market]'>) {
  const { market: slug } = await params;
  const code = SLUG_TO_CODE.get(slug);
  if (!code) notFound();

  const [accuracy, ledger] = await Promise.all([
    getAccuracySummary(),
    getSettledPredictions({ markets: [code], limit: 40 }),
  ]);

  const bucket = accuracy.byMarket.find((b) => b.key === code);
  const otherMarkets = Object.values(MARKETS).filter((m) => m.code !== code);

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <nav className="text-sm text-fg-muted" aria-label="Breadcrumb">
        <Link href="/accuracy" className="hover:text-fg">
          Accuracy record
        </Link>
        <span aria-hidden className="mx-2 text-fg-dim">/</span>
        <span className="text-fg">{MARKETS[code].displayName}</span>
      </nav>

      <header className="mt-4 max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          {MARKETS[code].displayName} prediction accuracy
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">{MARKET_COPY[code]}</p>
      </header>

      <section className="mt-6 rounded-xl border border-line bg-surface p-6">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
              Hit rate in this market
            </p>
            <p className="mt-1 text-5xl font-semibold tabular-nums leading-none">
              {bucket ? formatRate(bucket.hitRate) : '—'}
            </p>
            <p className="mt-2 text-sm text-fg-muted">
              {bucket
                ? `${formatCount(bucket.correct)} correct from ${formatCount(bucket.total)} graded predictions`
                : 'No graded predictions in this market yet'}
            </p>
          </div>
          <Link
            href={`/tips/${MARKETS[code].slug}`}
            className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand/40 hover:text-brand"
          >
            Today&rsquo;s {MARKETS[code].shortName} picks →
          </Link>
        </div>
        <p className="mt-5 border-t border-line-soft pt-4 text-xs leading-relaxed text-fg-muted">
          Every published pick in this market is counted here — nothing excluded,
          losses kept. The cross-market comparison lives on the{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            main accuracy page
          </Link>
          .
        </p>
      </section>

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <section className="rounded-xl border border-line bg-surface p-5">
          <h2 className="text-sm font-semibold">Against every market</h2>
          <div className="mt-4">
            <HitRateBars buckets={accuracy.byMarket} />
          </div>
        </section>
        <section className="rounded-xl border border-line bg-surface p-5">
          <h2 className="text-sm font-semibold">Cumulative hit rate over time</h2>
          <p className="mt-1 mb-4 text-xs leading-relaxed text-fg-muted">
            All markets combined — this market&rsquo;s history is too thin to chart alone.
          </p>
          <AccuracyTimeline points={accuracy.timeline} />
        </section>
      </div>

      <section className="mt-6 rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-semibold">Is the confidence number honest?</h2>
        <p className="mt-1 max-w-2xl text-xs leading-relaxed text-fg-muted">
          If a pick says 70%, it should win about 70% of the time. Calibration
          is checked across all markets, because confidence means the same thing
          everywhere.
        </p>
        <div className="mt-4">
          <CalibrationChart buckets={accuracy.byConfidenceBand} bands={CONFIDENCE_BANDS} />
        </div>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          Recent {MARKETS[code].shortName} picks
        </h2>
        <div className="mt-4 overflow-x-auto rounded-xl border border-line bg-surface">
          <table className="w-full min-w-[40rem] text-left text-sm">
            <caption className="sr-only">
              Recently settled {MARKETS[code].displayName} predictions
            </caption>
            <thead>
              <tr className="border-b border-line text-xs uppercase tracking-wider text-fg-dim">
                <th scope="col" className="px-4 py-3 font-medium">Fixture</th>
                <th scope="col" className="px-4 py-3 font-medium">Result</th>
                <th scope="col" className="px-4 py-3 font-medium">Pick</th>
                <th scope="col" className="px-4 py-3 font-medium">Outcome</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {ledger.map((row) => (
                <tr key={row.prediction.id} className="hover:bg-surface-hi/50">
                  <td className="px-4 py-3">
                    <Link href={`/matches/${row.match.id}`} className="hover:text-brand">
                      {row.homeTeam.shortName} v {row.awayTeam.shortName}
                    </Link>
                    <span className="ml-2 text-xs text-fg-dim">{row.league.shortName}</span>
                  </td>
                  <td className="px-4 py-3 tabular-nums text-fg-muted">
                    {row.match.homeScore}–{row.match.awayScore}
                  </td>
                  <td className="px-4 py-3">
                    {outcomeLabel(code, row.prediction.predictionValue)}
                  </td>
                  <td className="px-4 py-3">
                    <OutcomeBadge correct={row.result.wasCorrect} size="sm" />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {ledger.length === 0 && (
          <p className="mt-4 text-sm text-fg-muted">
            Nothing settled in this market yet.
          </p>
        )}
      </section>

      <section className="mt-12 border-t border-line pt-6">
        <h2 className="text-sm font-semibold">Accuracy in other markets</h2>
        <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-sm">
          {otherMarkets.map((other) => (
            <li key={other.code}>
              <Link
                href={`/accuracy/${other.slug}`}
                className="text-fg-muted hover:text-brand hover:underline"
              >
                {other.displayName} accuracy
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
