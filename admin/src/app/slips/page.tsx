import { redirect } from 'next/navigation';
import { getAnalysts, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { createSlipAction } from '@/app/actions';
import { PACKAGE_CODES } from '@/api/types';

export const dynamic = 'force-dynamic';

/**
 * There is no "list every draft slip" endpoint — GET /v1/slips only returns
 * published (open/settled) slips, deliberately: a draft isn't a product yet.
 * Creating a slip here redirects straight to its detail page, and that
 * detail page's URL is the record of it until it's published.
 */
export default async function SlipsPage({ searchParams }: PageProps<'/slips'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const params = await searchParams;
  const error = Array.isArray(params.error) ? params.error[0] : params.error;
  const analysts = await getAnalysts();

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">New slip</h1>
      <p className="mt-2 text-sm text-fg-muted">
        Created as a draft. Add its tips on the next page, then publish deliberately — publishing
        refuses if any tip&apos;s kickoff has already passed.
      </p>

      {error && <p className="mt-4 text-sm text-crit-text">{error}</p>}

      <form action={createSlipAction} className="mt-6 max-w-lg space-y-4">
        <div>
          <label htmlFor="title">Title</label>
          <input id="title" name="title" type="text" required placeholder="Saturday Banker" />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="packageCode">Package</label>
            <select id="packageCode" name="packageCode" required defaultValue="">
              <option value="" disabled>
                Select…
              </option>
              {PACKAGE_CODES.map((code) => (
                <option key={code} value={code}>
                  {code}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="priceUgx">Price (UGX)</label>
            <input id="priceUgx" name="priceUgx" type="number" min={500} step={1} required />
          </div>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="totalOdds">Total odds</label>
            <input
              id="totalOdds"
              name="totalOdds"
              type="text"
              inputMode="decimal"
              placeholder="4.500"
              required
            />
            <p className="mt-1 text-[11px] text-fg-dim">
              A decimal string, e.g. 4.500 — never a float. Accumulated odds across every tip.
            </p>
          </div>
          <div>
            <label htmlFor="tipCount">Tip count</label>
            <input id="tipCount" name="tipCount" type="number" min={1} step={1} required />
          </div>
        </div>

        <div>
          <label htmlFor="analystIds">Analyst(s)</label>
          <select id="analystIds" name="analystIds" multiple required size={Math.min(6, analysts.length || 1)}>
            {analysts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} (@{a.handle})
              </option>
            ))}
          </select>
          <p className="mt-1 text-[11px] text-fg-dim">Cmd/Ctrl-click to select more than one.</p>
        </div>

        <button
          type="submit"
          className="rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Create draft
        </button>
      </form>
    </Shell>
  );
}
