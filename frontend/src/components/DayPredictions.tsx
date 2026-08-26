import Link from 'next/link';
import { getFeed } from '@/api/client';
import { formatDayHeading, utcDayKey } from '@/lib/format';
import { LeagueChip } from '@/components/LeagueChip';
import { MatchCard } from '@/components/MatchCard';

function dayKeyFor(offset: number): string {
  return new Date(Date.now() + offset * 86_400_000).toISOString().slice(0, 10);
}

/**
 * Shared body for the /predictions/today and /predictions/tomorrow landing
 * pages. Shows *every* fixture we price on the target UTC day — the free
 * shortlist on '/' is a curated slice of the same data, so these pages are
 * complementary to it, not duplicates of it.
 */
export async function DayPredictions({ offset }: { offset: number }) {
  const feed = await getFeed();
  const targetKey = dayKeyFor(offset);

  const entries = feed.filter((entry) => utcDayKey(entry.match.kickoffAt) === targetKey);

  // Group by league for scanning; East Africa first, matching LeagueChip's
  // regional emphasis. Leagues with nothing on this day are omitted.
  const eastAfricaFirst = (region: string) => (region === 'east-africa' ? 0 : 1);
  const byLeague = new Map<string, typeof entries>();
  for (const entry of entries) {
    const list = byLeague.get(entry.league.id);
    if (list) list.push(entry);
    else byLeague.set(entry.league.id, [entry]);
  }
  const groups = [...byLeague.values()].sort(
    (a, b) => eastAfricaFirst(a[0].league.region) - eastAfricaFirst(b[0].league.region),
  );

  return (
    <>
      <p className="mt-3 text-[15px] leading-relaxed text-fg-muted max-w-2xl">
        Every fixture we have priced for this day, with the model&rsquo;s pick in
        each market. The{' '}
        <Link href="/" className="text-brand hover:underline">
          free daily shortlist
        </Link>{' '}
        is a curated selection of these.
      </p>

      {entries.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-line bg-surface/50 p-10 text-center">
          <p className="text-sm font-medium">No priced fixtures for this day yet</p>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-fg-muted">
            Rounds are priced several days ahead, but not every day has matches.
            The full window is always on{' '}
            <Link href="/fixtures" className="text-brand hover:underline">
              all fixtures
            </Link>
            .
          </p>
        </div>
      ) : (
        <div className="mt-8 space-y-10">
          {groups.map((group) => (
            <section key={group[0].league.id}>
              <div className="flex items-baseline justify-between gap-3 border-b border-line pb-2">
                <h2 className="text-sm font-semibold">
                  <LeagueChip league={group[0].league} />
                </h2>
                <span className="text-xs tabular-nums text-fg-dim">
                  {group.length} {group.length === 1 ? 'fixture' : 'fixtures'}
                </span>
              </div>
              <div className="mt-4 grid gap-3 lg:grid-cols-2">
                {group.map((entry) => (
                  <MatchCard key={entry.match.id} entry={entry} />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </>
  );
}

/** Heading block, kept identical across the two pages except the date. */
export function DayPredictionsHeader({ offset }: { offset: number }) {
  const key = dayKeyFor(offset);
  return (
    <header className="max-w-2xl">
      <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
        {formatDayHeading(key)} · UTC
      </p>
      <h1 className="mt-1 text-3xl font-semibold tracking-tight sm:text-4xl">
        Football predictions for {key}
      </h1>
    </header>
  );
}
