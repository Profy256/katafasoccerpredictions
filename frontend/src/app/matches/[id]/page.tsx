import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getMatchDetail } from '@/api/client';
import { LeagueChip } from '@/components/LeagueChip';
import { MarketBreakdown } from '@/components/MarketBreakdown';
import { ScorelineList } from '@/components/ScorelineList';
import { TeamFormPanel } from '@/components/TeamFormPanel';
import { formatDate, formatDateTime } from '@/lib/format';

export const dynamic = 'force-dynamic';

export async function generateMetadata({
  params,
}: PageProps<'/matches/[id]'>): Promise<Metadata> {
  const { id } = await params;
  const detail = await getMatchDetail(id);
  if (!detail) return { title: 'Match not found' };
  const { homeTeam, awayTeam, league } = detail;
  return {
    title: `${homeTeam.name} vs ${awayTeam.name} Prediction`,
    description: `Katafa's published predictions for ${homeTeam.name} versus ${awayTeam.name} in the ${league.name} — every market picked before kickoff and graded against the result afterwards.`,
    alternates: { canonical: `/matches/${id}` },
  };
}

/** Match detail with the reasoning behind every pick (FR-4). */
export default async function MatchPage({ params }: PageProps<'/matches/[id]'>) {
  const { id } = await params;
  const detail = await getMatchDetail(id);
  if (!detail) notFound();

  const { match, league, homeTeam, awayTeam, predictions, results, reasoning } = detail;
  const resultByPrediction = new Map((results ?? []).map((r) => [r.predictionId, r]));
  const finished = match.status === 'finished';
  const maxXg = Math.max(reasoning.xgHome, reasoning.xgAway);
  const teamName = (teamId: string) => (teamId === homeTeam.id ? homeTeam.name : awayTeam.name);

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <Link href="/" className="text-sm text-fg-muted hover:text-fg">
        ← All predictions
      </Link>

      {/* Fixture header */}
      <header className="mt-4 rounded-xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <LeagueChip league={league} />
          <span className="text-xs text-fg-dim">Round {match.round + 1}</span>
          <span className="text-xs text-fg-dim">·</span>
          <span className="text-xs tabular-nums text-fg-muted">
            {formatDateTime(match.kickoffAt)} UTC
          </span>
        </div>

        <div className="mt-4 flex flex-wrap items-end justify-between gap-4">
          <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
            {homeTeam.name}{' '}
            <span className="text-fg-dim">vs</span> {awayTeam.name}
          </h1>
          {finished && match.homeScore !== null && match.awayScore !== null && (
            <div className="text-right">
              <p className="text-xs uppercase tracking-wider text-fg-dim">Full time</p>
              <p className="text-2xl font-semibold tabular-nums">
                {match.homeScore}–{match.awayScore}
              </p>
            </div>
          )}
        </div>

        {finished && (
          <p className="mt-3 text-xs leading-relaxed text-fg-muted">
            This fixture has been played and every prediction below was graded
            automatically against the result. Nothing has been edited after the
            fact.
          </p>
        )}
      </header>

      {/* Expected goals — the single number everything else derives from */}
      <section className="mt-6 rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-semibold">Expected goals</h2>
        <p className="mt-1 text-xs leading-relaxed text-fg-muted">
          Each side&rsquo;s attacking strength multiplied by the opponent&rsquo;s
          defensive weakness and the league&rsquo;s venue baseline. Every market
          on this page is derived from these two numbers.
        </p>

        <div className="mt-4 space-y-3">
          {[
            { name: homeTeam.name, xg: reasoning.xgHome, venue: 'Home' },
            { name: awayTeam.name, xg: reasoning.xgAway, venue: 'Away' },
          ].map((row) => (
            <div key={row.venue} className="grid grid-cols-[1fr_auto] items-center gap-3">
              <div className="min-w-0">
                <div className="flex items-baseline justify-between gap-3">
                  <p className="truncate text-sm">{row.name}</p>
                  <p className="text-lg font-semibold tabular-nums">{row.xg.toFixed(2)}</p>
                </div>
                <span className="mt-1.5 block h-2 overflow-hidden rounded-full bg-line-soft">
                  <span
                    className="block h-full rounded-r-[4px] bg-brand"
                    style={{ width: `${(row.xg / maxXg) * 100}%` }}
                  />
                </span>
              </div>
            </div>
          ))}
        </div>

        <p className="mt-4 border-t border-line-soft pt-3 text-xs leading-relaxed text-fg-muted">
          Built from {reasoning.sampleSize.home} prior {homeTeam.name} matches and{' '}
          {reasoning.sampleSize.away} prior {awayTeam.name} matches · model{' '}
          <span className="font-mono">{reasoning.modelVersion}</span>
        </p>
      </section>

      {/* Published markets */}
      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-tight">Published markets</h2>
        <p className="mt-1 text-sm text-fg-muted">
          The model&rsquo;s selection in every market, published before kickoff
          and graded automatically once the match is played.
        </p>
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {predictions.map((prediction) => (
            <MarketBreakdown
              key={prediction.id}
              prediction={prediction}
              result={resultByPrediction.get(prediction.id)}
            />
          ))}
        </div>
      </section>

      {/* Reasoning inputs */}
      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-tight">What went into it</h2>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <TeamFormPanel team={homeTeam} form={reasoning.homeForm} />
          <TeamFormPanel team={awayTeam} form={reasoning.awayForm} />
        </div>

        <div className="mt-3 grid gap-3 md:grid-cols-2">
          <ScorelineList
            scorelines={reasoning.topScorelines}
            homeShort={homeTeam.shortName}
            awayShort={awayTeam.shortName}
          />

          <div className="rounded-xl border border-line bg-surface p-4">
            <h3 className="text-sm font-semibold">Head to head</h3>
            {reasoning.headToHead.length === 0 ? (
              <p className="mt-2 text-xs leading-relaxed text-fg-muted">
                No previous meetings inside the data window. The model weights
                team strength rather than head-to-head history, so this is
                context only.
              </p>
            ) : (
              <>
                <p className="mt-1 text-xs leading-relaxed text-fg-muted">
                  Context only — head-to-head is not an input to the model.
                </p>
                <ul className="mt-3 divide-y divide-line-soft">
                  {reasoning.headToHead.map((h2h) => (
                    <li
                      key={h2h.matchId}
                      className="flex items-center justify-between gap-3 py-2 text-xs"
                    >
                      <span className="truncate text-fg-muted">
                        {teamName(h2h.homeTeamId)} vs {teamName(h2h.awayTeamId)}
                      </span>
                      <span className="shrink-0 tabular-nums">
                        <span className="font-semibold">
                          {h2h.homeScore}–{h2h.awayScore}
                        </span>
                        <span className="ml-2 text-fg-dim">{formatDate(h2h.kickoffAt)}</span>
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
