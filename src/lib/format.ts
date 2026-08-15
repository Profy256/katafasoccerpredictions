/**
 * All date formatting is pinned to UTC and the en-GB locale on purpose.
 *
 * These strings are produced during server rendering; formatting them in the
 * viewer's local timezone would produce markup that disagrees with what the
 * client would render. Kickoff times are labelled UTC in the UI so the fixed
 * timezone is visible rather than misleading.
 */

const DATE_TIME = new Intl.DateTimeFormat('en-GB', {
  day: '2-digit',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
  timeZone: 'UTC',
});

const DATE_ONLY = new Intl.DateTimeFormat('en-GB', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  timeZone: 'UTC',
});

const DAY_HEADING = new Intl.DateTimeFormat('en-GB', {
  weekday: 'long',
  day: 'numeric',
  month: 'long',
  timeZone: 'UTC',
});

const TIME_ONLY = new Intl.DateTimeFormat('en-GB', {
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
  timeZone: 'UTC',
});

export function formatDateTime(iso: string): string {
  return DATE_TIME.format(new Date(iso));
}

export function formatDate(iso: string): string {
  return DATE_ONLY.format(new Date(iso));
}

export function formatTime(iso: string): string {
  return TIME_ONLY.format(new Date(iso));
}

/** `2026-08-15` — the UTC calendar day a timestamp falls on. */
export function utcDayKey(iso: string): string {
  return iso.slice(0, 10);
}

/** "Today" / "Tomorrow" / "Saturday 15 August". */
export function formatDayHeading(dayKey: string): string {
  const date = new Date(`${dayKey}T00:00:00.000Z`);
  const now = new Date();
  const todayKey = new Date(
    Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()),
  )
    .toISOString()
    .slice(0, 10);
  const tomorrowKey = new Date(
    Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1),
  )
    .toISOString()
    .slice(0, 10);

  if (dayKey === todayKey) return 'Today';
  if (dayKey === tomorrowKey) return 'Tomorrow';
  return DAY_HEADING.format(date);
}

/** `64.3%` from a 0..1 rate. */
export function formatRate(rate: number, digits = 1): string {
  return `${(rate * 100).toFixed(digits)}%`;
}

/** `64.3%` from an already-percentage number. */
export function formatPct(pct: number, digits = 1): string {
  return `${pct.toFixed(digits)}%`;
}

/** Strength multipliers read best as `1.24×`. */
export function formatStrength(value: number): string {
  return `${value.toFixed(2)}×`;
}

export function formatCount(n: number): string {
  return n.toLocaleString('en-GB');
}

/** Prices are entered by the admin in Ugandan shillings. */
export function formatUgx(amount: number): string {
  return `UGX ${amount.toLocaleString('en-GB')}`;
}

/** Decimal odds always read with two places: 1.50, not 1.5. */
export function formatOdds(odds: number): string {
  return odds.toFixed(2);
}

/** Signed percentage for ROI figures. */
export function formatSignedPct(rate: number, digits = 1): string {
  const pct = rate * 100;
  return `${pct >= 0 ? '+' : ''}${pct.toFixed(digits)}%`;
}
