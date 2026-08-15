import type { League } from '@/api/types';

/**
 * League identity. East African leagues get a subtle accent because coverage
 * there is the product's stated differentiator (PRD 1.3) — it should be visible
 * in the feed, not buried in a filter.
 */
export function LeagueChip({ league, showName = true }: { league: League; showName?: boolean }) {
  const highlight = league.region === 'east-africa';
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span
        className={`rounded px-1.5 py-0.5 font-mono text-[10px] font-medium tracking-wide ${
          highlight ? 'bg-brand/15 text-brand-pale' : 'bg-surface-hi text-fg-muted'
        }`}
      >
        {league.countryCode}
      </span>
      {showName && <span className="text-fg-muted">{league.name}</span>}
    </span>
  );
}
