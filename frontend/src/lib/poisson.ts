import type { MarketCode } from '@/api/types';
import { MARKETS, OVER_UNDER_LINES } from './markets';

/**
 * The Poisson goal-expectation model described in PRD section 5.2.
 *
 * Every MVP market is derived by summing regions of a single scoreline
 * probability matrix, which is what keeps the published markets mutually
 * consistent (e.g. Over 2.5 can never disagree with the 1X2 numbers).
 */

/** Scorelines above this are truncated; residual mass is normalised away. */
export const MAX_GOALS = 10;

function logFactorial(n: number): number {
  let acc = 0;
  for (let i = 2; i <= n; i++) acc += Math.log(i);
  return acc;
}

/** P(X = k) for X ~ Poisson(lambda). */
export function poissonPmf(k: number, lambda: number): number {
  if (k < 0 || !Number.isFinite(lambda) || lambda <= 0) return k === 0 ? 1 : 0;
  // Computed in log space: lambda^k overflows well before k = MAX_GOALS otherwise.
  return Math.exp(-lambda + k * Math.log(lambda) - logFactorial(k));
}

/**
 * Joint probability of every scoreline up to `maxGoals` each side, assuming
 * the two teams' goal counts are independent Poisson draws.
 *
 * `matrix[i][j]` is P(home scores i, away scores j). Rows/cols are normalised
 * so the matrix sums to exactly 1 despite truncation.
 */
export function buildScorelineMatrix(
  xgHome: number,
  xgAway: number,
  maxGoals: number = MAX_GOALS,
): number[][] {
  const homeProbs: number[] = [];
  const awayProbs: number[] = [];
  for (let k = 0; k <= maxGoals; k++) {
    homeProbs.push(poissonPmf(k, xgHome));
    awayProbs.push(poissonPmf(k, xgAway));
  }

  const matrix: number[][] = [];
  let total = 0;
  for (let i = 0; i <= maxGoals; i++) {
    const row: number[] = [];
    for (let j = 0; j <= maxGoals; j++) {
      const p = homeProbs[i] * awayProbs[j];
      row.push(p);
      total += p;
    }
    matrix.push(row);
  }

  if (total > 0 && Math.abs(total - 1) > 1e-12) {
    for (let i = 0; i <= maxGoals; i++) {
      for (let j = 0; j <= maxGoals; j++) matrix[i][j] /= total;
    }
  }
  return matrix;
}

export interface OutcomeProbability {
  value: string;
  probability: number;
}

/** 1X2 probabilities: lower triangle / diagonal / upper triangle of the matrix. */
function oneXTwo(matrix: number[][]): { home: number; draw: number; away: number } {
  let home = 0;
  let draw = 0;
  let away = 0;
  for (let i = 0; i < matrix.length; i++) {
    for (let j = 0; j < matrix[i].length; j++) {
      const p = matrix[i][j];
      if (i > j) home += p;
      else if (i === j) draw += p;
      else away += p;
    }
  }
  return { home, draw, away };
}

/** P(both teams score) — every cell where neither side is on zero. */
function bttsYes(matrix: number[][]): number {
  let p = 0;
  for (let i = 1; i < matrix.length; i++) {
    for (let j = 1; j < matrix[i].length; j++) p += matrix[i][j];
  }
  return p;
}

/** P(total goals > line) — the goal-total bands of the matrix. */
function overLine(matrix: number[][], line: number): number {
  let p = 0;
  for (let i = 0; i < matrix.length; i++) {
    for (let j = 0; j < matrix[i].length; j++) {
      if (i + j > line) p += matrix[i][j];
    }
  }
  return p;
}

/**
 * Full probability distribution for one market, in the market's display order.
 *
 * Probabilities are 0..1 and sum to 1 for every market **except** Double
 * Chance, whose three outcomes overlap (each 1X2 result is covered by two of
 * them), so they sum to 2. Render these as independent bars, never as a
 * stacked "share of 100%" chart.
 */
