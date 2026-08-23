import Link from 'next/link';
import type { MatchWithPredictions } from '@/api/types';
import { MARKETS, outcomeLabel } from '@/lib/markets';
import { formatTime } from '@/lib/format';
import { LeagueChip } from './LeagueChip';

/** One fixture in the feed, with every published market for it (FR-1, FR-2). */
export function MatchCard({ entry }: { entry: MatchWithPredictions }) {
  const { match, league, homeTeam, awayTeam, predictions } = entry;

  return (
    <Link
      href={`/matches/${match.id}`}
      className="group block rounded-xl border border-line bg-surface p-4 transition-colors hover:border-brand/40 hover:bg-surface-hi/60"
    >
      <div className="flex items-center justify-between gap-3">
        <LeagueChip league={league} />
        <span className="shrink-0 text-xs tabular-nums text-fg-muted">
          {formatTime(match.kickoffAt)}
        </span>
      </div>

      {/* Which side is at home changes the pick, so it is labelled rather
          than implied by row order alone. */}
      <div className="mt-3 space-y-1">
        {[
          { team: homeTeam, tag: 'H', venue: 'Home' },
          { team: awayTeam, tag: 'A', venue: 'Away' },
        ].map(({ team, tag, venue }) => (
          <p key={team.id} className="flex items-baseline gap-2">
            <span
              aria-hidden
              className="w-3 shrink-0 text-[10px] font-medium text-fg-dim"
            >
              {tag}
            </span>
            <span className="sr-only">{venue}:</span>
            <span className="truncate text-[15px] font-semibold leading-6">{team.name}</span>
          </p>
        ))}
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
        {predictions.map((prediction) => (
          <div
            key={prediction.id}
            className="rounded-lg border border-line-soft bg-canvas/40 px-2.5 py-2"
          >
            <p className="text-[10px] font-medium uppercase tracking-wider text-fg-dim">
              {MARKETS[prediction.marketType].shortName}
            </p>
            <p className="mt-0.5 truncate text-[13px] font-semibold">
              {outcomeLabel(prediction.marketType, prediction.predictionValue)}
            </p>
          </div>
        ))}
      </div>

      <p className="mt-3 text-xs text-fg-dim group-hover:text-brand">
        Why the model picked this →
      </p>
    </Link>
  );
}
