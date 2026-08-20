import Link from 'next/link';
import type { Analyst, Slip } from '@/api/types';
import { formatDateTime, formatOdds, formatUgx } from '@/lib/format';
import { OutcomeBadge } from './OutcomeBadge';

/**
 * A slip in a listing. Never renders the selections themselves — an open slip
 * is paid content, and the picks only arrive from the API once the viewer owns
 * it. What is shown here is what someone needs to decide whether to buy.
 */
export function SlipCard({
  slip,
  analysts,
  packageName,
  owned,
}: {
  slip: Slip;
  analysts: Analyst[];
  packageName: string;
  owned: boolean;
}) {
  const settled = slip.status === 'settled';
  const won = slip.wonTips ?? 0;
  const allWon = settled && won === slip.tipCount;

  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded bg-surface-hi px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-fg-muted">
              {packageName}
            </span>
            <span className="text-xs text-fg-dim">
              {slip.tipCount} {slip.tipCount === 1 ? 'selection' : 'selections'}
            </span>
            {settled && (
              <OutcomeBadge correct={allWon} size="sm" />
            )}
          </div>

          <p className="mt-2 truncate text-sm font-medium">
            {analysts.map((a) => a.name).join(' & ') || 'Katafa analysts'}
          </p>
          <p className="mt-0.5 text-xs text-fg-dim">
            Published {formatDateTime(slip.publishedAt)} UTC
          </p>
        </div>

        <div className="text-right">
          <p className="text-[10px] uppercase tracking-wider text-fg-dim">Total odds</p>
          <p className="text-lg font-semibold tabular-nums">{formatOdds(slip.totalOdds)}</p>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-line-soft pt-3">
        {settled ? (
          <p className="text-xs text-fg-muted">
            <span className="font-semibold text-fg tabular-nums">
              {won}/{slip.tipCount}
            </span>{' '}
            landed · selections now public
          </p>
        ) : owned ? (
          <p className="text-xs text-good-text">Unlocked — you own this slip</p>
        ) : (
          <p className="flex items-center gap-1.5 text-xs text-fg-muted">
            <LockIcon />
            Selections hidden ·{' '}
            <span className="font-semibold text-fg tabular-nums">
              {formatUgx(slip.priceUgx)}
            </span>
          </p>
        )}

        <Link
          href={`/pro/slips/${slip.id}`}
          className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
            settled || owned
              ? 'border border-line text-fg-muted hover:border-brand/40 hover:text-brand'
              : 'bg-brand text-canvas hover:opacity-90'
          }`}
        >
          {settled ? 'View result' : owned ? 'View selections' : 'Unlock slip'}
        </Link>
      </div>
    </div>
  );
}

function LockIcon() {
  return (
    <svg
      aria-hidden
      viewBox="0 0 16 16"
      className="h-3.5 w-3.5"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
    >
      <rect x="3.5" y="7" width="9" height="6.5" rx="1.5" />
      <path d="M5.75 7V5.25a2.25 2.25 0 0 1 4.5 0V7" />
    </svg>
  );
}
