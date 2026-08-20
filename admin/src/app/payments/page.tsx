import Link from 'next/link';
import { redirect } from 'next/navigation';
import { getPaymentLedger, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { lookupTraceAction } from '@/app/actions';
import { formatDateTime, formatUGX } from '@/lib/format';

export const dynamic = 'force-dynamic';

const STATUS_TONE: Record<string, string> = {
  completed: 'text-good-text',
  failed: 'text-crit-text',
  expired: 'text-crit-text',
};

/**
 * Every collection attempt in the window, successes and failures alike — a
 * statement line missing here because it didn't succeed is a line nobody
 * could explain. Defaults to the API's own window (last 7 days).
 */
export default async function PaymentsPage({ searchParams }: PageProps<'/payments'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const params = await searchParams;
  const from = Array.isArray(params.from) ? params.from[0] : params.from;
  const to = Array.isArray(params.to) ? params.to[0] : params.to;

  const ledger = await getPaymentLedger(from, to, 200);

  return (
    <Shell user={user}>
      <h1 className="text-xl font-semibold tracking-tight">Payment ledger</h1>

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

      <form action={lookupTraceAction} className="mt-4 flex max-w-sm items-end gap-2">
        <div className="flex-1">
          <label htmlFor="traceCode">Look up a trace code</label>
          <input id="traceCode" name="traceCode" type="text" placeholder="KTF-3F9A2B7C" />
        </div>
        <button
          type="submit"
          className="rounded-lg border border-line px-3 py-2 text-sm hover:border-brand"
        >
          Go
        </button>
      </form>

      <div className="mt-6 overflow-x-auto rounded-lg border border-line">
        <table className="w-full min-w-[840px] text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs text-fg-muted">
              <th className="px-4 py-2 font-medium">Trace</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">Amount</th>
              <th className="px-4 py-2 font-medium">Slip</th>
              <th className="px-4 py-2 font-medium">Buyer</th>
              <th className="px-4 py-2 font-medium">Phone</th>
              <th className="px-4 py-2 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {ledger.rows.map((row) => (
              <tr key={row.traceCode} className="border-b border-line-soft last:border-0">
                <td className="px-4 py-2 font-mono text-xs">
                  <Link href={`/payments/${row.traceCode}`} className="text-brand hover:underline">
                    {row.traceCode}
                  </Link>
                </td>
                <td className={'px-4 py-2 ' + (STATUS_TONE[row.status] ?? 'text-fg-muted')}>
                  {row.status}
                </td>
                <td className="px-4 py-2">{formatUGX(row.amountUgx)}</td>
                <td className="px-4 py-2">{row.slipTitle}</td>
                <td className="px-4 py-2 text-fg-muted">{row.userEmail}</td>
                <td className="px-4 py-2 text-fg-muted">{row.phoneNumber}</td>
                <td className="px-4 py-2 text-fg-muted">{formatDateTime(row.createdAt)}</td>
              </tr>
            ))}
            {ledger.rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-6 text-center text-fg-dim">
                  No collection attempts in this window.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </Shell>
  );
}
