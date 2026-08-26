import type { Team } from '@/api/types';
import { kebab } from './leagues';

/**
 * URL slugs for match pages (/matches/[id]).
 *
 * "Chelsea vs Arsenal prediction" is the single highest-intent query this
 * site can answer, and a bare UUID in the URL throws away the only free
 * relevance signal available — the fixture's own name. So the segment carries
 * both: a readable slug and, after a `--` separator, the id the API actually
 * resolves.
 *
 * The id stays in the URL on purpose rather than looking a fixture up by
 * name. Two teams meet twice a season (and cup ties repeat the pairing), so a
 * name-only URL is ambiguous by construction, and disambiguating it by date
 * would break the moment a fixture is rescheduled. The id is the stable
 * identity; the slug is the part search engines and humans read.
 *
 * `--` is safe as the separator because `kebab` collapses every run of
 * non-alphanumerics into a single hyphen, so it can never occur inside the
 * slug half, and a UUID contains only single hyphens — meaning a bare id is
 * still parsed correctly by the reader below. That is what keeps every
 * `/matches/{uuid}` link ever published working.
 */

const SEPARATOR = '--';

/** The canonical segment for a fixture: readable half, then the id. */
export function matchSlug(home: Team, away: Team, matchId: string): string {
  return `${kebab(`${home.name} vs ${away.name}`)}${SEPARATOR}${matchId}`;
}

/** The canonical href for a fixture. */
export function matchHref(home: Team, away: Team, matchId: string): string {
  return `/matches/${matchSlug(home, away, matchId)}`;
}

/**
 * The match id inside a URL segment. Accepts the canonical slugged form and
 * the legacy bare-id form alike; the page redirects the latter.
 */
export function matchIdFromSlug(segment: string): string {
  const at = segment.indexOf(SEPARATOR);
  return at === -1 ? segment : segment.slice(at + SEPARATOR.length);
}
