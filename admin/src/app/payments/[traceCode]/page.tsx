import Link from 'next/link';
import { notFound, redirect } from 'next/navigation';
import { getPaymentTrace, requireAdminSession } from '@/api/client';
import { Shell } from '@/components/Shell';
import { formatDateTime, formatUGX } from '@/lib/format';

export const dynamic = 'force-dynamic';

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-line-soft px-4 py-2.5 text-sm last:border-0">
      <span className="text-fg-muted">{label}</span>
      <span className="text-right">{value}</span>
    </div>
  );
}

/**
 * Resolves a code read off a MarzPay statement, or quoted by a user from
 * their SMS, to the slip and buyer behind it — accepts with or without the
 * KTF- prefix, whichever way it was retyped.
 */
export default async function PaymentTracePage({
  params,
}: PageProps<'/payments/[traceCode]'>) {
  const user = await requireAdminSession();
  if (!user) redirect('/login');

  const { traceCode } = await params;
  const trace = await getPaymentTrace(traceCode);
  if (!trace) notFound();

  return (
    <Shell user={user}>
      <Link href="/payments" className="text-sm text-fg-muted hover:text-fg">
        ← Payment ledger
      </Link>

      <h1 className="mt-4 font-mono text-xl font-semibold tracking-tight">{trace.traceCode}</h1>

      <div className="mt-6 rounded-lg border border-line">
        <Row label="Status" value={trace.status} />
        <Row label="Amount" value={formatUGX(trace.amountUgx)} />
        <Row label="Phone" value={trace.phoneNumber} />
        <Row label="Mobile provider" value={trace.mobileProvider ?? '—'} />
        <Row label="Provider transaction id" value={trace.providerTxnId ?? '—'} />
        <Row label="Created" value={formatDateTime(trace.createdAt)} />
        <Row label="Settled" value={trace.settledAt ? formatDateTime(trace.settledAt) : '—'} />
      </div>

      <h2 className="mt-8 text-sm font-medium text-fg-muted">Purchase</h2>
      <div className="mt-2 rounded-lg border border-line">
        <Row label="Purchase status" value={trace.purchaseStatus} />
        <Row label="Paid at" value={trace.paidAt ? formatDateTime(trace.paidAt) : '—'} />
        <Row
          label="Slip"
          value={
            <Link href={`/slips/${trace.slipId}`} className="text-brand hover:underline">
              {trace.slipTitle}
            </Link>
          }
        />
        <Row label="Package" value={trace.packageCode} />
        <Row label="Buyer" value={`${trace.userName} · ${trace.userEmail}`} />
      </div>
    </Shell>
  );
}
