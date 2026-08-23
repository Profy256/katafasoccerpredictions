import Link from 'next/link';
import type { FreeTipGroup } from '@/api/types';
import { MARKETS, outcomeLabel } from '@/lib/markets';
import { formatOdds, formatTime } from '@/lib/format';
import { LeagueChip } from './LeagueChip';

/**
 * One market's free shortlist.
 *
 * Odds shown are indicative — derived from the model's own price with a
 * typical margin applied, not scraped from any bookmaker. They exist so a
 * reader can judge whether a pick is worth taking, and are labelled as such.
 */
export function FreeTipsSection({ group }: { group: FreeTipGroup }) {
  const market = MARKETS[group.market];

  return (
    <section className="rounded-xl border border-line bg-surface">
      <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 className="text-sm font-semibold">{market.displayName}</h2>
        <span className="text-xs tabular-nums text-fg-dim">
          {group.tips.length} {group.tips.length === 1 ? 'tip' : 'tips'}
        </span>
      </div>

      <ul className="divide-y divide-line-soft">
        {group.tips.map(({ match, league, homeTeam, awayTeam, prediction, odds }) => (
          <li key={prediction.id}>
            <Link
              href={`/matches/${match.id}`}
              className="group grid gap-3 px-4 py-3 transition-colors hover:bg-surface-hi/50 sm:grid-cols-[1fr_auto] sm:items-center"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <LeagueChip league={league} showName={false} />
                  <p className="truncate text-sm font-medium">
                    {homeTeam.name} <span className="text-fg-dim">v</span> {awayTeam.name}
                  </p>
                </div>
                <p className="mt-1 text-xs text-fg-dim">
                  {league.name} · {formatTime(match.kickoffAt)} UTC
                </p>
              </div>

              <div className="flex items-center gap-4 sm:justify-end">
                <div className="min-w-0 sm:w-44">
                  <p className="text-sm font-semibold text-brand-pale">
                    {outcomeLabel(prediction.marketType, prediction.predictionValue)}
                  </p>
                </div>

                <span className="shrink-0 rounded-lg border border-line bg-canvas/60 px-2.5 py-1.5 text-sm font-semibold tabular-nums">
                  {formatOdds(odds)}
                </span>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
