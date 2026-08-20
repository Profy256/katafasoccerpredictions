import { redirect } from 'next/navigation';
import { getRevenue, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { formatUGX } from '@/lib/format';
import type { RevenueBucket } from '@/api/types';

export const dynamic = 'force-dynamic';

function Bucket({ title, rows }: { title: string; rows: RevenueBucket[] }) {
  if (rows.length === 0) return null;
  return (
    <div>
      <h3 className="text-xs font-medium text-fg-muted">{title}</h3>
      <ul className="mt-2 divide-y divide-line-soft rounded-lg border border-line text-sm">
        {rows.map((r) => (
          <li key={r.key} className="flex items-center justify-between px-3 py-2">
            <span>{r.label}</span>
            <span className="text-fg-muted">
              {r.purchases} · {formatUGX(r.grossUgx)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Windowed on paid_at, not created_at — a purchase started one day and paid
 * the next is the second day's money, matching what a MarzPay statement
 * would show. Defaults to the API's own default window (last 30 days).
 */
export default async function RevenuePage({ searchParams }: PageProps<'/revenue'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const params = await searchParams;
  const from = Array.isArray(params.from) ? params.from[0] : params.from;
  const to = Array.isArray(params.to) ? params.to[0] : params.to;

  const report = await getRevenue(from, to);

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Revenue</h1>

      <form method="GET" className="mt-4 flex flex-wrap items-end gap-4">
        <div>
          <label htmlFor="from">From</label>
          <input id="from" name="from" type="date" defaultValue={from} />
        </div>
        <div>
          <label htmlFor="to">To (inclusive)</label>
          <input id="to" name="to" type="date" defaultValue={to} />
        </div>
        <button
          type="submit"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand"
        >
          Apply
        </button>
      </form>

      <div className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
        {[
          ['Gross', formatUGX(report.grossUgx)],
          ['Refunded', formatUGX(report.refundedUgx)],
          ['Net', formatUGX(report.netUgx)],
          ['Pending', formatUGX(report.pendingUgx)],
        ].map(([label, value]) => (
          <div key={label} className="rounded-lg border border-line px-4 py-3">
            <p className="text-xs text-fg-muted">{label}</p>
            <p className="mt-1 text-lg font-semibold">{value}</p>
          </div>
        ))}
      </div>

      <p className="mt-3 text-xs text-fg-dim">
        {report.paidPurchases} paid · {report.refundedPurchases} refunded ·{' '}
        {report.pendingPurchases} pending · {report.failedPurchases} failed
      </p>

      <div className="mt-8 grid gap-6 sm:grid-cols-2">
        <Bucket title="By package" rows={report.byPackage} />
        <Bucket title="By analyst" rows={report.byAnalyst} />
        <Bucket title="By slip" rows={report.bySlip} />
        <Bucket title="By mobile provider" rows={report.byMobileProvider} />
        <Bucket title="By day" rows={report.byDay} />
      </div>
    </Shell>
  );
}