export function marketDistribution(matrix: number[][], market: MarketCode): OutcomeProbability[] {
  switch (market) {
    case 'ONE_X_TWO': {
      const { home, draw, away } = oneXTwo(matrix);
      return [
        { value: 'HOME', probability: home },
        { value: 'DRAW', probability: draw },
        { value: 'AWAY', probability: away },
      ];
    }
    case 'DOUBLE_CHANCE': {
      const { home, draw, away } = oneXTwo(matrix);
      return [
        { value: '1X', probability: home + draw },
        { value: '12', probability: home + away },
        { value: 'X2', probability: draw + away },
      ];
    }
    case 'BTTS': {
      const yes = bttsYes(matrix);
      return [
        { value: 'YES', probability: yes },
        { value: 'NO', probability: 1 - yes },
      ];
    }
    default: {
      const line = OVER_UNDER_LINES[market];
      if (line === undefined) throw new Error(`Unknown market: ${market}`);
      const over = overLine(matrix, line);
      return [
        { value: 'OVER', probability: over },
        { value: 'UNDER', probability: 1 - over },
      ];
    }
  }
}

/** The outcome the model would publish as its pick: simply the most likely one. */
export function pickBest(distribution: OutcomeProbability[]): OutcomeProbability {
  return distribution.reduce((best, o) => (o.probability > best.probability ? o : best));
}

export interface Scoreline {
  home: number;
  away: number;
  probability: number;
}

/** The `n` most likely exact scorelines, for the match detail view. */
export function topScorelines(matrix: number[][], n = 6): Scoreline[] {
  const all: Scoreline[] = [];
  for (let i = 0; i < matrix.length; i++) {
    for (let j = 0; j < matrix[i].length; j++) {
      all.push({ home: i, away: j, probability: matrix[i][j] });
    }
  }
  return all.sort((a, b) => b.probability - a.probability).slice(0, n);
}

/**
 * The outcome a finished match actually settled to, per market.
 * This is the grading function behind FR-5.
 */
export function settleMarket(market: MarketCode, homeScore: number, awayScore: number): string {
  const total = homeScore + awayScore;
  switch (market) {
    case 'ONE_X_TWO':
      return homeScore > awayScore ? 'HOME' : homeScore === awayScore ? 'DRAW' : 'AWAY';
    case 'DOUBLE_CHANCE':
      // A double chance bet settles to whichever of the three pairs contains
      // the real result; '12' only wins when the match was not drawn.
      return homeScore > awayScore ? 'HOME_SIDE' : homeScore === awayScore ? 'DRAW_SIDE' : 'AWAY_SIDE';
    case 'BTTS':
      return homeScore > 0 && awayScore > 0 ? 'YES' : 'NO';
    default: {
      const line = OVER_UNDER_LINES[market];
      if (line === undefined) throw new Error(`Unknown market: ${market}`);
      return total > line ? 'OVER' : 'UNDER';
    }
  }
}

/** Whether a published pick won, given the settled outcome. */
export function isPickCorrect(market: MarketCode, pick: string, settled: string): boolean {
  if (market !== 'DOUBLE_CHANCE') return pick === settled;
  // '1X' covers home win and draw, '12' covers either win, 'X2' covers draw and away win.
  const covered: Record<string, string[]> = {
    '1X': ['HOME_SIDE', 'DRAW_SIDE'],
    '12': ['HOME_SIDE', 'AWAY_SIDE'],
    X2: ['DRAW_SIDE', 'AWAY_SIDE'],
  };
  return covered[pick]?.includes(settled) ?? false;
}

/** Human-readable form of a settled outcome, for the results ledger. */
export function settledOutcomeLabel(market: MarketCode, settled: string): string {
  if (market === 'DOUBLE_CHANCE') {
    return settled === 'HOME_SIDE' ? 'Home Win' : settled === 'DRAW_SIDE' ? 'Draw' : 'Away Win';
  }
  return MARKETS[market].outcomes.find((o) => o.value === settled)?.label ?? settled;
}
