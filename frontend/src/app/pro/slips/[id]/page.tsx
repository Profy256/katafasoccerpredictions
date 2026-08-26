import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getSession, getSlip } from '@/api/client';
import { purchaseSlipAction } from '@/app/actions';
import { OutcomeBadge } from '@/components/OutcomeBadge';
import { settledOutcomeLabel } from '@/lib/poisson';
import { formatDateTime, formatOdds, formatUgx } from '@/lib/format';

export const dynamic = 'force-dynamic';

export async function generateMetadata({
  params,
}: PageProps<'/pro/slips/[id]'>): Promise<Metadata> {
  const { id } = await params;
  const slip = await getSlip(id);
  if (!slip) return { title: 'Slip not found' };
  return {
    title: `${slip.package.name} slip: ${slip.title}`,
    description: `${slip.tipCount}-selection ${slip.package.name} slip by Katafa's analysts, priced at ${formatUgx(slip.priceUgx)} and graded automatically against every result once settled.`,
    alternates: { canonical: `/pro/slips/${id}` },
  };
}

export default async function SlipPage({
  params,
  searchParams,
}: PageProps<'/pro/slips/[id]'>) {
  const { id } = await params;
  const query = await searchParams;
  const [session, slip] = await Promise.all([getSession(), getSlip(id)]);
  // A draft slip is a 404 rather than a 403, so unpublished ids stay
  // undiscoverable by probing.
  if (!slip) notFound();

  const single = (value: string | string[] | undefined) =>
    Array.isArray(value) ? value[0] : value;
  const pendingTrace = single(query.pending);
  const purchaseError = single(query.error);

  const resultByTip = new Map((slip.results ?? []).map((r) => [r.tipId, r]));
  const settled = slip.status === 'settled';
  const won = slip.wonTips ?? 0;

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      <Link href="/pro" className="text-sm text-fg-muted hover:text-fg">
        ← All slips
      </Link>

      <header className="mt-4 rounded-xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded bg-surface-hi px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-fg-muted">
            {slip.package.name}
          </span>
          <span className="text-xs text-fg-dim">
            {slip.tipCount} {slip.tipCount === 1 ? 'selection' : 'selections'}
          </span>
          {settled && <OutcomeBadge correct={won === slip.tipCount} size="sm" />}
        </div>

        <div className="mt-3 flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">
              {slip.analysts.map((a) => a.name).join(' & ') || 'Katafa analysts'}
            </h1>
            <p className="mt-1 text-xs text-fg-dim">
              Published {formatDateTime(slip.publishedAt)} UTC
            </p>
          </div>
          <div className="text-right">
            <p className="text-[10px] uppercase tracking-wider text-fg-dim">Total odds</p>
            <p className="text-2xl font-semibold tabular-nums">
              {formatOdds(slip.totalOdds)}
            </p>
          </div>
        </div>

        {slip.analysts.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2 border-t border-line-soft pt-4">
            {slip.analysts.map((analyst) => (
              <Link
                key={analyst.id}
                href={`/analysts/${analyst.slug}`}
                className="rounded-lg border border-line px-2.5 py-1.5 text-xs text-fg-muted hover:border-brand/40 hover:text-brand"
              >
                {analyst.name} — see record →
              </Link>
            ))}
          </div>
        )}
      </header>

      {slip.unlocked ? (
        <section className="mt-6">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-lg font-semibold tracking-tight">Selections</h2>
            {settled && (
              <p className="text-sm text-fg-muted tabular-nums">
                {won} of {slip.tipCount} landed
              </p>
            )}
          </div>

          <div className="mt-3 overflow-x-auto rounded-xl border border-line bg-surface">
            <table className="w-full min-w-[40rem] text-left text-sm">
              <caption className="sr-only">Selections on this slip</caption>
              <thead>
                <tr className="border-b border-line text-xs uppercase tracking-wider text-fg-dim">
                  <th scope="col" className="px-4 py-3 font-medium">Fixture</th>
                  <th scope="col" className="px-4 py-3 font-medium">Market</th>
                  <th scope="col" className="px-4 py-3 font-medium">Selection</th>
                  <th scope="col" className="px-4 py-3 font-medium">Odds</th>
                  {settled && (
                    <th scope="col" className="px-4 py-3 font-medium">Outcome</th>
                  )}
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {slip.tips.map((tip) => {
                  const result = resultByTip.get(tip.id);
                  return (
                    <tr key={tip.id}>
                      <td className="px-4 py-3">
                        {tip.matchId ? (
                          <Link href={`/matches/${tip.matchId}`} className="hover:text-brand">
                            {tip.fixtureLabel}
                          </Link>
                        ) : (
                          tip.fixtureLabel
                        )}
                        <span className="block text-xs text-fg-dim">
                          {formatDateTime(tip.kickoffAt)} UTC
                        </span>
                      </td>
                      <td className="px-4 py-3 text-fg-muted">{tip.marketLabel}</td>
                      <td className="px-4 py-3 font-medium">{tip.selectionLabel}</td>
                      <td className="px-4 py-3 tabular-nums">{formatOdds(tip.odds)}</td>
                      {settled && (
                        <td className="px-4 py-3">
                          {result ? (
                            <span className="flex flex-wrap items-center gap-2">
                              <OutcomeBadge correct={result.wasCorrect} size="sm" />
                              {tip.marketType && (
                                <span className="text-xs text-fg-dim">
                                  {settledOutcomeLabel(tip.marketType, result.actualOutcome)}
                                </span>
                              )}
                            </span>
                          ) : (
                            <span className="text-xs text-fg-dim">Awaiting</span>
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {settled && (
            <p className="mt-3 text-xs leading-relaxed text-fg-muted">
              Settled slips are published in full, free of charge. That is what
              makes the analyst records above verifiable rather than a claim.
            </p>
          )}
        </section>
      ) : (
        <section className="mt-6 rounded-xl border border-line bg-surface p-6">
          <h2 className="text-lg font-semibold tracking-tight">
            {slip.tipCount} selections locked
          </h2>
          <p className="mt-2 max-w-lg text-sm leading-relaxed text-fg-muted">
            Unlock this slip to see the fixtures, markets and selections. You pay
            once for this slip only — there is nothing recurring.
          </p>

          {/* Placeholder rows: shape of what is behind the paywall, no data. */}
          <ul aria-hidden className="mt-5 space-y-2">
            {Array.from({ length: slip.tipCount }).map((_, i) => (
              <li
                key={i}
                className="flex items-center gap-3 rounded-lg border border-line-soft bg-canvas/40 px-3 py-3"
              >
                <span className="h-3 w-1/3 rounded bg-line" />
                <span className="h-3 w-1/5 rounded bg-line-soft" />
                <span className="ml-auto h-3 w-10 rounded bg-line" />
              </li>
            ))}
          </ul>

          {pendingTrace && (
            <div className="mt-6 rounded-lg border border-warn/40 bg-warn/5 p-4">
              <p className="text-sm font-medium text-warn">
                Check your phone for the mobile money prompt.
              </p>
              <p className="mt-1.5 text-xs leading-relaxed text-fg-muted">
                Approve it and refresh this page — the slip unlocks once the
                payment clears. Your reference is{' '}
                <span className="font-mono font-medium text-fg">{pendingTrace}</span>,
                which also appears in the confirmation SMS. Quote it if you need
                help with this payment.
              </p>
            </div>
          )}

          {purchaseError && (
            <p className="mt-6 rounded-lg border border-crit/40 bg-crit/5 p-4 text-sm text-crit-text">
              {purchaseError}
            </p>
          )}

          <div className="mt-6 border-t border-line-soft pt-5">
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div>
                <p className="text-[10px] uppercase tracking-wider text-fg-dim">Price</p>
                <p className="text-2xl font-semibold tabular-nums">
                  {formatUgx(slip.priceUgx)}
                </p>
              </div>

              {session ? (
                <form
                  action={purchaseSlipAction}
                  className="flex flex-wrap items-end gap-3"
                >
                  <input type="hidden" name="slipId" value={slip.id} />
                  <div>
                    <label
                      htmlFor="phone"
                      className="block text-[10px] uppercase tracking-wider text-fg-dim"
                    >
                      MTN or Airtel number
                    </label>
                    <input
                      id="phone"
                      name="phone"
                      type="tel"
                      required
                      inputMode="tel"
                      autoComplete="tel"
                      defaultValue={session.phone ?? ''}
                      placeholder="0772 123 456"
                      className="mt-1.5 w-44 rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
                    />
                  </div>
                  <button
                    type="submit"
                    className="rounded-lg bg-brand px-4 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
                  >
                    Unlock with Mobile Money
                  </button>
                </form>
              ) : (
                <Link
                  href={`/login?next=/pro/slips/${slip.id}`}
                  className="rounded-lg bg-brand px-4 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
                >
                  Sign in to unlock
                </Link>
              )}
            </div>
          </div>

          <p className="mt-4 text-xs leading-relaxed text-fg-dim">
            You are charged once for this slip. The selections appear as soon as
            the payment clears — usually within a few seconds of approving the
            prompt.
          </p>
        </section>
      )}
    </div>
  );
}
