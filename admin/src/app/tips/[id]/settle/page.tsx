import { redirect } from 'next/navigation';
import { requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { settleTipAction } from '@/app/actions';

export const dynamic = 'force-dynamic';

/**
 * For a free-text tip only — a structured tip (matchId + market + selection)
 * grades itself off the result feed automatically. This exists for the tips
 * that don't map onto a tracked match/market, per docs/backend/SETTLEMENT.md.
 */
export default async function SettleTipPage({ params, searchParams }: PageProps<'/tips/[id]/settle'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const { id } = await params;
  const search = await searchParams;
  const error = Array.isArray(search.error) ? search.error[0] : search.error;
  const done = search.done === '1';

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Settle tip by hand</h1>
      <p className="mt-2 max-w-lg text-sm text-fg-muted">
        Only for a tip that isn&apos;t structured against a tracked match and market — those grade
        themselves. This writes a result once and is not reversible from here.
      </p>

      {done && (
        <p className="mt-4 rounded-lg border border-good/30 bg-good/10 px-3 py-2 text-sm text-good-text">
          Settled.
        </p>
      )}
      {error && <p className="mt-4 text-sm text-crit-text">{error}</p>}

      <form action={settleTipAction} className="mt-6 max-w-lg space-y-4">
        <input type="hidden" name="tipId" value={id} />

        <div>
          <label>Outcome</label>
          <div className="flex gap-6 text-sm">
            <label className="flex items-center gap-2 font-normal text-fg">
              <input type="radio" name="wasCorrect" value="true" className="w-auto" required />
              Won
            </label>
            <label className="flex items-center gap-2 font-normal text-fg">
              <input type="radio" name="wasCorrect" value="false" className="w-auto" required />
              Lost
            </label>
          </div>
        </div>

        <div>
          <label htmlFor="actualOutcome">Actual outcome</label>
          <input
            id="actualOutcome"
            name="actualOutcome"
            type="text"
            required
            placeholder="What actually happened, in the same terms as the selection"
          />
        </div>

        <div>
          <label htmlFor="reason">Reason</label>
          <textarea id="reason" name="reason" required rows={3} />
        </div>

        <button
          type="submit"
          className="rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Settle
        </button>
      </form>
    </Shell>
  );
}
