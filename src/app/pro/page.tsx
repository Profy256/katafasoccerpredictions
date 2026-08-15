import type { Metadata } from 'next';
import Link from 'next/link';
import { getAnalystLeaderboard, getPackages, getSlips } from '@/api/client';
import { SlipCard } from '@/components/SlipCard';
import { getOwnedSlipIds, getSession } from '@/lib/session';
import { formatCount, formatRate, formatUgx } from '@/lib/format';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Pro slips',
  description:
    'Hand-built slips from Ugandan football analysts across three packages. Pay per slip, with every analyst’s record public before you buy.',
};

const SETTLED_PER_PACKAGE = 3;

export default async function ProPage() {
  const [packages, openSlips, settledSlips, leaderboard, session, owned] =
    await Promise.all([
      getPackages(),
      getSlips({ status: 'open' }),
      getSlips({ status: 'settled', limit: 24 }),
      getAnalystLeaderboard(),
      getSession(),
      getOwnedSlipIds(),
    ]);

  const analystsById = new Map(leaderboard.map((r) => [r.analyst.id, r.analyst]));

  return (
    <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6">
      <header className="max-w-2xl">
        <p className="text-xs font-medium uppercase tracking-wider text-brand">Pro</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
          Slips from our analysts
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Three packages, built by hand rather than by the model. You buy a
          single slip at the price set for it — there is no monthly
          subscription and nothing recurring to cancel.
        </p>
      </header>

      <div className="mt-5 flex flex-wrap items-center gap-3 rounded-xl border border-line bg-surface px-4 py-3">
        {session ? (
          <p className="text-sm text-fg-muted">
            Signed in as <span className="font-medium text-fg">{session.email}</span> ·{' '}
            {owned.size} {owned.size === 1 ? 'slip' : 'slips'} unlocked
          </p>
        ) : (
          <>
            <p className="text-sm text-fg-muted">
              Sign in to buy a slip and keep your purchases.
            </p>
            <Link
              href="/login?next=/pro"
              className="rounded-lg border border-line px-3 py-1.5 text-xs hover:border-brand/40 hover:text-brand"
            >
              Sign in
            </Link>
          </>
        )}
      </div>

      {/* Packages */}
      <div className="mt-8 space-y-10">
        {packages.map((pkg) => {
          const open = openSlips.filter((s) => s.packageCode === pkg.code);
          const settled = settledSlips
            .filter((s) => s.packageCode === pkg.code)
            .slice(0, SETTLED_PER_PACKAGE);

          return (
            <section key={pkg.code} id={pkg.code} className="scroll-mt-20">
              <div className="rounded-xl border border-line bg-surface p-5">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="max-w-xl">
                    <h2 className="text-lg font-semibold tracking-tight">{pkg.name}</h2>
                    <p className="mt-0.5 text-sm text-brand-pale">{pkg.tagline}</p>
                    <p className="mt-2 text-sm leading-relaxed text-fg-muted">
                      {pkg.description}
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="text-[10px] uppercase tracking-wider text-fg-dim">
                      Typically
                    </p>
                    <p className="text-xl font-semibold tabular-nums">
                      {formatUgx(pkg.typicalPriceUgx)}
                    </p>
                    <p className="mt-0.5 text-[11px] text-fg-dim">per slip</p>
                  </div>
                </div>

                <ul className="mt-4 grid gap-2 border-t border-line-soft pt-4 sm:grid-cols-3">
                  {pkg.highlights.map((highlight) => (
                    <li key={highlight} className="flex gap-2 text-xs leading-relaxed text-fg-muted">
                      <span aria-hidden className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-brand" />
                      {highlight}
                    </li>
                  ))}
                </ul>
              </div>

              <div className="mt-4">
                <h3 className="text-xs font-medium uppercase tracking-wider text-fg-dim">
                  Open now
                </h3>
                {open.length === 0 ? (
                  <p className="mt-2 rounded-xl border border-dashed border-line bg-surface/50 px-4 py-6 text-center text-sm text-fg-muted">
                    No {pkg.name} slip is open right now.
                    {pkg.code === 'akatambula' &&
                      ' Akatambula is only released when the analysts agree it is worth putting out.'}
                  </p>
                ) : (
                  <div className="mt-2 grid gap-3 md:grid-cols-2">
                    {open.map((slip) => (
                      <SlipCard
                        key={slip.id}
                        slip={slip}
                        packageName={pkg.name}
                        owned={owned.has(slip.id)}
                        analysts={slip.analystIds
                          .map((id) => analystsById.get(id))
                          .filter((a): a is NonNullable<typeof a> => Boolean(a))}
                      />
                    ))}
                  </div>
                )}
              </div>

              {settled.length > 0 && (
                <div className="mt-5">
                  <h3 className="text-xs font-medium uppercase tracking-wider text-fg-dim">
                    Recently settled — selections public
                  </h3>
                  <div className="mt-2 grid gap-3 md:grid-cols-3">
                    {settled.map((slip) => (
                      <SlipCard
                        key={slip.id}
                        slip={slip}
                        packageName={pkg.name}
                        owned
                        analysts={slip.analystIds
                          .map((id) => analystsById.get(id))
                          .filter((a): a is NonNullable<typeof a> => Boolean(a))}
                      />
                    ))}
                  </div>
                </div>
              )}
            </section>
          );
        })}
      </div>

      {/* Analyst record teaser */}
      <section className="mt-12 rounded-xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-xl">
            <h2 className="text-sm font-semibold">Check the record before you pay</h2>
            <p className="mt-1.5 text-sm leading-relaxed text-fg-muted">
              Every analyst tip is graded the same way the model&rsquo;s are, and
              once a slip settles its selections become public so anyone can
              check the maths.
            </p>
          </div>
          <Link
            href="/analysts"
            className="shrink-0 rounded-lg border border-line px-3 py-2 text-sm hover:border-brand/40 hover:text-brand"
          >
            All analyst records →
          </Link>
        </div>

        <ul className="mt-4 divide-y divide-line-soft border-t border-line-soft">
          {leaderboard.map((record) => (
            <li key={record.analyst.id}>
              <Link
                href={`/analysts/${record.analyst.slug}`}
                className="flex items-center justify-between gap-4 py-3 hover:text-brand"
              >
                <span className="flex min-w-0 items-center gap-3">
                  <span
                    aria-hidden
                    className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-surface-hi text-xs font-semibold"
                  >
                    {record.analyst.initials}
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">
                      {record.analyst.name}
                    </span>
                    <span className="block truncate text-xs text-fg-dim">
                      {record.analyst.handle}
                    </span>
                  </span>
                </span>
                <span className="shrink-0 text-right text-sm tabular-nums">
                  <span className="font-semibold">{formatRate(record.overall.hitRate)}</span>
                  <span className="ml-2 text-xs text-fg-dim">
                    n={formatCount(record.overall.total)}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <p className="mt-6 text-xs leading-relaxed text-fg-dim">
        Payments are not yet connected. The unlock flow below is a placeholder —
        no money moves and nothing is charged. MarzPay collection will be wired
        in with the Go backend.
      </p>
    </div>
  );
}
