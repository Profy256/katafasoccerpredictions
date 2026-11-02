/**
 * Timestamps. Postgres is asked to return raw strings (see the type parsers in
 * db.ts) so nothing is lost to JS Date rounding, and this formats them the way
 * Go's time.Time marshals to JSON: RFC3339 with a Z.
 *
 * Postgres emits `2026-08-26 12:19:38.330879+00` for TIMESTAMPTZ in a UTC
 * session. Go scans that into a time.Time and writes it back as
 * `2026-08-26T12:19:38.330879Z`; the fractional digits match because both
 * trim trailing zeros.
 */
export function formatTimestamptz(raw: string): string {
  return raw.replace(' ', 'T').replace(/\+00(:00)?$/, 'Z');
}

/**
 * Go's `t.Format(time.RFC3339)`: seconds precision only. Used where Go
 * deliberately drops the fraction — the accuracy dashboard's settlement
 * window — while everything else keeps RFC3339Nano.
 */
export function formatRFC3339(raw: string): string {
  return raw.replace(' ', 'T').replace(/\.\d+/, '').replace(/\+00(:00)?$/, 'Z');
}

/** The zero time as Go's default JSON encoder writes it. */
export function formatZeroTime(): string {
  return '0001-01-01T00:00:00Z';
}
