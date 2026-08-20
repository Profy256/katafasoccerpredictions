/**
 * Pinned to UTC and en-GB, matching frontend/src/lib/format.ts — an admin
 * looking at a kickoff time next to a payment timestamp needs both apps
 * agreeing on what timezone they're printed in.
 */

const DATE_TIME = new Intl.DateTimeFormat('en-GB', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
  timeZone: 'UTC',
});

export function formatDateTime(iso: string): string {
  return DATE_TIME.format(new Date(iso));
}

/** UGX is zero-decimal — whole shillings, never divided or rounded. */
export function formatUGX(amount: number): string {
  return `UGX ${amount.toLocaleString('en-GB')}`;
}
