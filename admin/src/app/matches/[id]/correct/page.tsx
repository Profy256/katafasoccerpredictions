import { redirect } from 'next/navigation';
import { requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { correctMatchAction } from '@/app/actions';

export const dynamic = 'force-dynamic';

/**
 * The only path that may change a finished score — and it does not re-grade.
 * prediction_results is immutable, so this flags affected predictions for
 * review instead of silently rewriting history; a correction is published as
 * a new row, separately, once someone reviews the flag.
 */
export default async function CorrectMatchPage({
  params,
  searchParams,
}: PageProps<'/matches/[id]/correct'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const { id } = await params;
  const search = await searchParams;
  const error = Array.isArray(search.error) ? search.error[0] : search.error;
  const done = search.done === '1';

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Correct a finished score</h1>
      <p className="mt-2 max-w-lg text-sm text-fg-muted">
        Existing results are unchanged. Predictions built on this match are flagged for a
        published correction — nothing re-grades automatically.
      </p>

      {done && (
        <p className="mt-4 rounded-lg border border-good/30 bg-good/10 px-3 py-2 text-sm text-good-text">
          Score corrected and affected predictions flagged.
        </p>
      )}
      {error && <p className="mt-4 text-sm text-crit-text">{error}</p>}

      <form action={correctMatchAction} className="mt-6 max-w-md space-y-4">
        <input type="hidden" name="matchId" value={id} />

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="homeScore">Home score</label>
            <input id="homeScore" name="homeScore" type="number" min={0} step={1} required />
          </div>
          <div>
            <label htmlFor="awayScore">Away score</label>
            <input id="awayScore" name="awayScore" type="number" min={0} step={1} required />
          </div>
        </div>

        <div>
          <label htmlFor="reason">Reason</label>
          <textarea id="reason" name="reason" required rows={3} placeholder="e.g. provider corrected the final score after a VAR review" />
        </div>

        <button
          type="submit"
          className="rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Correct score
        </button>
      </form>
    </Shell>
  );
}
