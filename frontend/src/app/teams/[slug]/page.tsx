import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import {
  getFeed,
  getLeagues,
  getSettledPredictions,
} from '@/api/client';
import type { MatchWithPredictions, SettledPrediction } from '@/api/types';
import { OutcomeBadge } from '@/components/OutcomeBadge';
import { MARKETS, outcomeLabel } from '@/lib/markets';
import { formatDate } from '@/lib/format';
import { collectTeams, teamSlugMap, type TeamWithLeague } from '@/lib/teams';

export const dynamic = 'force-dynamic';

/**
 * Per-team landing pages: "{Team} predictions" is one of the highest-volume
 * long-tail patterns in this niche. Content is whatever actually exists —
 * priced fixtures and graded picks involving the team — never padding.
 *
 * No aggregate hit rate is computed here on purpose: accuracy rollups live in
 * Postgres and are published once, on /accuracy (AGENTS.md non-negotiable 5).
 * This page links to that record instead of deriving a second number.
 */

async function loadData(): Promise<{
  feed: MatchWithPredictions[];
  ledger: SettledPrediction[];
  slugMap: Map<string, TeamWithLeague>;
}> {
  const [feed, ledger] = await Promise.all([
    getFeed(),
    getSettledPredictions({ limit: 500 }).catch(() => []),
  ]);
  // Leagues are only needed to disambiguate same-named teams by country.
  const leagues = await getLeagues().catch(() => []);
  const leagueById = new Map(leagues.map((l) => [l.id, l]));
  const withLeague = [...collectTeams(feed, ledger).values()].map((entry) => ({
    ...entry,
    league: leagueById.get(entry.league.id) ?? entry.league,
  }));
  return { feed, ledger, slugMap: teamSlugMap(withLeague) };
}

export async function generateMetadata({
  params,
}: PageProps<'/teams/[slug]'>): Promise<Metadata> {
  const { slug } = await params;
  const { slugMap } = await loadData();
  const entry = slugMap.get(slug);
  if (!entry) return { title: 'Team not found' };
  const { team, league } = entry;
  return {
    title: `${team.name} Predictions & Fixtures`,
    description: `Every ${team.name} fixture with a published model prediction, plus ${team.name}'s recent graded picks under the Katafa record — ${league.name}, wins and losses both kept.`,
    alternates: { canonical: `/teams/${slug}` },
  };
}

export default async function TeamPage({ params }: PageProps<'/teams/[slug]'>) {
  const { slug } = await params;
  const { feed, ledger, slugMap } = await loadData();
  const entry = slugMap.get(slug);
  if (!entry) notFound();
  const { team } = entry;

  const upcoming = feed.filter(
    (e) => e.match.homeTeamId === team.id || e.match.awayTeamId === team.id,
  );
  const history = ledger.filter(
    (row) => row.match.homeTeamId === team.id || row.match.awayTeamId === team.id,
  );

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <nav className="text-sm text-fg-muted" aria-label="Breadcrumb">
        <Link href="/fixtures" className="hover:text-fg">
          All fixtures
        </Link>
        <span aria-hidden className="mx-2 text-fg-dim">/</span>
        <span className="text-fg">{team.name}</span>
      </nav>

      <header className="mt-4 max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          {team.name} predictions
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Every priced {team.name} fixture carries a published statistical
          prediction before kickoff. Recent results below are graded
          automatically — the full cross-team record is on the{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            accuracy page
          </Link>
          .
        </p>
      </header>

      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-tight">
          Upcoming {team.name} fixtures
        </h2>
        {upcoming.length === 0 ? (
          <p className="mt-3 text-sm text-fg-muted">
            No upcoming {team.name} matches are priced right now — the next
            round appears as soon as it is ingested.
          </p>
        ) : (
          <div className="mt-4 grid gap-3 lg:grid-cols-2">
            {upcoming.map((e) => (
              <Link
                key={e.match.id}
                href={`/matches/${e.match.id}`}
                className="block rounded-xl border border-line bg-surface p-4 hover:border-brand/40"
              >
                <p className="text-sm font-semibold">
                  {e.homeTeam.name} <span className="text-fg-dim">vs</span>{' '}
                  {e.awayTeam.name}
                </p>
                <p className="mt-1 text-xs text-fg-muted">
                  {formatDate(e.match.kickoffAt)} ·{' '}
                  {e.predictions.length} markets priced
                </p>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          {team.name} prediction history
        </h2>
        <p className="mt-1 text-sm text-fg-muted">
          The most recent graded picks in matches involving {team.name}.
        </p>
        {history.length === 0 ? (
          <p className="mt-3 text-sm text-fg-muted">Nothing settled yet for this team.</p>
        ) : (
          <div className="mt-4 overflow-x-auto rounded-xl border border-line bg-surface">
            <table className="w-full min-w-[40rem] text-left text-sm">
              <caption className="sr-only">
                Recently graded predictions involving {team.name}
              </caption>
              <thead>
                <tr className="border-b border-line text-xs uppercase tracking-wider text-fg-dim">
                  <th scope="col" className="px-4 py-3 font-medium">Match</th>
                  <th scope="col" className="px-4 py-3 font-medium">Score</th>
                  <th scope="col" className="px-4 py-3 font-medium">Market</th>
                  <th scope="col" className="px-4 py-3 font-medium">Pick</th>
                  <th scope="col" className="px-4 py-3 font-medium">Outcome</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {history.slice(0, 12).map((row) => (
                  <tr key={row.prediction.id} className="hover:bg-surface-hi/50">
                    <td className="px-4 py-3">
                      <Link href={`/matches/${row.match.id}`} className="hover:text-brand">
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
                    <td className="px-4 py-3">
                      <OutcomeBadge correct={row.result.wasCorrect} size="sm" />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <p className="mt-10 border-t border-line pt-6 text-sm text-fg-muted">
        The complete, unfiltered record across every team is on the{' '}
        <Link href="/accuracy" className="text-brand hover:underline">
          accuracy page
        </Link>
        , and every competition we price is listed in the footer.
      </p>
    </div>
  );
}
