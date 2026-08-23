import { redirect } from 'next/navigation';
import { getAnalysts, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { createAnalystAction, deactivateAnalystAction } from '@/app/actions';
import { formatDateTime } from '@/lib/format';

export const dynamic = 'force-dynamic';

/**
 * Analysts are named here once, then just picked by name on every slip and
 * tip after — this page is the only place that ever writes their record.
 * Removing one deactivates rather than deletes: their published slips and
 * settled tips stay in the public track record either way.
 */
export default async function AnalystsPage({ searchParams }: PageProps<'/analysts'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const params = await searchParams;
  const error = Array.isArray(params.error) ? params.error[0] : params.error;
  const analysts = await getAnalysts();

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Analysts</h1>
      <p className="mt-2 max-w-lg text-sm text-fg-muted">
        Add an analyst once — they then show up as an option everywhere a slip needs one. Remove
        one when they stop publishing; their existing slips and settled tips are untouched.
      </p>

      {error && <p className="mt-4 text-sm text-crit-text">{error}</p>}

      <section className="mt-8">
        <h2 className="text-sm font-medium text-fg-muted">Current analysts</h2>
        {analysts.length === 0 ? (
          <p className="mt-2 text-sm text-fg-dim">None yet — add the first one below.</p>
        ) : (
          <ul className="mt-2 divide-y divide-line-soft rounded-lg border border-line">
            {analysts.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                <div>
                  <p className="font-medium">
                    {a.name} <span className="text-fg-muted">@{a.handle}</span>
                  </p>
                  <p className="text-xs text-fg-dim">
                    {a.initials} · joined {formatDateTime(a.joinedAt)}
                    {a.bio ? ` · ${a.bio}` : ''}
                  </p>
                </div>
                <form action={deactivateAnalystAction}>
                  <input type="hidden" name="analystId" value={a.id} />
                  <button
                    type="submit"
                    className="rounded-lg border border-crit/40 px-3 py-1.5 text-xs text-crit-text hover:bg-crit/10"
                  >
                    Remove
                  </button>
                </form>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="mt-8 border-t border-line-soft pt-6">
        <h2 className="text-sm font-medium text-fg-muted">Add an analyst</h2>
        <form action={createAnalystAction} className="mt-4 max-w-lg space-y-4">
          <div>
            <label htmlFor="name">Name</label>
            <input id="name" name="name" type="text" required placeholder="Kato Ronald" />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label htmlFor="handle">Handle</label>
              <input id="handle" name="handle" type="text" required placeholder="katoronald" />
            </div>
            <div>
              <label htmlFor="initials">Initials</label>
              <input id="initials" name="initials" type="text" required maxLength={3} placeholder="KR" />
            </div>
          </div>

          <div>
            <label htmlFor="bio">Bio (optional)</label>
            <textarea id="bio" name="bio" rows={3} placeholder="Focuses on East African top flights." />
          </div>

          <button
            type="submit"
            className="rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
          >
            Add analyst
          </button>
        </form>
      </section>
    </Shell>
  );
}
