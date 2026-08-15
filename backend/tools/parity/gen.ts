/**
 * Emits the reference output of the TypeScript model, for the Go port to be
 * diffed against. Run via tsc + node; the JSON lands in
 * backend/internal/model/testdata/parity.json.
 */
import { MARKET_CODES } from './types';
import { buildScorelineMatrix, marketDistribution, pickBest, poissonPmf, topScorelines } from './poisson';
import {
  computeLeagueBaselines,
  computeTeamStrengths,
  expectedGoals,
  recentForm,
  type PlayedMatch,
} from './model';

// A deterministic pseudo-random league so both implementations see identical
// history. Mulberry32 — small, and reproducible across languages.
function mulberry32(seed: number) {
  return function () {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const TEAMS = ['t0', 't1', 't2', 't3', 't4', 't5', 't6', 't7'];

function buildHistory(count: number): PlayedMatch[] {
  const rng = mulberry32(20260814);
  const out: PlayedMatch[] = [];
  let day = Date.UTC(2025, 0, 4, 15, 0, 0);
  for (let i = 0; i < count; i++) {
    const h = i % TEAMS.length;
    const a = (h + 1 + (i % 3)) % TEAMS.length;
    if (h === a) continue;
    out.push({
      homeTeamId: TEAMS[h],
      awayTeamId: TEAMS[a],
      homeScore: Math.floor(rng() * 4),
      awayScore: Math.floor(rng() * 4),
      kickoffAt: new Date(day).toISOString(),
    });
    day += 86_400_000 / 2;
  }
  return out;
}

const XG_PAIRS: [number, number][] = [
  [1.5, 1.15],
  [0.15, 0.15],
  [4.75, 4.75],
  [2.31, 0.87],
  [0.42, 3.9],
  [1.0, 1.0],
  [3.14159, 2.71828],
  [0.9, 1.1],
];

const pmf: { k: number; lambda: number; p: number }[] = [];
for (const lambda of [0.15, 0.5, 1, 1.5, 2.75, 4.75]) {
  for (let k = 0; k <= 10; k++) pmf.push({ k, lambda, p: poissonPmf(k, lambda) });
}

const matrices = XG_PAIRS.map(([xgHome, xgAway]) => {
  const matrix = buildScorelineMatrix(xgHome, xgAway);
  return {
    xgHome,
    xgAway,
    sum: matrix.reduce((t, row) => t + row.reduce((s, p) => s + p, 0), 0),
    cells: matrix,
    distributions: MARKET_CODES.map((market) => ({
      market,
      outcomes: marketDistribution(matrix, market),
      best: pickBest(marketDistribution(matrix, market)),
    })),
    topScorelines: topScorelines(matrix, 6),
  };
});

const history = buildHistory(120);
const baselines = computeLeagueBaselines(history);

// Walk-forward: strengths are computed from a prefix of the history only, the
// way a real prediction would see it.
const cutoffs = [0, 1, 12, 40, 119];
const strengths = cutoffs.flatMap((cutoff) =>
  TEAMS.slice(0, 4).map((teamId) => {
    const prefix = history.slice(0, cutoff);
    const base = computeLeagueBaselines(prefix);
    return {
      cutoff,
      teamId,
      baselines: base,
      strengths: computeTeamStrengths(teamId, prefix, base),
      recentForm: recentForm(teamId, prefix, 5),
    };
  }),
);

const fixtures = cutoffs.map((cutoff) => {
  const prefix = history.slice(0, cutoff);
  const base = computeLeagueBaselines(prefix);
  const home = computeTeamStrengths('t0', prefix, base);
  const away = computeTeamStrengths('t3', prefix, base);
  return { cutoff, xg: expectedGoals(home, away, base) };
});

process.stdout.write(
  JSON.stringify({ pmf, matrices, history, baselines, strengths, fixtures }, null, 1),
);
