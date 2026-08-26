import type { Metadata } from 'next';
import { Fragment } from 'react';
import Link from 'next/link';
import { CONFIDENCE_BANDS, getAccuracySummary, getLeagues, getSettledPredictions } from '@/api/client';
import { AccuracyTimeline } from '@/components/charts/AccuracyTimeline';
import { CalibrationChart } from '@/components/charts/CalibrationChart';
import { HitRateBars } from '@/components/charts/HitRateBars';
import { OutcomeBadge } from '@/components/OutcomeBadge';
import { MARKETS, outcomeLabel } from '@/lib/markets';
import { leagueSlugMap, leagueHref } from '@/lib/leagues';
import { settledOutcomeLabel } from '@/lib/poisson';
import { formatCount, formatDate, formatRate } from '@/lib/format';
import { matchHref } from '@/lib/matches';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Football Prediction Accuracy Record',
  description:
    'The complete, unfiltered hit-rate record for every football prediction Katafa has published — by market, by league, and over time. Nothing excluded, including the misses.',
  alternates: { canonical: '/accuracy' },
};

const LEDGER_LIMIT = 40;
const OUTCOME_TABS = [
  { key: 'all', label: 'All' },
  { key: 'hit', label: 'Hits only' },
  { key: 'miss', label: 'Misses only' },
] as const;

