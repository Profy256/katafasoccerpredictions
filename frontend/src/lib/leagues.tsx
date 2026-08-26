import Link from 'next/link';
import type { League } from '@/api/types';

/**
 * URL slugs for league landing pages (/leagues/[slug]).
 *
 * Slugs are derived from the league name, but the mapping must be stable and
 * collision-free across the whole league set: two competitions can share a
 * name ("Premier League" is not unique), and a URL that means two different
 * leagues depending on ingestion order is worse than no URL. Any name that
 * collides gets disambiguated by country for *every* league sharing it, so
 * the mapping depends only on the current league set, never on insert order.
 */

/** Kebab-case slug for a display name; shared by league and team URLs. */
export function kebab(value: string): string {
  return value
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/**
 * Every league's public URL segment, resolved against the full league set.
 * Returns a Map from slug to league; every slug in the result is unique.
 */
export function leagueSlugMap(leagues: League[]): Map<string, League> {
  const byName = new Map<string, number>();
  for (const league of leagues) {
    const key = kebab(league.name);
    byName.set(key, (byName.get(key) ?? 0) + 1);
  }

  const map = new Map<string, League>();
  for (const league of leagues) {
    const key = kebab(league.name);
    const slug =
      byName.get(key)! > 1 ? kebab(`${league.country}-${league.name}`) : key;
    // A pathological set could still collide after disambiguation; the first
    // league wins and the other simply has no landing page rather than an
    // ambiguous one.
    if (!map.has(slug)) map.set(slug, league);
  }
  return map;
}

/** The slug for one league within a resolved map. */
export function slugForLeague(map: Map<string, League>, leagueId: string): string | null {
  for (const [slug, league] of map) {
    if (league.id === leagueId) return slug;
  }
  return null;
}

/** A league's landing-page href, or null when it has none. */
export function leagueHref(
  map: Map<string, League>,
  leagueId: string,
): string | null {
  const slug = slugForLeague(map, leagueId);
  return slug ? `/leagues/${slug}` : null;
}

/** Contextual internal link to a league page, or plain text when unmapped. */
export function LeagueLink({
  map,
  league,
}: {
  map: Map<string, League>;
  league: League;
}) {
  const href = leagueHref(map, league.id);
  if (!href) return <>{league.name}</>;
  return (
    <Link href={href} className="text-brand hover:underline">
      {league.name}
    </Link>
  );
}
