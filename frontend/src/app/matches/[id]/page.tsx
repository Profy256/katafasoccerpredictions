import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound, permanentRedirect } from 'next/navigation';
import { getFeed, getLeagues, getMatchDetail, getSettledPredictions } from '@/api/client';
import type { MatchDetail, Team } from '@/api/types';
import { LeagueChip } from '@/components/LeagueChip';
import { MarketBreakdown } from '@/components/MarketBreakdown';
import { ScorelineList } from '@/components/ScorelineList';
import { TeamFormPanel } from '@/components/TeamFormPanel';
import { formatDate, formatDateTime, formatPct } from '@/lib/format';
import { leagueHref, leagueSlugMap } from '@/lib/leagues';
import { MARKETS, marketHref, outcomeLabel } from '@/lib/markets';
import { matchIdFromSlug, matchSlug } from '@/lib/matches';
import { resolveTeamSlugs, teamHref } from '@/lib/teams';

export const dynamic = 'force-dynamic';

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/**
 * A one-line, plain-language statement of what the model expects. This is the
 * sentence a search result can quote and the first thing a reader arriving
 * from "{home} vs {away} prediction" is looking for, so it says the pick and
 * its probability outright rather than making them read a table.
 */
function headlineCall(detail: MatchDetail): string | null {
  const oneXTwo = detail.predictions.find((p) => p.marketType === 'ONE_X_TWO');
  if (!oneXTwo) return null;
  const pick = outcomeLabel('ONE_X_TWO', oneXTwo.predictionValue);
  const side =
    oneXTwo.predictionValue === 'HOME'
      ? detail.homeTeam.name
      : oneXTwo.predictionValue === 'AWAY'
        ? detail.awayTeam.name
        : 'the draw';
  const goals = (detail.reasoning.xgHome + detail.reasoning.xgAway).toFixed(2);
  return `The model's pick is ${pick} — ${side} at ${formatPct(oneXTwo.confidencePct)}, off an expected ${goals} goals in the match.`;
}

export async function generateMetadata({
  params,
}: PageProps<'/matches/[id]'>): Promise<Metadata> {
  const { id } = await params;
  const detail = await getMatchDetail(matchIdFromSlug(id));
  if (!detail) return { title: 'Match not found' };
  const { homeTeam, awayTeam, league, match } = detail;
  const call = headlineCall(detail);
  return {
    title: `${homeTeam.name} vs ${awayTeam.name} Prediction`,
    description: call
      ? `${call} Every market priced before kickoff, with the form, expected goals and scorelines behind it — ${league.name}, ${formatDate(match.kickoffAt)}.`
      : `Katafa's published predictions for ${homeTeam.name} versus ${awayTeam.name} in the ${league.name} — every market picked before kickoff and graded against the result afterwards.`,
    alternates: {
      canonical: `/matches/${matchSlug(homeTeam, awayTeam, match.id)}`,
    },
  };
}

/**
 * SportsEvent for the fixture itself — the part of this page that is plain
 * fact and safe to hand a crawler verbatim. The predictions are deliberately
 * *not* marked up as claims or offers: they are estimates, and there is no
 * schema.org type that says so without overstating them.
 */
function eventJsonLd(detail: MatchDetail, canonical: string) {
  const { match, league, homeTeam, awayTeam } = detail;
  return {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'SportsEvent',
        name: `${homeTeam.name} vs ${awayTeam.name}`,
        url: canonical,
        startDate: match.kickoffAt,
        eventAttendanceMode: 'https://schema.org/OfflineEventAttendanceMode',
        sport: 'Association football',
        homeTeam: { '@type': 'SportsTeam', name: homeTeam.name },
        awayTeam: { '@type': 'SportsTeam', name: awayTeam.name },
        superEvent: {
          '@type': 'SportsOrganization',
          name: league.name,
        },
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          { '@type': 'ListItem', position: 1, name: 'Predictions', item: SITE_URL },
          { '@type': 'ListItem', position: 2, name: league.name },
          {
            '@type': 'ListItem',
            position: 3,
            name: `${homeTeam.name} vs ${awayTeam.name}`,
            item: canonical,
          },
        ],
      },
    ],
  };
}