export default async function AccuracyPage({ searchParams }: PageProps<'/accuracy'>) {
  const params = await searchParams;
  const rawOutcome = Array.isArray(params.outcome) ? params.outcome[0] : params.outcome;
  const outcome =
    rawOutcome === 'hit' || rawOutcome === 'miss' ? rawOutcome : ('all' as const);

  const [accuracy, ledger, leagues] = await Promise.all([
    getAccuracySummary(),
    getSettledPredictions({ outcome, limit: LEDGER_LIMIT }),
    getLeagues().catch(() => []),
  ]);
  const leagueMap = leagueSlugMap(leagues);

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <header className="max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Accuracy record</h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Every prediction ever published, graded automatically against the final
          result. Nothing is excluded, re-scored or deleted — the losing picks are
          in here too, and they stay.
        </p>
      </header>

      {/* Hero figure — the one number this page leads with */}
      <section className="mt-6 rounded-xl border border-line bg-surface p-6">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
              Overall hit rate
            </p>
            <p className="mt-1 text-5xl font-semibold tabular-nums leading-none">
              {formatRate(accuracy.overall.hitRate)}
            </p>
            <p className="mt-2 text-sm text-fg-muted">
              {formatCount(accuracy.overall.correct)} correct from{' '}
              {formatCount(accuracy.overall.total)} graded predictions
            </p>
          </div>
          <dl className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
            <dt className="text-fg-dim">First settled</dt>
            <dd className="text-right tabular-nums">
              {accuracy.firstSettledAt ? formatDate(accuracy.firstSettledAt) : '—'}
            </dd>
            <dt className="text-fg-dim">Last settled</dt>
            <dd className="text-right tabular-nums">
              {accuracy.lastSettledAt ? formatDate(accuracy.lastSettledAt) : '—'}
            </dd>
            <dt className="text-fg-dim">Model</dt>
            <dd className="text-right font-mono text-xs">{accuracy.modelVersion}</dd>
          </dl>
        </div>

        <p className="mt-5 border-t border-line-soft pt-4 text-xs leading-relaxed text-fg-muted">
          A blended figure across six markets is not a like-for-like number —
          Double Chance is far easier to call than 1X2, so the mix matters. Read
          the per-market breakdown below before drawing conclusions from this one.
        </p>
      </section>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-surface px-5 py-4">
        <p className="text-sm text-fg-muted">
          This page covers the <span className="text-fg">model&rsquo;s</span> picks.
          The analysts&rsquo; slips are graded separately, on their own records.
        </p>
        <Link
          href="/analysts"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand/40 hover:text-brand"
        >
          Analyst records →
        </Link>
      </div>

      {/* Trend */}
      <section className="mt-6 rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-semibold">Cumulative hit rate over time</h2>
        <p className="mt-1 text-xs leading-relaxed text-fg-muted">
          All markets combined, running from the first settled prediction.
        </p>
        <div className="mt-4 overflow-x-auto">
          <AccuracyTimeline points={accuracy.timeline} />
        </div>
      </section>

      {/* Breakdowns */}
      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <section className="rounded-xl border border-line bg-surface p-5">
          <h2 className="text-sm font-semibold">By market</h2>
          <p className="mt-1 text-xs leading-relaxed text-fg-muted">
            Hit rate per market type, on a fixed 0–100% scale.
          </p>
          <div className="mt-4">
            <HitRateBars buckets={accuracy.byMarket} />
          </div>
        </section>

        <section className="rounded-xl border border-line bg-surface p-5">
          <h2 className="text-sm font-semibold">By league</h2>
          <p className="mt-1 text-xs leading-relaxed text-fg-muted">
            Where the model is strong, and where coverage is still thin.
          </p>
          <div className="mt-4">
            <HitRateBars buckets={accuracy.byLeague} />
          </div>
        </section>
      </div>

      {/* Calibration */}
      <section className="mt-6 rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-semibold">Is the confidence number honest?</h2>
        <p className="mt-1 max-w-2xl text-xs leading-relaxed text-fg-muted">
          If the model says 70%, it should be right about 70% of the time. This
          groups every graded prediction by the confidence it was published at and
          checks what actually happened.
        </p>
        <div className="mt-4">
          <CalibrationChart buckets={accuracy.byConfidenceBand} bands={CONFIDENCE_BANDS} />
        </div>
      </section>

      {/* Table view — every charted number, reachable without hovering */}
      <details className="mt-4 rounded-xl border border-line bg-surface p-5">
        <summary className="cursor-pointer text-sm font-semibold">
          View all figures as a table
        </summary>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full min-w-[32rem] text-left text-sm">
            <caption className="sr-only">
              Hit rate by market, league and confidence band
            </caption>
            <thead>
              <tr className="border-b border-line text-xs uppercase tracking-wider text-fg-dim">
                <th scope="col" className="py-2 pr-4 font-medium">Group</th>
                <th scope="col" className="py-2 pr-4 font-medium">Correct</th>
                <th scope="col" className="py-2 pr-4 font-medium">Graded</th>
                <th scope="col" className="py-2 font-medium">Hit rate</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {[
                { heading: 'Market', rows: accuracy.byMarket },
                { heading: 'League', rows: accuracy.byLeague },
                { heading: 'Confidence band', rows: accuracy.byConfidenceBand },
              ].map((group) => (
                <Fragment key={group.heading}>
                  <tr className="bg-canvas/40">
                    <th
                      scope="colgroup"
                      colSpan={4}
                      className="py-1.5 text-xs font-medium uppercase tracking-wider text-fg-dim"
                    >
                      {group.heading}
                    </th>
                  </tr>
                  {group.rows.map((row) => {
                    // League rows link to the league's own landing page —
                    // the table is also the crawl path into /leagues/*.
                    const leagueHrefResolved =
                      group.heading === 'League'
                        ? leagueHref(leagueMap, row.key)
                        : null;
                    return (
                      <tr key={`${group.heading}-${row.key}`}>
                        <td className="py-2 pr-4">
                          {leagueHrefResolved ? (
                            <Link href={leagueHrefResolved} className="hover:text-brand hover:underline">
                              {row.label}
                            </Link>
                          ) : (
                            row.label
                          )}
                        </td>
                        <td className="py-2 pr-4 tabular-nums text-fg-muted">
                          {formatCount(row.correct)}
                        </td>
                        <td className="py-2 pr-4 tabular-nums text-fg-muted">
                          {formatCount(row.total)}
                        </td>
                        <td className="py-2 tabular-nums font-medium">{formatRate(row.hitRate)}</td>
                      </tr>
                    );
                  })}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      </details>

      {/* The ledger */}
      <section className="mt-10">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">The ledger</h2>
            <p className="mt-1 text-sm text-fg-muted">
              The {LEDGER_LIMIT} most recently settled picks, exactly as published.
            </p>
          </div>
          <nav className="flex gap-1" aria-label="Filter ledger by outcome">
            {OUTCOME_TABS.map((tab) => (
              <Link
                key={tab.key}
                href={tab.key === 'all' ? '/accuracy' : `/accuracy?outcome=${tab.key}`}
                aria-current={outcome === tab.key ? 'page' : undefined}
                className={`rounded-lg border px-2.5 py-1.5 text-xs transition-colors ${
                  outcome === tab.key
                    ? 'border-brand bg-brand/15 text-fg'
                    : 'border-line bg-surface text-fg-muted hover:text-fg'
                }`}
              >
                {tab.label}
              </Link>
            ))}
          </nav>
        </div>

        <div className="mt-4 overflow-x-auto rounded-xl border border-line bg-surface">
          <table className="w-full min-w-[46rem] text-left text-sm">
            <caption className="sr-only">Recently settled predictions</caption>
            <thead>
              <tr className="border-b border-line text-xs uppercase tracking-wider text-fg-dim">
                <th scope="col" className="px-4 py-3 font-medium">Fixture</th>
                <th scope="col" className="px-4 py-3 font-medium">Result</th>
                <th scope="col" className="px-4 py-3 font-medium">Market</th>
                <th scope="col" className="px-4 py-3 font-medium">Pick</th>
                <th scope="col" className="px-4 py-3 font-medium">Settled as</th>
                <th scope="col" className="px-4 py-3 font-medium">Outcome</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {ledger.map((row) => (
                <tr key={row.prediction.id} className="hover:bg-surface-hi/50">
                  <td className="px-4 py-3">
                    <Link
                      href={matchHref(row.homeTeam, row.awayTeam, row.match.id)}
                      className="hover:text-brand"
                    >
                      {row.homeTeam.shortName} v {row.awayTeam.shortName}
                    </Link>
                    <span className="ml-2 text-xs text-fg-dim">
                      {formatDate(row.result.settledAt)}
                    </span>
                  </td>
                  <td className="px-4 py-3 tabular-nums text-fg-muted">
                    {row.match.homeScore}–{row.match.awayScore}
                  </td>
                  <td className="px-4 py-3 text-fg-muted">
                    {MARKETS[row.prediction.marketType].shortName}
                  </td>
                  <td className="px-4 py-3">
                    {outcomeLabel(row.prediction.marketType, row.prediction.predictionValue)}
                  </td>
                  <td className="px-4 py-3 text-fg-muted">
                    {settledOutcomeLabel(row.prediction.marketType, row.result.actualOutcome)}
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
          <p className="mt-4 text-sm text-fg-muted">No settled predictions match this view.</p>
        )}
      </section>
    </div>
  );
}
