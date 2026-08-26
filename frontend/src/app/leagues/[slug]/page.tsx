import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getAccuracySummary, getFeed, getLeagues } from '@/api/client';
import {
  formatCount,
  formatDayHeading,
  formatRate,
  utcDayKey,
} from '@/lib/format';
import { leagueSlugMap } from '@/lib/leagues';
import { MatchCard } from '@/components/MatchCard';
import { StatTile } from '@/components/StatTile';

export const dynamic = 'force-dynamic';

/**
 * Per-league landing pages. Each competition gets one crawlable URL that
 * answers "{League} predictions" with this league's fixtures, the model's
 * picks and its graded hit rate in that league — instead of leaving that
 * intent to a filter parameter on /fixtures that search engines can't index.
 */
export async function generateMetadata({
  params,
}: PageProps<'/leagues/[slug]'>): Promise<Metadata> {
  const { slug } = await params;
  const leagues = await getLeagues();
  const map = leagueSlugMap(leagues);
  const league = map.get(slug);
  if (!league) return { title: 'League not found' };
  return {
    title: `${league.name} Predictions & Fixtures`,
    description: `Free ${league.name} (${league.country}) predictions for every fixture — model probabilities across Match Result, Double Chance, BTTS and Over/Under, plus Katafa's graded accuracy record for this league.`,
    alternates: { canonical: `/leagues/${slug}` },
  };
}

export default async function LeaguePage({
  params,
}: PageProps<'/leagues/[slug]'>) {
  const { slug } = await params;
  const leagues = await getLeagues();
  const map = leagueSlugMap(leagues);
  const league = map.get(slug);
  if (!league) notFound();

  const [feed, accuracy] = await Promise.all([
    getFeed({ leagueIds: [league.id] }),
    getAccuracySummary(),
  ]);

  // The accuracy rollup buckets are keyed by league id.
  const bucket = accuracy.byLeague.find((b) => b.key === league.id);
  const others = leagues.filter((l) => l.id !== league.id);

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
      <nav className="text-sm text-fg-muted" aria-label="Breadcrumb">
        <Link href="/fixtures" className="hover:text-fg">
          All fixtures
        </Link>
        <span aria-hidden className="mx-2 text-fg-dim">
          /
        </span>
        <span className="text-fg">{league.name}</span>
      </nav>

      <header className="mt-4 max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          {league.name} predictions
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          A published statistical prediction on every upcoming{' '}
          {league.name} fixture ({league.country}), across all six markets. Every
          pick is graded against the real result afterwards — see where this
          league sits in the full{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            accuracy record
          </Link>
          .
        </p>
      </header>

      <div className="mt-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label="Hit rate here"
          value={bucket ? formatRate(bucket.hitRate) : '—'}
          note={
            bucket
              ? `${formatCount(bucket.correct)} of ${formatCount(bucket.total)} graded`
              : 'No graded picks yet'
          }
          href="/accuracy"
        />
        <StatTile
          label="Upcoming fixtures"
          value={formatCount(feed.length)}
          note={`In the ${league.name}`}
        />
        <StatTile
          label="Country"
          value={league.countryCode}
          note={`${league.country} · tier ${league.tier}`}
        />
        <StatTile
          label="Model version"
          value={accuracy.modelVersion.replace('poisson-', 'v')}
          note="Poisson goal expectation"
          href="/methodology"
        />
      </div>

      {days.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-line bg-surface/50 p-10 text-center">
          <p className="text-sm font-medium">No published {league.name} fixtures right now</p>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-fg-muted">
            Predictions appear as soon as the next round is priced. Other
            competitions already have open picks — try the{' '}
            <Link href="/" className="text-brand hover:underline">
              daily shortlist
            </Link>{' '}
            or{' '}
            <Link href="/fixtures" className="text-brand hover:underline">
              all fixtures
            </Link>
            .
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

      {others.length > 0 && (
        <section className="mt-12 border-t border-line pt-6">
          <h2 className="text-sm font-semibold">Predictions by league</h2>
          <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-sm">
            {others.map((other) => {
              const otherSlug = [...map.entries()].find(
                ([, l]) => l.id === other.id,
              )?.[0];
              if (!otherSlug) return null;
              return (
                <li key={other.id}>
                  <Link
                    href={`/leagues/${otherSlug}`}
                    className="text-fg-muted hover:text-brand hover:underline"
                  >
                    {other.name}
                  </Link>
                </li>
              );
            })}
          </ul>
        </section>
      )}
    </div>
  );
}
