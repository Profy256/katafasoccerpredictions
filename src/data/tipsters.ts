import type {
  Analyst,
  League,
  Match,
  MarketCode,
  PackageCode,
  Prediction,
  Slip,
  Team,
  Tip,
  TipPackage,
  TipResult,
} from '@/api/types';
import { MARKETS } from '@/lib/markets';
import { isPickCorrect, settleMarket } from '@/lib/poisson';
import { makeRng } from './rng';

/**
 * The pro tier: human analysts, their slips, and the grading of those slips.
 *
 * Two structural facts drive this module:
 *
 *  1. Users buy **individual slips**, not subscriptions. The price therefore
 *     lives on the slip (`priceUgx`), set by the admin when the slip is
 *     entered, not on the package.
 *  2. Analyst tips are entered by hand, so a tip carries display strings
 *     (`marketLabel`, `selectionLabel`) as its source of truth. The structured
 *     `marketType`/`selectionValue` pair is optional and only present when the
 *     tip happens to land on a market we can grade automatically — anything
 *     else has to be settled by the admin.
 *
 * In this demo build every generated tip maps onto a tracked market so the
 * whole ledger can be auto-graded, but the shape deliberately allows for tips
 * that cannot be.
 */

/** Placeholder personas for the demo build — replace with the real roster. */
const ANALYST_SEEDS: (Omit<Analyst, 'id'> & { skill: number })[] = [
  {
    slug: 'jajja-mappesa',
    name: 'Jajja Mappesa',
    handle: '@jajjamappesa',
    initials: 'JM',
    bio: 'Long-time Kampala tipster. Focuses on East African leagues and low-scoring markets others overlook.',
    packages: ['vip', 'akatambula'],
    joinedAt: '2025-11-04T00:00:00.000Z',
    skill: 0.88,
  },
  {
    slug: 'timo',
    name: 'Timo',
    handle: '@timoanalysis',
    initials: 'T',
    bio: 'European leagues specialist. Builds slips around goals markets and team news.',
    packages: ['ordinary', 'vip'],
    joinedAt: '2026-01-18T00:00:00.000Z',
    skill: 0.62,
  },
  {
    slug: 'ssalongo-kato',
    name: 'Ssalongo Kato',
    handle: '@ssalongokato',
    initials: 'SK',
    bio: 'Volume tipster covering the Uganda Premier League and FKF Premier League week to week.',
    packages: ['ordinary'],
    joinedAt: '2026-03-02T00:00:00.000Z',
    skill: 0.42,
  },
  {
    slug: 'nabirye-sarah',
    name: 'Nabirye Sarah',
    handle: '@nabiryetips',
    initials: 'NS',
    bio: 'Data-led analyst. Prefers a small number of high-conviction selections per slip.',
    packages: ['vip', 'akatambula'],
    joinedAt: '2026-02-11T00:00:00.000Z',
    skill: 0.8,
  },
];

export const PACKAGES: Record<PackageCode, TipPackage> = {
  ordinary: {
    code: 'ordinary',
    name: 'Ordinary',
    tagline: 'The daily working slip',
    description:
      'A longer slip of everyday value selections across the leagues we cover. The entry point to the analysts’ work.',
    typicalPriceUgx: 2000,
    highlights: [
      'Around five selections per slip',
      'Published every matchday morning',
      'Mixed markets across all covered leagues',
    ],
  },
  vip: {
    code: 'vip',
    name: 'VIP',
    tagline: 'Fewer legs, higher conviction',
    description:
      'A tighter slip. The analysts drop anything they are not confident in, so there are fewer selections and more reasoning behind each one.',
    typicalPriceUgx: 5000,
    highlights: [
      'Three selections per slip',
      'Written reasoning on every pick',
      'Posted at least four hours before kickoff',
    ],
  },
  akatambula: {
    code: 'akatambula',
    name: 'AKATAMBULA',
    tagline: 'The one they stake themselves',
    description:
      'The flagship slip, assembled by hand and released only when the analysts agree it is worth putting out. Some days there is no Akatambula at all.',
    typicalPriceUgx: 20000,
    highlights: [
      'Two selections, maximum conviction',
      'Hand-entered by the admin, never auto-generated',
      'Not published every day',
    ],
  },
};

/** Markets each package tends to draw from. */
const PACKAGE_MARKETS: Record<PackageCode, MarketCode[]> = {
  ordinary: [
    'ONE_X_TWO',
    'DOUBLE_CHANCE',
    'BTTS',
    'OVER_UNDER_1_5',
    'OVER_UNDER_2_5',
    'OVER_UNDER_3_5',
  ],
  vip: ['ONE_X_TWO', 'BTTS', 'OVER_UNDER_2_5', 'DOUBLE_CHANCE'],
  akatambula: ['ONE_X_TWO', 'DOUBLE_CHANCE', 'OVER_UNDER_2_5'],
};

