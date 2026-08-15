import type {
  FormSummary,
  HeadToHeadMatch,
  League,
  Match,
  MarketCode,
  MatchReasoning,
  Prediction,
  PredictionResult,
  Team,
} from '@/api/types';
import { MARKET_CODES } from '@/api/types';
import {
  buildScorelineMatrix,
  isPickCorrect,
  marketDistribution,
  pickBest,
  settleMarket,
  topScorelines,
} from '@/lib/poisson';
import {
  computeLeagueBaselines,
  computeTeamStrengths,
  expectedGoals,
  recentForm,
  type PlayedMatch,
  type TeamStrengths,
} from '@/lib/model';
import { LEAGUES, LEAGUE_SCORING, MODEL_VERSION, TEAM_ROSTERS } from './seed';
import { jitter, makeRng, samplePoisson } from './rng';
import { buildTipsters, type TipsterData } from './tipsters';

/**
 * Builds the demo dataset that stands in for the ingestion pipeline and
 * prediction engine (PRD sections 4.3 and 4.4) until the Python services exist.
 *
 * Two properties matter here, because the whole product claim is accuracy
 * transparency:
 *
 *  1. Predictions are generated **walk-forward**. The pick for round N is
 *     computed using only results from rounds before N, so the accuracy figures
 *     on the dashboard are a genuine out-of-sample backtest rather than a
 *     model grading itself on data it already saw.
 *  2. Nothing is filtered. Every generated prediction is graded and published,
 *     losses included (FR-7).
 *
 * Caveat worth remembering: results here are *simulated* from the same Poisson
 * family the model assumes, so the model is better calibrated against this data
 * than it will be against real football. Treat the hit rates as a UI fixture,
 * not as evidence the model works.
 */

const TOTAL_ROUNDS = 26;
const UPCOMING_ROUNDS = 3;
const LAST_PLAYED_ROUND = TOTAL_ROUNDS - UPCOMING_ROUNDS - 1; // 22
/** Rounds used only to build up form; too little history to predict from. */
const BURN_IN_ROUNDS = 9;

const DAY_MS = 86_400_000;
const ROUND_SPACING_MS = 3.5 * DAY_MS;
/** Kickoff slots within a matchday, in UTC hours. */
const KICKOFF_SLOTS = [12.5, 15, 17.5, 20];

const H2H_LIMIT = 5;

interface LatentTeam extends Team {
  /** Ground-truth ratings used to simulate results; never exposed to the model. */
  attack: number;
  defense: number;
}

export interface Dataset {
  leagues: League[];
  leaguesById: Map<string, League>;
  teams: Team[];
  teamsById: Map<string, Team>;
  matches: Match[];
  matchesById: Map<string, Match>;
  predictions: Prediction[];
  predictionsByMatch: Map<string, Prediction[]>;
  predictionsById: Map<string, Prediction>;
  results: PredictionResult[];
  resultsByPrediction: Map<string, PredictionResult>;
  reasoningByMatch: Map<string, MatchReasoning>;
  tipsters: TipsterData;
  modelVersion: string;
}

/* ------------------------------------------------------------------ *
 * Scheduling
 * ------------------------------------------------------------------ */

/**
 * Circle-method round robin: one leg of `n - 1` rounds where every team plays
 * every other team exactly once.
 */
function singleLeg(teamIds: string[]): [string, string][][] {
  const n = teamIds.length;
  const rotation = [...teamIds];
  const rounds: [string, string][][] = [];

  for (let r = 0; r < n - 1; r++) {
    const pairs: [string, string][] = [];
    for (let i = 0; i < n / 2; i++) {
      const a = rotation[i];
      const b = rotation[n - 1 - i];
      // Alternate venues so no team ends up permanently at home.
      pairs.push((r + i) % 2 === 0 ? [a, b] : [b, a]);
    }
    rounds.push(pairs);
    // Rotate everything except the first entry.
    const [fixed, ...rest] = rotation;
    rest.unshift(rest.pop()!);
    rotation.splice(0, rotation.length, fixed, ...rest);
  }
  return rounds;
}