/** Match detail with the reasoning behind every pick (FR-4). */
export default async function MatchPage({ params }: PageProps<'/matches/[id]'>) {
  const { id } = await params;
  const detail = await getMatchDetail(matchIdFromSlug(id));
  if (!detail) notFound();

  const { match, league, homeTeam, awayTeam, predictions, results, reasoning } = detail;

  // Old `/matches/{uuid}` links, and anything that guessed the readable half
  // wrong, land on the canonical slug instead of quietly serving a duplicate.
  const canonicalSlug = matchSlug(homeTeam, awayTeam, match.id);
  if (id !== canonicalSlug) permanentRedirect(`/matches/${canonicalSlug}`);

  // Sibling landing pages this fixture belongs to. Both fall back to plain
  // text when the team or competition has no page of its own, so a missing
  // link is never a broken one.
  const [leagues, feed, ledger] = await Promise.all([
    getLeagues().catch(() => []),
    getFeed().catch(() => []),
    getSettledPredictions({ limit: 500 }).catch(() => []),
  ]);
  const leaguePage = leagueHref(leagueSlugMap(leagues), league.id);
  const teams = resolveTeamSlugs(feed, ledger, leagues);
  const teamPage = (team: Team) => teamHref(teams, team.id);

  const resultByPrediction = new Map((results ?? []).map((r) => [r.predictionId, r]));
  const finished = match.status === 'finished';
  const maxXg = Math.max(reasoning.xgHome, reasoning.xgAway);
  const teamName = (teamId: string) => (teamId === homeTeam.id ? homeTeam.name : awayTeam.name);
  const call = headlineCall(detail);

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* Static JSON-LD built from our own data, no user input. */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify(
            eventJsonLd(detail, `${SITE_URL}/matches/${canonicalSlug}`),
          ),
        }}
      />

      <nav className="text-sm text-fg-muted" aria-label="Breadcrumb">
        <Link href="/" className="hover:text-fg">
          Predictions
        </Link>
        <span className="mx-2 text-fg-dim">/</span>
        {leaguePage ? (
          <Link href={leaguePage} className="hover:text-fg">
            {league.name}
          </Link>
        ) : (
          <span>{league.name}</span>
        )}
      </nav>

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
            {homeTeam.name} <span className="text-fg-dim">vs</span> {awayTeam.name}{' '}
            <span className="text-fg-muted">prediction</span>
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

        {call && <p className="mt-3 max-w-2xl text-sm leading-relaxed">{call}</p>}

        <p className="mt-3 text-xs leading-relaxed text-fg-muted">
          {finished ? (
            <>
              This fixture has been played and every prediction below was graded
              automatically against the result. Nothing has been edited after
              the fact.
            </>
          ) : (
            <>
              Published before kickoff and graded automatically once the match
              is played, win or lose — see the{' '}
              <Link href="/accuracy" className="text-brand hover:underline">
                full accuracy record
              </Link>{' '}
              and{' '}
              <Link href="/methodology" className="text-brand hover:underline">
                how the model works
              </Link>
              .
            </>
          )}
        </p>
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

      {/* Where this fixture sits in the rest of the site. Every match page is
          a crawl entry point into its two teams, its competition and the
          market pages — without this, /teams/* and /markets are reachable
          only from the footer. */}
      <nav className="mt-8 rounded-xl border border-line bg-surface p-5" aria-label="Related">
        <h2 className="text-sm font-semibold">More predictions</h2>
        <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-2 text-sm">
          {[homeTeam, awayTeam].map((team) => {
            const href = teamPage(team);
            return href ? (
              <li key={team.id}>
                <Link href={href} className="text-brand hover:underline">
                  {team.name} predictions
                </Link>
              </li>
            ) : null;
          })}
          {leaguePage && (
            <li>
              <Link href={leaguePage} className="text-brand hover:underline">
                {league.name} predictions
              </Link>
            </li>
          )}
          <li>
            <Link href="/predictions/today" className="text-brand hover:underline">
              Today&rsquo;s predictions
            </Link>
          </li>
        </ul>

        <p className="mt-4 text-xs font-medium uppercase tracking-wider text-fg-dim">
          By market
        </p>
        <ul className="mt-2 flex flex-wrap gap-x-5 gap-y-2 text-sm">
          {Object.values(MARKETS).map((m) => (
            <li key={m.code}>
              <Link href={marketHref(m.code)} className="text-fg-muted hover:text-brand hover:underline">
                {m.displayName}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  );
}