const TIPS_PER_PACKAGE: Record<PackageCode, number> = {
  ordinary: 5,
  vip: 3,
  akatambula: 2,
};

/** Bookmaker margin applied when turning a model probability into odds. */
const ODDS_MARGIN = 0.96;

/** Floor on the model probability an analyst will put on a slip (~4.4 max odds). */
const MIN_TIP_PROBABILITY = 0.22;

/** How many past days of slips to generate. */
const HISTORY_DAYS = 24;

export function oddsFromProbability(probability: number): number {
  if (probability <= 0.01) return 50;
  return Math.max(1.02, Math.round((ODDS_MARGIN / probability) * 100) / 100);
}

export interface TipsterInput {
  matches: Match[];
  matchesById: Map<string, Match>;
  predictionsByMatch: Map<string, Prediction[]>;
  teamsById: Map<string, Team>;
  leaguesById: Map<string, League>;
}

export interface TipsterData {
  analysts: Analyst[];
  analystsById: Map<string, Analyst>;
  analystsBySlug: Map<string, Analyst>;
  slips: Slip[];
  slipsById: Map<string, Slip>;
  tipsBySlip: Map<string, Tip[]>;
  tipResultsByTip: Map<string, TipResult>;
  tipsByAnalyst: Map<string, Tip[]>;
}

function utcDay(iso: string): string {
  return iso.slice(0, 10);
}

/** Deterministic shuffle so the demo slips are stable within a day. */
function shuffled<T>(items: T[], rng: () => number): T[] {
  const out = [...items];
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(rng() * (i + 1));
    [out[i], out[j]] = [out[j], out[i]];
  }
  return out;
}