/** Enough rounds for the season window, using a reversed second leg. */
function buildSchedule(teamIds: string[], rounds: number): [string, string][][] {
  const first = singleLeg(teamIds);
  const second = first.map((round) => round.map(([h, a]) => [a, h] as [string, string]));
  const all = [...first, ...second];
  const out: [string, string][][] = [];
  for (let r = 0; r < rounds; r++) out.push(all[r % all.length]);
  return out;
}

/** Midnight UTC today — keeps "upcoming" fixtures genuinely upcoming. */
function seasonAnchor(): number {
  const now = new Date();
  return Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
}

function kickoffFor(anchor: number, round: number, indexInRound: number): string {
  // The first upcoming round sits one day out; everything else is spaced back
  // and forward from there.
  const base = anchor + DAY_MS + (round - (LAST_PLAYED_ROUND + 1)) * ROUND_SPACING_MS;
  const dayOffset = indexInRound % 2;
  // Round spacing is a half-integer number of days, so snap back to midnight
  // before adding the slot — otherwise the half-day carries through and
  // fixtures kick off at 00:30.
  const midnight = Math.floor((base + dayOffset * DAY_MS) / DAY_MS) * DAY_MS;
  const slot = KICKOFF_SLOTS[Math.floor(indexInRound / 2) % KICKOFF_SLOTS.length];
  return new Date(midnight + slot * 3_600_000).toISOString();
}

/* ------------------------------------------------------------------ *
 * Generation
 * ------------------------------------------------------------------ */

function buildTeams(league: League, rng: () => number): LatentTeam[] {
  const roster = TEAM_ROSTERS[league.id];
  return roster.map(([name, shortName], i) => {
    const t = roster.length === 1 ? 0 : i / (roster.length - 1);
    return {
      id: `${league.id}-${shortName.toLowerCase()}`,
      name,
      shortName,
      leagueId: league.id,
      // Strongest club scores ~1.40x league average and concedes ~0.66x.
      attack: Math.max(0.45, 1.4 - 0.78 * t + jitter(rng, 0.09)),
      defense: Math.max(0.5, 0.66 + 0.72 * t + jitter(rng, 0.09)),
    };
  });
}

function formSummary(
  teamId: string,
  strengths: TeamStrengths,
  venue: 'home' | 'away',
  history: PlayedMatch[],
): FormSummary {
  const v = venue === 'home' ? strengths.home : strengths.away;
  return {
    teamId,
    venue,
    played: v.games,
    wins: v.wins,
    draws: v.draws,
    losses: v.losses,
    goalsFor: v.goalsFor,
    goalsAgainst: v.goalsAgainst,
    recent: recentForm(teamId, history, 5),
    attackStrength: v.attack,
    defenseStrength: v.defense,
  };
}

/** Stable key for a fixture pairing, regardless of which side was at home. */
function pairKey(a: string, b: string): string {
  return a < b ? `${a}|${b}` : `${b}|${a}`;
}

function headToHead(
  pairHistory: PlayedMatch[],
  matchIds: Map<PlayedMatch, string>,
): HeadToHeadMatch[] {
  return pairHistory
    .slice(-H2H_LIMIT)
    .reverse()
    .map((m) => ({
      matchId: matchIds.get(m) ?? '',
      kickoffAt: m.kickoffAt,
      homeTeamId: m.homeTeamId,
      awayTeamId: m.awayTeamId,
      homeScore: m.homeScore,
      awayScore: m.awayScore,
    }));
}

