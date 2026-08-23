import Link from 'next/link';
import { notFound, redirect } from 'next/navigation';
import { getAnalysts, getMarkets, getSlip, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { AddTipForm } from '@/components/AddTipForm';
import { BulkAddTipsForm } from '@/components/BulkAddTipsForm';
import { deleteSlipAction, publishSlipAction } from '@/app/actions';
import { formatDateTime, formatUGX } from '@/lib/format';

export const dynamic = 'force-dynamic';

export default async function SlipDetailPage({ params, searchParams }: PageProps<'/slips/[id]'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const { id } = await params;
  const search = await searchParams;
  const error = Array.isArray(search.error) ? search.error[0] : search.error;

  const [slip, analysts, markets] = await Promise.all([getSlip(id), getAnalysts(), getMarkets()]);
  if (!slip) notFound();

  const draft = slip.status === 'draft';

  return (
    <Shell user={user}>
      <Link href="/slips" className="text-sm text-fg-muted hover:text-fg">
        ← New slip
      </Link>

      <div className="mt-4 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{slip.title}</h1>
          <p className="mt-1 text-sm text-fg-muted">
            {slip.packageCode} · {formatUGX(slip.priceUgx)} · odds {slip.totalOdds} ·{' '}
            {slip.tipCount} tip{slip.tipCount === 1 ? '' : 's'}
          </p>
        </div>
        <span
          className={
            'rounded-full px-3 py-1 text-xs font-medium ' +
            (draft
              ? 'bg-surface-hi text-fg-muted'
              : slip.status === 'open'
                ? 'bg-brand/15 text-brand-pale'
                : 'bg-good/15 text-good-text')
          }
        >
          {slip.status}
        </span>
      </div>

      {error && <p className="mt-4 text-sm text-crit-text">{error}</p>}

      <section className="mt-6">
        <h2 className="text-sm font-medium text-fg-muted">
          Tips ({slip.tips.length}/{slip.tipCount})
        </h2>
        {slip.tips.length === 0 ? (
          <p className="mt-2 text-sm text-fg-dim">None added yet.</p>
        ) : (
          <ul className="mt-2 divide-y divide-line-soft rounded-lg border border-line">
            {slip.tips.map((tip) => (
              <li key={tip.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                <div>
                  <p className="font-medium">{tip.fixtureLabel}</p>
                  <p className="text-fg-muted">
                    {tip.marketLabel} — {tip.selectionLabel} @ {tip.odds}
                    {tip.marketType && (
                      <span className="ml-2 text-fg-dim">
                        (structured: {tip.marketType} = {tip.selectionValue})
                      </span>
                    )}
                  </p>
                  <p className="text-xs text-fg-dim">Kicks off {formatDateTime(tip.kickoffAt)}</p>
                </div>
                <Link
                  href={`/tips/${tip.id}/settle`}
                  className="rounded-lg border border-line px-3 py-1.5 text-xs hover:border-brand"
                >
                  Settle by hand →
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {draft && (
        <section className="mt-6">
          <h2 className="text-sm font-medium text-fg-muted">Add a tip</h2>
          <AddTipForm
            slipId={slip.id}
            analysts={analysts}
            markets={markets}
            nextPosition={slip.tips.length + 1}
          />
        </section>
      )}

      {draft && (
        <section className="mt-6">
          <h2 className="text-sm font-medium text-fg-muted">Paste many at once</h2>
          <p className="mt-1 text-xs text-fg-dim">
            One match per line, in whatever shape you already have it — team names, market,
            selection, odds, kickoff, in any order. Nothing is added until you check the preview
            below and fix anything it got wrong.
          </p>
          <BulkAddTipsForm slipId={slip.id} analysts={analysts} nextPosition={slip.tips.length + 1} />
        </section>
      )}

      {draft && (
        <section className="mt-8 flex flex-wrap gap-3 border-t border-line-soft pt-6">
          <form action={publishSlipAction}>
            <input type="hidden" name="slipId" value={slip.id} />
            <button
              type="submit"
              disabled={slip.tips.length === 0}
              className="rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Publish
            </button>
          </form>
          <p className="max-w-sm self-center text-xs text-fg-dim">
            Refuses if any tip&apos;s kickoff has already passed.
          </p>

          <form action={deleteSlipAction} className="ml-auto">
            <input type="hidden" name="slipId" value={slip.id} />
            <button
              type="submit"
              className="rounded-lg border border-crit/40 px-3 py-2 text-sm text-crit-text hover:bg-crit/10"
            >
              Delete draft
            </button>
          </form>
        </section>
      )}

      {!draft && (
        <p className="mt-8 border-t border-line-soft pt-6 text-xs text-fg-dim">
          Published slips are never deleted or edited — that history stays, including the losing
          ones. Settle individual tips at /tips/[id]/settle, or correct a match score at
          /matches/[id]/correct.
        </p>
      )}
    </Shell>
  );
}
