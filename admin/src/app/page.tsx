import Link from 'next/link';
import { redirect } from 'next/navigation';
import { getIngestStatus, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { formatDateTime } from '@/lib/format';

export const dynamic = 'force-dynamic';

export default async function DashboardPage() {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const status = await getIngestStatus();

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Dashboard</h1>

      {status.openCorrectionReviews > 0 && (
        <div className="mt-6 rounded-lg border border-warn/40 bg-warn/10 px-4 py-3 text-sm">
          <span className="font-medium text-warn">{status.openCorrectionReviews}</span>{' '}
          match{status.openCorrectionReviews === 1 ? '' : 'es'} flagged for correction review —
          scores were corrected but affected predictions have not been re-published.
        </div>
      )}

      <section className="mt-8">
        <h2 className="text-sm font-medium text-fg-muted">Ingestion</h2>
        <div className="mt-3 overflow-x-auto rounded-lg border border-line">
          <table className="w-full min-w-[640px] text-left text-sm">
            <thead>
              <tr className="border-b border-line text-xs text-fg-muted">
                <th className="px-4 py-2 font-medium">Provider</th>
                <th className="px-4 py-2 font-medium">Requests used</th>
                <th className="px-4 py-2 font-medium">Leagues</th>
                <th className="px-4 py-2 font-medium">Last fetched</th>
                <th className="px-4 py-2 font-medium">Throttled</th>
                <th className="px-4 py-2 font-medium">Ungraded past kickoff</th>
              </tr>
            </thead>
            <tbody>
              {status.providers.map((p) => (
                <tr key={p.provider} className="border-b border-line-soft last:border-0">
                  <td className="px-4 py-2">{p.provider}</td>
                  <td className="px-4 py-2">
                    {p.requestsUsed}
                    {p.dailyLimit ? ` / ${p.dailyLimit}` : ''}
                  </td>
                  <td className="px-4 py-2">{p.leagues}</td>
                  <td className="px-4 py-2 text-fg-muted">
                    {p.lastFetchedAt ? formatDateTime(p.lastFetchedAt) : '—'}
                  </td>
                  <td className="px-4 py-2 text-fg-muted">
                    {p.throttledAt ? formatDateTime(p.throttledAt) : '—'}
                  </td>
                  <td
                    className={
                      p.ungradedPastKickoff > 0
                        ? 'px-4 py-2 font-medium text-crit-text'
                        : 'px-4 py-2 text-fg-muted'
                    }
                  >
                    {p.ungradedPastKickoff}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="mt-8 flex flex-wrap gap-3">
        <Link
          href="/slips"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand"
        >
          Manage slips →
        </Link>
        <Link
          href="/revenue"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand"
        >
          Revenue →
        </Link>
        <Link
          href="/payments"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand"
        >
          Payment ledger →
        </Link>
      </section>

      <p className="mt-8 text-xs text-fg-dim">
        Settling a tip and correcting a match score have no list view yet — open them directly at{' '}
        <code className="text-fg-muted">/tips/[id]/settle</code> and{' '}
        <code className="text-fg-muted">/matches/[id]/correct</code>.
      </p>
    </Shell>
  );
}