function generate(): Dataset {
  const anchor = seasonAnchor();

  const teams: Team[] = [];
  const matches: Match[] = [];
  const predictions: Prediction[] = [];
  const results: PredictionResult[] = [];
  const reasoningByMatch = new Map<string, MatchReasoning>();

  LEAGUES.forEach((league, leagueIndex) => {
    const rng = makeRng(0x5eed_00 + leagueIndex * 7919);
    const latent = buildTeams(league, rng);
    const byId = new Map(latent.map((t) => [t.id, t]));
    teams.push(...latent.map(({ id, name, shortName, leagueId }) => ({ id, name, shortName, leagueId })));

    const scoring = LEAGUE_SCORING[league.id];
    const schedule = buildSchedule(
      latent.map((t) => t.id),
      TOTAL_ROUNDS,
    );

    // Matches, with results simulated for every round that has been played.
    const leagueMatches: Match[] = [];
    schedule.forEach((round, roundIndex) => {
      round.forEach(([homeId, awayId], i) => {
        const played = roundIndex <= LAST_PLAYED_ROUND;
        let homeScore: number | null = null;
        let awayScore: number | null = null;

        if (played) {
          const home = byId.get(homeId)!;
          const away = byId.get(awayId)!;
          const xgHome = Math.min(5, Math.max(0.1, home.attack * away.defense * scoring.home));
          const xgAway = Math.min(5, Math.max(0.1, away.attack * home.defense * scoring.away));
          homeScore = samplePoisson(xgHome, rng);
          awayScore = samplePoisson(xgAway, rng);
        }

        leagueMatches.push({
          id: `${league.id}-r${roundIndex}-${i}`,
          leagueId: league.id,
          homeTeamId: homeId,
          awayTeamId: awayId,
          kickoffAt: kickoffFor(anchor, roundIndex, i),
          status: played ? 'finished' : 'scheduled',
          homeScore,
          awayScore,
          round: roundIndex,
        });
      });
    });

    // Walk-forward prediction: round by round, using only earlier results.
    // The per-team and per-pairing indexes are maintained incrementally —
    // rescanning the whole league history for every fixture is what made this
    // slow once the league count grew.
    const playedSoFar: PlayedMatch[] = [];
    const playedMatchIds = new Map<PlayedMatch, string>();
    const historyByTeam = new Map<string, PlayedMatch[]>();
    const historyByPair = new Map<string, PlayedMatch[]>();

    const pushIndexed = (map: Map<string, PlayedMatch[]>, key: string, m: PlayedMatch) => {
      const list = map.get(key);
      if (list) list.push(m);
      else map.set(key, [m]);
    };

    for (let roundIndex = 0; roundIndex < TOTAL_ROUNDS; roundIndex++) {
      const roundMatches = leagueMatches.filter((m) => m.round === roundIndex);

      if (roundIndex >= BURN_IN_ROUNDS) {
        const baselines = computeLeagueBaselines(playedSoFar);

        for (const match of roundMatches) {
          // These indexes only contain rounds strictly before this one, so the
          // model cannot see the results it is about to be graded against.
          const homeHistory = historyByTeam.get(match.homeTeamId) ?? [];
          const awayHistory = historyByTeam.get(match.awayTeamId) ?? [];
          const homeStrengths = computeTeamStrengths(match.homeTeamId, homeHistory, baselines);
          const awayStrengths = computeTeamStrengths(match.awayTeamId, awayHistory, baselines);
          const { xgHome, xgAway } = expectedGoals(homeStrengths, awayStrengths, baselines);
          const matrix = buildScorelineMatrix(xgHome, xgAway);

          const createdAt = new Date(Date.parse(match.kickoffAt) - 2 * DAY_MS).toISOString();

          for (const market of MARKET_CODES) {
            const distribution = marketDistribution(matrix, market);
            const best = pickBest(distribution);
            const prediction: Prediction = {
              id: `${match.id}-${market}`,
              matchId: match.id,
              marketType: market,
              predictionValue: best.value,
              confidencePct: Math.round(best.probability * 1000) / 10,
              modelVersion: MODEL_VERSION,
              createdAt,
              distribution,
            };
            predictions.push(prediction);

            // Auto-grade as soon as the match has a result (FR-5).
            if (match.status === 'finished' && match.homeScore !== null && match.awayScore !== null) {
              const settled = settleMarket(market, match.homeScore, match.awayScore);
              results.push({
                predictionId: prediction.id,
                actualOutcome: settled,
                wasCorrect: isPickCorrect(market, best.value, settled),
                settledAt: new Date(Date.parse(match.kickoffAt) + 2 * 3_600_000).toISOString(),
              });
            }
          }

          reasoningByMatch.set(match.id, {
            matchId: match.id,
            xgHome,
            xgAway,
            homeForm: formSummary(match.homeTeamId, homeStrengths, 'home', homeHistory),
            awayForm: formSummary(match.awayTeamId, awayStrengths, 'away', awayHistory),
            headToHead: headToHead(
              historyByPair.get(pairKey(match.homeTeamId, match.awayTeamId)) ?? [],
              playedMatchIds,
            ),
            topScorelines: topScorelines(matrix, 6),
            modelVersion: MODEL_VERSION,
            sampleSize: { home: homeStrengths.totalGames, away: awayStrengths.totalGames },
          });
        }
      }

      // Only now do this round's results become visible to later rounds.
      for (const match of roundMatches) {
        if (match.status === 'finished' && match.homeScore !== null && match.awayScore !== null) {
          const played: PlayedMatch = {
            homeTeamId: match.homeTeamId,
            awayTeamId: match.awayTeamId,
            homeScore: match.homeScore,
            awayScore: match.awayScore,
            kickoffAt: match.kickoffAt,
          };
          playedSoFar.push(played);
          playedMatchIds.set(played, match.id);
          pushIndexed(historyByTeam, match.homeTeamId, played);
          pushIndexed(historyByTeam, match.awayTeamId, played);
          pushIndexed(historyByPair, pairKey(match.homeTeamId, match.awayTeamId), played);
        }
      }
    }

    matches.push(...leagueMatches);
  });

  matches.sort((a, b) => Date.parse(a.kickoffAt) - Date.parse(b.kickoffAt));

  const predictionsByMatch = new Map<string, Prediction[]>();
  for (const p of predictions) {
    const list = predictionsByMatch.get(p.matchId);
    if (list) list.push(p);
    else predictionsByMatch.set(p.matchId, [p]);
  }
  // Keep markets in the canonical display order within each match.
  const marketOrder = new Map<MarketCode, number>(MARKET_CODES.map((m, i) => [m, i]));
  for (const list of predictionsByMatch.values()) {
    list.sort((a, b) => marketOrder.get(a.marketType)! - marketOrder.get(b.marketType)!);
  }

  const leaguesById = new Map(LEAGUES.map((l) => [l.id, l]));
  const teamsById = new Map(teams.map((t) => [t.id, t]));
  const matchesById = new Map(matches.map((m) => [m.id, m]));

  // The pro tier sits on top of the priced fixtures, so it is built last.
  const tipsters = buildTipsters({
    matches,
    matchesById,
    predictionsByMatch,
    teamsById,
    leaguesById,
  });

  return {
    leagues: LEAGUES,
    leaguesById,
    teams,
    teamsById,
    matches,
    matchesById,
    predictions,
    predictionsByMatch,
    predictionsById: new Map(predictions.map((p) => [p.id, p])),
    results,
    resultsByPrediction: new Map(results.map((r) => [r.predictionId, r])),
    reasoningByMatch,
    tipsters,
    modelVersion: MODEL_VERSION,
  };
}

let cached: { anchor: number; data: Dataset } | null = null;

/** Memoised per UTC day so the fixture list rolls forward without churn. */
export function getDataset(): Dataset {
  const anchor = seasonAnchor();
  if (!cached || cached.anchor !== anchor) {
    cached = { anchor, data: generate() };
  }
  return cached.data;
}
