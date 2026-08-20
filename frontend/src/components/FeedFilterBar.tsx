'use client';

import { useRouter } from 'next/navigation';
import { useTransition } from 'react';
import type { FeedFilters, League, MarketCode } from '@/api/types';
import { MARKET_CODES, REGIONS } from '@/api/types';
import { MARKETS } from '@/lib/markets';
import { CONFIDENCE_STEPS, buildFeedQuery, countActiveFilters } from '@/lib/filters';

/**
 * Feed filters (FR-3). State lives in the URL, so this component only ever
 * computes the next query string and navigates — there is no local mirror of
 * the filter state to drift out of sync.
 */
export function FeedFilterBar({
  leagues,
  filters,
  resultCount,
  basePath = '/fixtures',
}: {
  leagues: League[];
  filters: FeedFilters;
  resultCount: number;
  basePath?: string;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();

  const apply = (next: FeedFilters) => {
    startTransition(() => {
      router.push(`${basePath}${buildFeedQuery(next)}`, { scroll: false });
    });
  };

  const toggle = <T extends string>(list: T[] | undefined, value: T): T[] | undefined => {
    const current = list ?? [];
    const next = current.includes(value)
      ? current.filter((v) => v !== value)
      : [...current, value];
    return next.length ? next : undefined;
  };

  const activeCount = countActiveFilters(filters);

  // With this many leagues, showing every chip at once is unusable. Narrow the
  // league list to the selected region.
  const visibleLeagues =
    filters.region && filters.region !== 'all'
      ? leagues.filter((l) => l.region === filters.region)
      : leagues;

  return (
    <section
      aria-label="Filter predictions"
      className={`rounded-xl border border-line bg-surface p-4 transition-opacity ${
        pending ? 'opacity-60' : ''
      }`}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">Filters</h2>
        <div className="flex items-center gap-3">
          <span className="text-xs tabular-nums text-fg-muted" aria-live="polite">
            {resultCount} {resultCount === 1 ? 'fixture' : 'fixtures'}
          </span>
          {activeCount > 0 && (
            <button
              type="button"
              onClick={() => apply({})}
              className="rounded-lg px-2 py-1 text-xs text-fg-muted hover:bg-surface-hi hover:text-fg"
            >
              Clear all
            </button>
          )}
        </div>
      </div>

      <div className="mt-4 space-y-4">
        <FilterGroup label="Region">
          <Chip
            active={(filters.region ?? 'all') === 'all'}
            onClick={() => apply({ ...filters, region: 'all', leagueIds: undefined })}
          >
            All
          </Chip>
          {REGIONS.map(({ value, label }) => (
            <Chip
              key={value}
              active={filters.region === value}
              onClick={() => apply({ ...filters, region: value, leagueIds: undefined })}
            >
              {label}
            </Chip>
          ))}
        </FilterGroup>

        <FilterGroup label="League">
          {visibleLeagues.map((league) => (
            <Chip
              key={league.id}
              active={Boolean(filters.leagueIds?.includes(league.id))}
              onClick={() => apply({ ...filters, leagueIds: toggle(filters.leagueIds, league.id) })}
              title={`${league.name} — ${league.country}`}
            >
              {league.shortName}
              <span className="sr-only"> — {league.name}</span>
            </Chip>
          ))}
        </FilterGroup>

        <FilterGroup label="Market">
          {MARKET_CODES.map((code) => (
            <Chip
              key={code}
              active={Boolean(filters.markets?.includes(code))}
              onClick={() =>
                apply({ ...filters, markets: toggle<MarketCode>(filters.markets, code) })
              }
            >
              {MARKETS[code].shortName}
            </Chip>
          ))}
        </FilterGroup>

        <FilterGroup label="Minimum confidence">
          {CONFIDENCE_STEPS.map((step) => (
            <Chip
              key={step}
              active={(filters.minConfidence ?? 0) === step}
              onClick={() =>
                apply({ ...filters, minConfidence: step === 0 ? undefined : step })
              }
            >
              {step === 0 ? 'Any' : `${step}%+`}
            </Chip>
          ))}
        </FilterGroup>
      </div>
    </section>
  );
}

function FilterGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-4">
      <p className="w-40 shrink-0 pt-1.5 text-xs font-medium uppercase tracking-wider text-fg-dim">
        {label}
      </p>
      <div className="flex flex-wrap gap-1.5">{children}</div>
    </div>
  );
}

function Chip({
  active,
  onClick,
  title,
  children,
}: {
  active: boolean;
  onClick: () => void;
  title?: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-pressed={active}
      className={`rounded-lg border px-2.5 py-1.5 text-xs transition-colors ${
        active
          ? 'border-brand bg-brand/15 text-fg'
          : 'border-line bg-canvas/40 text-fg-muted hover:border-line hover:bg-surface-hi hover:text-fg'
      }`}
    >
      {children}
    </button>
  );
}
