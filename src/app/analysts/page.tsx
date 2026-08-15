import type { Metadata } from 'next';
import Link from 'next/link';
import { getAnalystLeaderboard } from '@/api/client';
import { formatCount, formatOdds, formatRate, formatSignedPct } from '@/lib/format';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Analyst records',
  description:
    'The public, auto-graded record for every analyst publishing slips on Katafa.',
};

/** Below this many settled tips, a hit rate is mostly noise. */
const THIN_SAMPLE = 60;

export default async function AnalystsPage() {
  const leaderboard = await getAnalystLeaderboard();

  return (
    <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6">
      <header className="max-w-2xl">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Analyst records
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Every tip our analysts publish is graded the same way the model&rsquo;s
          are, and settled slips are released in full so the numbers can be
          checked rather than taken on trust.
        </p>
      </header>

      <div className="mt-8 grid gap-3 md:grid-cols-2">
        {leaderboard.map((record) => {
          const thin = record.overall.total < THIN_SAMPLE;
          return (
            <Link
              key={record.analyst.id}
              href={`/analysts/${record.analyst.slug}`}
              className="rounded-xl border border-line bg-surface p-5 transition-colors hover:border-brand/40"
            >
              <div className="flex items-start gap-3">
                <span
                  aria-hidden
                  className="grid h-11 w-11 shrink-0 place-items-center rounded-full bg-surface-hi text-sm font-semibold"
                >
                  {record.analyst.initials}
                </span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold">{record.analyst.name}</p>
                  <p className="truncate text-xs text-fg-dim">{record.analyst.handle}</p>
                  <p className="mt-1 flex flex-wrap gap-1">
                    {record.analyst.packages.map((code) => (
                      <span
                        key={code}
                        className="rounded bg-surface-hi px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-fg-muted"
                      >
                        {code}
                      </span>
                    ))}
                  </p>
                </div>
              </div>

              <p className="mt-4 text-sm leading-relaxed text-fg-muted line-clamp-2">
                {record.analyst.bio}
              </p>

              <dl className="mt-4 grid grid-cols-3 gap-3 border-t border-line-soft pt-4">
                <div>
                  <dt className="text-[10px] uppercase tracking-wider text-fg-dim">
                    Hit rate
                  </dt>
                  <dd className="mt-0.5 text-lg font-semibold tabular-nums">
                    {formatRate(record.overall.hitRate)}
                  </dd>
                  <dd className="text-[11px] text-fg-dim tabular-nums">
                    n={formatCount(record.overall.total)}
                  </dd>
                </div>
                <div>
                  <dt className="text-[10px] uppercase tracking-wider text-fg-dim">
                    Avg odds
                  </dt>
                  <dd className="mt-0.5 text-lg font-semibold tabular-nums">
                    {formatOdds(record.averageOdds)}
                  </dd>
                </div>
                <div>
                  <dt className="text-[10px] uppercase tracking-wider text-fg-dim">
                    ROI
                  </dt>
                  <dd
                    className={`mt-0.5 text-lg font-semibold tabular-nums ${
                      record.roi >= 0 ? 'text-good-text' : 'text-crit-text'
                    }`}
                  >
                    {formatSignedPct(record.roi)}
                  </dd>
                  <dd className="text-[11px] text-fg-dim">level stakes</dd>
                </div>
              </dl>

              {thin && (
                <p className="mt-3 text-[11px] leading-relaxed text-warn">
                  Small sample — treat this record as provisional.
                </p>
              )}
            </Link>
          );
        })}
      </div>

      <p className="mt-8 text-xs leading-relaxed text-fg-dim">
        ROI assumes a flat one-unit stake on every settled tip at the odds shown
        when it was published. A positive figure over a few dozen tips is well
        within normal variance and is not by itself evidence of an edge.
      </p>
    </div>
  );
}
