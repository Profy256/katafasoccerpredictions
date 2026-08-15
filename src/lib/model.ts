/**
 * Team strength estimation — step 1 and 2 of PRD section 5.2.
 *
 * Strengths are venue-split and recency-weighted, expressed as multiples of the
 * league average (1.0 == exactly average). They feed straight into the expected
 * goals calculation that the Poisson matrix is built from.
 */

export interface PlayedMatch {
  homeTeamId: string;
  awayTeamId: string;
  homeScore: number;
  awayScore: number;
  kickoffAt: string;
}

export interface LeagueBaselines {
  /** Mean goals scored by the home side across the league. */
  homeGoals: number;
  /** Mean goals scored by the away side across the league. */
  awayGoals: number;
  matches: number;
}

/** Fallbacks used until a league has enough played matches to speak for itself. */
const DEFAULT_BASELINES: LeagueBaselines = { homeGoals: 1.5, awayGoals: 1.15, matches: 0 };

/** Matches at a given venue considered for form. PRD FR-9 asks for 10-15. */
export const FORM_WINDOW = 12;

/** Weight halves every this many matches back, biasing toward recent form. */
const RECENCY_HALF_LIFE = 8;

/**
 * Pseudo-matches of "league average" mixed into every estimate. Stops a team
 * with two freak results from being handed an extreme strength rating.
 */
const PRIOR_WEIGHT = 3.5;

/** xG is clamped to this range; the Poisson tail gets silly outside it. */
const MIN_XG = 0.15;
const MAX_XG = 4.75;

export function computeLeagueBaselines(matches: PlayedMatch[]): LeagueBaselines {
  if (matches.length === 0) return DEFAULT_BASELINES;
  let home = 0;
  let away = 0;
  for (const m of matches) {
    home += m.homeScore;
    away += m.awayScore;
  }
  return {
    homeGoals: home / matches.length,
    awayGoals: away / matches.length,
    matches: matches.length,
  };
}

export interface VenueStrength {
  /** Goals scored relative to league average at this venue. Higher is better. */
  attack: number;
  /** Goals conceded relative to league average at this venue. Lower is better. */
  defense: number;
  /** Matches actually used, after windowing. */
  games: number;
  goalsFor: number;
  goalsAgainst: number;
  wins: number;
  draws: number;
  losses: number;
}

export interface TeamStrengths {
  teamId: string;
  home: VenueStrength;
  away: VenueStrength;
  totalGames: number;
}

const EMPTY_VENUE: VenueStrength = {
  attack: 1,
  defense: 1,
  games: 0,
  goalsFor: 0,
  goalsAgainst: 0,
  wins: 0,
  draws: 0,
  losses: 0,
};

/**
 * @param history matches involving this team, oldest first.
 */
function venueStrength(
  teamId: string,
  history: PlayedMatch[],
  venue: 'home' | 'away',
  baselines: LeagueBaselines,
): VenueStrength {
  const atVenue = history.filter((m) =>
    venue === 'home' ? m.homeTeamId === teamId : m.awayTeamId === teamId,
  );
  const window = atVenue.slice(-FORM_WINDOW);
  if (window.length === 0) return { ...EMPTY_VENUE };

  // Reference points: what an average team scores/concedes at this venue.
  const refFor = venue === 'home' ? baselines.homeGoals : baselines.awayGoals;
  const refAgainst = venue === 'home' ? baselines.awayGoals : baselines.homeGoals;

  let weight = 0;
  let weightedFor = 0;
  let weightedAgainst = 0;
  let goalsFor = 0;
  let goalsAgainst = 0;
  let wins = 0;
  let draws = 0;
  let losses = 0;

  window.forEach((m, index) => {
    const matchesAgo = window.length - 1 - index;
    const w = Math.pow(0.5, matchesAgo / RECENCY_HALF_LIFE);
    const gf = venue === 'home' ? m.homeScore : m.awayScore;
    const ga = venue === 'home' ? m.awayScore : m.homeScore;

    weight += w;
    weightedFor += w * gf;
    weightedAgainst += w * ga;
    goalsFor += gf;
    goalsAgainst += ga;
    if (gf > ga) wins++;
    else if (gf === ga) draws++;
    else losses++;
  });

  const rawAttack = refFor > 0 ? weightedFor / weight / refFor : 1;
  const rawDefense = refAgainst > 0 ? weightedAgainst / weight / refAgainst : 1;

  // Shrink toward league average in proportion to how little evidence we have.
  const denom = weight + PRIOR_WEIGHT;
  return {
    attack: (weight * rawAttack + PRIOR_WEIGHT) / denom,
    defense: (weight * rawDefense + PRIOR_WEIGHT) / denom,
    games: window.length,
    goalsFor,
    goalsAgainst,
    wins,
    draws,
    losses,
  };
}

export function computeTeamStrengths(
  teamId: string,
  leagueHistory: PlayedMatch[],
  baselines: LeagueBaselines,
): TeamStrengths {
  const history = leagueHistory.filter(
    (m) => m.homeTeamId === teamId || m.awayTeamId === teamId,
  );
  return {
    teamId,
    home: venueStrength(teamId, history, 'home', baselines),
    away: venueStrength(teamId, history, 'away', baselines),
    totalGames: history.length,
  };
}

export interface ExpectedGoals {
  xgHome: number;
  xgAway: number;
}

/**
 * Step 2 of PRD 5.2: a team's expected goals is its attacking strength times
 * the opponent's defensive weakness times the league's venue baseline.
 */
export function expectedGoals(
  home: TeamStrengths,
  away: TeamStrengths,
  baselines: LeagueBaselines,
): ExpectedGoals {
  const clamp = (v: number) => Math.min(MAX_XG, Math.max(MIN_XG, v));
  return {
    xgHome: clamp(home.home.attack * away.away.defense * baselines.homeGoals),
    xgAway: clamp(away.away.attack * home.home.defense * baselines.awayGoals),
  };
}

/** Most recent results first, as W/D/L from `teamId`'s perspective. */
export function recentForm(
  teamId: string,
  leagueHistory: PlayedMatch[],
  limit = 5,
): ('W' | 'D' | 'L')[] {
  return leagueHistory
    .filter((m) => m.homeTeamId === teamId || m.awayTeamId === teamId)
    .slice(-limit)
    .reverse()
    .map((m) => {
      const isHome = m.homeTeamId === teamId;
      const gf = isHome ? m.homeScore : m.awayScore;
      const ga = isHome ? m.awayScore : m.homeScore;
      return gf > ga ? 'W' : gf === ga ? 'D' : 'L';
    });
}
