import type { Metadata } from 'next';
import Link from 'next/link';
import { getAccuracySummary, getCoverageStats, getFeed, getLeagues } from '@/api/client';
import { parseFeedFilters } from '@/lib/filters';
import { formatCount, formatDayHeading, formatRate, utcDayKey } from '@/lib/format';
import { FeedFilterBar } from '@/components/FeedFilterBar';
import { MatchCard } from '@/components/MatchCard';
import { StatTile } from '@/components/StatTile';

/**
 * The prediction feed (FR-1). Filters arrive as search params, so the list is
 * already filtered by the time it reaches the browser.
 */
export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Football Fixtures & Match Predictions',
  description:
    'Upcoming football fixtures with a published model prediction attached to every match — filter by league, region and market, and see the reasoning behind each pick.',
  alternates: { canonical: '/fixtures' },
};

export default async function FixturesPage({ searchParams }: PageProps<'/fixtures'>) {
  const params = await searchParams;
  const leagues = await getLeagues();
  const filters = parseFeedFilters(params, leagues.map((l) => l.id));

  const [feed, stats, accuracy] = await Promise.all([
    getFeed(filters),
    getCoverageStats(),
    getAccuracySummary(),
  ]);

  // Group fixtures into matchdays for scanning.
  const byDay = new Map<string, typeof feed>();
  for (const entry of feed) {
    const key = utcDayKey(entry.match.kickoffAt);
    const list = byDay.get(key);
    if (list) list.push(entry);
    else byDay.set(key, [entry]);
  }
  const days = [...byDay.entries()].sort(([a], [b]) => a.localeCompare(b));

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <section className="max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          All fixtures
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Every upcoming fixture we price, across all six markets. The free daily
          shortlist on the{' '}
          <Link href="/" className="text-brand hover:underline">
            front page
          </Link>{' '}
          is a curated slice of this.
        </p>
      </section>

      <div className="mt-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label="Overall hit rate"
          value={formatRate(accuracy.overall.hitRate)}
          note={`${formatCount(accuracy.overall.correct)} of ${formatCount(
            accuracy.overall.total,
          )} graded picks`}
          href="/accuracy"
        />
        <StatTile
          label="Live predictions"
          value={formatCount(stats.livePredictions)}
          note={`Across ${formatCount(stats.upcomingFixtures)} upcoming fixtures`}
        />
        <StatTile
          label="Leagues covered"
          value={formatCount(stats.leagues)}
          note="Including Uganda and Kenya top flights"
        />
        <StatTile
          label="Model version"
          value={stats.modelVersion.replace('poisson-', 'v')}
          note="Poisson goal expectation"
          href="/methodology"
        />
      </div>

      <div className="mt-8">
        <FeedFilterBar
          leagues={leagues}
          filters={filters}
          resultCount={feed.length}
          basePath="/fixtures"
        />
      </div>

      {days.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-line bg-surface/50 p-10 text-center">
          <p className="text-sm font-medium">No fixtures match these filters</p>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-fg-muted">
            Try lowering the confidence threshold or selecting more leagues. The
            feed only shows fixtures that still have a published market after
            filtering.
          </p>
        </div>
      ) : (
        <div className="mt-8 space-y-10">
          {days.map(([day, entries]) => (
            <section key={day}>
              <div className="flex items-baseline justify-between gap-3 border-b border-line pb-2">
                <h2 className="text-sm font-semibold">{formatDayHeading(day)}</h2>
                <span className="text-xs tabular-nums text-fg-dim">
                  {entries.length} {entries.length === 1 ? 'fixture' : 'fixtures'}
                </span>
              </div>
              <div className="mt-4 grid gap-3 lg:grid-cols-2">
                {entries.map((entry) => (
                  <MatchCard key={entry.match.id} entry={entry} />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
