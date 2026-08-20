import Link from 'next/link';

/**
 * Stat tile: a headline number that would be a one-bar chart if it were
 * plotted. Label in sentence case, value semibold, optional supporting note.
 */
export function StatTile({
  label,
  value,
  note,
  href,
}: {
  label: string;
  value: string;
  note?: string;
  href?: string;
}) {
  const body = (
    <>
      <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">{label}</p>
      <p className="mt-1.5 text-2xl font-semibold tabular-nums">{value}</p>
      {note && <p className="mt-1 text-xs leading-relaxed text-fg-muted">{note}</p>}
    </>
  );

  const className =
    'block rounded-xl border border-line bg-surface p-4' +
    (href ? ' transition-colors hover:border-brand/40 hover:bg-surface-hi/60' : '');

  return href ? (
    <Link href={href} className={className}>
      {body}
    </Link>
  ) : (
    <div className={className}>{body}</div>
  );
}