export function buildTipsters(input: TipsterInput): TipsterData {
  const rng = makeRng(0x7a1b_2c3d);

  const analysts: Analyst[] = ANALYST_SEEDS.map((seed, i) => ({
    id: `analyst-${i + 1}`,
    slug: seed.slug,
    name: seed.name,
    handle: seed.handle,
    initials: seed.initials,
    bio: seed.bio,
    packages: seed.packages,
    joinedAt: seed.joinedAt,
  }));
  const skillById = new Map(analysts.map((a, i) => [a.id, ANALYST_SEEDS[i].skill]));

  // Only fixtures the engine actually priced can carry a tip.
  const predictable = input.matches.filter(
    (m) => (input.predictionsByMatch.get(m.id)?.length ?? 0) > 0,
  );

  const byDay = new Map<string, Match[]>();
  for (const match of predictable) {
    const day = utcDay(match.kickoffAt);
    const list = byDay.get(day);
    if (list) list.push(match);
    else byDay.set(day, [match]);
  }

  const settledDays: string[] = [];
  const openDays: string[] = [];
  for (const [day, dayMatches] of [...byDay.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    if (dayMatches.every((m) => m.status === 'finished')) settledDays.push(day);
    else if (dayMatches.some((m) => m.status === 'scheduled')) openDays.push(day);
  }

  const days = [...settledDays.slice(-HISTORY_DAYS), ...openDays.slice(0, 2)];

  const slips: Slip[] = [];
  const tipsBySlip = new Map<string, Tip[]>();
  const tipResultsByTip = new Map<string, TipResult>();
  const tipsByAnalyst = new Map<string, Tip[]>();

  /**
   * Ordinary and VIP slips are published by a single analyst under their own
   * name, which is what gives each of them a record worth reading. Akatambula
   * is the joint slip — every analyst on that package co-signs it.
   */
  const slipPlans: { day: string; packageCode: PackageCode; authors: Analyst[]; id: string }[] = [];

  for (const day of days) {
    for (const packageCode of ['ordinary', 'vip', 'akatambula'] as PackageCode[]) {
      const eligibleAnalysts = analysts.filter((a) => a.packages.includes(packageCode));
      if (eligibleAnalysts.length === 0) continue;

      if (packageCode === 'akatambula') {
        // Deliberately not an every-day product.
        if (rng() < 0.35) continue;
        slipPlans.push({
          day,
          packageCode,
          authors: eligibleAnalysts,
          id: `slip-${packageCode}-${day}`,
        });
        continue;
      }

      for (const analyst of eligibleAnalysts) {
        slipPlans.push({
          day,
          packageCode,
          authors: [analyst],
          id: `slip-${packageCode}-${analyst.slug}-${day}`,
        });
      }
    }
  }

  for (const plan of slipPlans) {
    {
      const { day, packageCode, authors: picked, id: slipId } = plan;
      const dayMatches = byDay.get(day) ?? [];
      const wanted = TIPS_PER_PACKAGE[packageCode];
      const candidates = shuffled(dayMatches, rng).slice(0, wanted);
      if (candidates.length === 0) continue;

      const tips: Tip[] = [];
      let totalOdds = 1;
      let allSettled = true;
      let wonTips = 0;

      candidates.forEach((match, index) => {
        const author = picked[index % picked.length];
        const skill = skillById.get(author.id) ?? 0.5;
        const predictions = input.predictionsByMatch.get(match.id) ?? [];

        const pool = PACKAGE_MARKETS[packageCode];
        const available = predictions.filter((p) => pool.includes(p.marketType));
        const prediction = available.length
          ? available[Math.floor(rng() * available.length)]
          : predictions[0];
        if (!prediction) return;

        const market = MARKETS[prediction.marketType];
        // A stronger analyst lands on the model's own favourite more often.
        // When they go against it they still lean toward the next most likely
        // outcome rather than picking blindly, so a deviation costs less than
        // a uniform random choice would.
        const takesFavourite = rng() < skill;
        let chosenValue = prediction.predictionValue;

        if (!takesFavourite) {
          const alternatives = market.outcomes
            .filter((o) => o.value !== prediction.predictionValue)
            .map((o) => ({
              value: o.value,
              weight:
                prediction.distribution.find((d) => d.value === o.value)?.probability ?? 0.01,
            }))
            // Analysts talk themselves into the second favourite, not into a
            // 6.0 shot. Without this floor a weak tipster's five-fold prices up
            // in the hundreds, which no real slip does.
            .filter((a) => a.weight >= MIN_TIP_PROBABILITY);

          if (alternatives.length === 0) {
            alternatives.push({ value: prediction.predictionValue, weight: 1 });
          }
          const totalWeight = alternatives.reduce((sum, a) => sum + a.weight, 0);
          let roll = rng() * totalWeight;
          chosenValue = alternatives[alternatives.length - 1].value;
          for (const alternative of alternatives) {
            roll -= alternative.weight;
            if (roll <= 0) {
              chosenValue = alternative.value;
              break;
            }
          }
        }

        const probability =
          prediction.distribution.find((d) => d.value === chosenValue)?.probability ?? 0.5;
        const odds = oddsFromProbability(probability);
        const home = input.teamsById.get(match.homeTeamId)!;
        const away = input.teamsById.get(match.awayTeamId)!;

        const tip: Tip = {
          id: `${slipId}-t${index}`,
          slipId,
          analystId: author.id,
          matchId: match.id,
          fixtureLabel: `${home.name} v ${away.name}`,
          marketLabel: market.displayName,
          selectionLabel:
            market.outcomes.find((o) => o.value === chosenValue)?.label ?? chosenValue,
          marketType: prediction.marketType,
          selectionValue: chosenValue,
          odds,
          kickoffAt: match.kickoffAt,
        };
        tips.push(tip);
        totalOdds *= odds;

        if (match.status === 'finished' && match.homeScore !== null && match.awayScore !== null) {
          const actual = settleMarket(prediction.marketType, match.homeScore, match.awayScore);
          const correct = isPickCorrect(prediction.marketType, chosenValue, actual);
          if (correct) wonTips++;
          tipResultsByTip.set(tip.id, {
            tipId: tip.id,
            wasCorrect: correct,
            actualOutcome: actual,
            settledAt: new Date(Date.parse(match.kickoffAt) + 2 * 3_600_000).toISOString(),
            settledBy: 'auto',
          });
        } else {
          allSettled = false;
        }

        const forAnalyst = tipsByAnalyst.get(author.id);
        if (forAnalyst) forAnalyst.push(tip);
        else tipsByAnalyst.set(author.id, [tip]);
      });

      if (tips.length === 0) continue;

      // Price is whatever the admin entered for this slip; the package figure
      // is only a typical value.
      const base = PACKAGES[packageCode].typicalPriceUgx;
      const priceUgx = Math.round((base * (0.9 + rng() * 0.35)) / 500) * 500;

      const earliest = tips.reduce(
        (min, t) => Math.min(min, Date.parse(t.kickoffAt)),
        Number.POSITIVE_INFINITY,
      );

      slips.push({
        id: slipId,
        packageCode,
        title: `${PACKAGES[packageCode].name} slip · ${day}`,
        analystIds: [...new Set(tips.map((t) => t.analystId))],
        publishedAt: new Date(earliest - 5 * 3_600_000).toISOString(),
        priceUgx,
        status: allSettled ? 'settled' : 'open',
        tipCount: tips.length,
        totalOdds: Math.round(totalOdds * 100) / 100,
        wonTips: allSettled ? wonTips : undefined,
      });
      tipsBySlip.set(slipId, tips);
    }
  }

  slips.sort((a, b) => Date.parse(b.publishedAt) - Date.parse(a.publishedAt));

  return {
    analysts,
    analystsById: new Map(analysts.map((a) => [a.id, a])),
    analystsBySlug: new Map(analysts.map((a) => [a.slug, a])),
    slips,
    slipsById: new Map(slips.map((s) => [s.id, s])),
    tipsBySlip,
    tipResultsByTip,
    tipsByAnalyst,
  };
}
