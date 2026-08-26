import type { League, MatchWithPredictions, SettledPrediction, Team } from '@/api/types';
import { kebab } from './leagues';

/**
 * URL slugs for team landing pages (/teams/[slug]).
 *
 * Unlike leagues, there is no teams endpoint: the team universe is whatever
 * the feed and the settled ledger mention. That is enough for a page — every
 * team with a priced fixture or a graded pick has real content — but it means
 * slug resolution must run over both sources, and a team absent from both
 * simply has no page rather than an empty one.
 */

export interface TeamWithLeague {
  team: Team;
  league: League;
}

/** Every team in the feed and the graded ledger, keyed by id. */
export function collectTeams(
  feed: MatchWithPredictions[],
  ledger: SettledPrediction[],
): Map<string, TeamWithLeague> {
  const map = new Map<string, TeamWithLeague>();
  const add = (team: Team, league: League) => {
    if (!map.has(team.id)) map.set(team.id, { team, league });
  };
  for (const entry of feed) {
    add(entry.homeTeam, entry.league);
    add(entry.awayTeam, entry.league);
  }
  for (const row of ledger) {
    add(row.homeTeam, row.league);
    add(row.awayTeam, row.league);
  }
  return map;
}

/**
 * Slug → team entry. Same collision rule as leagues: a shared name
 * disambiguates by country for every league involved.
 */
export function teamSlugMap(teams: Iterable<TeamWithLeague>): Map<string, TeamWithLeague> {
  const byName = new Map<string, number>();
  for (const { team } of teams) {
    const key = kebab(team.name);
    byName.set(key, (byName.get(key) ?? 0) + 1);
  }

  const map = new Map<string, TeamWithLeague>();
  for (const entry of teams) {
    const key = kebab(entry.team.name);
    const slug =
      byName.get(key)! > 1 ? kebab(`${entry.league.country}-${entry.team.name}`) : key;
    if (!map.has(slug)) map.set(slug, entry);
  }
  return map;
}

/**
 * The one true slug resolution, shared by every caller.
 *
 * `teamSlugMap` disambiguates same-named teams by their league's country, so
 * the answer depends on the league objects it is handed. Feed and ledger rows
 * embed their own copy, and the leagues endpoint is the authority — resolving
 * against it here means the sitemap, the match pages and the team pages can
 * never disagree about a slug, which would otherwise surface as an internal
 * link into a 404.
 */
export function resolveTeamSlugs(
  feed: MatchWithPredictions[],
  ledger: SettledPrediction[],
  leagues: League[],
): Map<string, TeamWithLeague> {
  const leagueById = new Map(leagues.map((l) => [l.id, l]));
  const withLeague = [...collectTeams(feed, ledger).values()].map((entry) => ({
    ...entry,
    league: leagueById.get(entry.league.id) ?? entry.league,
  }));
  return teamSlugMap(withLeague);
}

/** A team's landing-page href, or null when it has none. */
export function teamHref(
  map: Map<string, TeamWithLeague>,
  teamId: string,
): string | null {
  for (const [slug, entry] of map) {
    if (entry.team.id === teamId) return `/teams/${slug}`;
  }
  return null;
}
