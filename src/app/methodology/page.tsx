import type { Metadata } from 'next';
import Link from 'next/link';
import { getCoverageStats } from '@/api/client';
import { MARKETS } from '@/lib/markets';
import { MARKET_CODES } from '@/api/types';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Methodology',
  description:
    'How Katafa generates predictions: a Poisson goal-expectation model, what the confidence number means, and what the model cannot see.',
};

const STEPS = [
  {
    title: 'Measure team strength',
    body: 'For each team we compute how many goals it scores and concedes relative to the league average, split by whether it is playing at home or away. Recent matches count for more than old ones, and a team with only a couple of matches on record is pulled toward the league average rather than being handed an extreme rating on thin evidence.',
  },
  {
    title: 'Turn strength into expected goals',
    body: "A team's expected goals for a fixture is its attacking strength multiplied by the opponent's defensive weakness and the league's baseline scoring rate for that venue. Two numbers come out: one per side.",
  },
  {
    title: 'Build the scoreline matrix',
    body: 'Each side\'s goal count is modelled as a Poisson distribution around its expected goals. Combining them gives a probability for every possible scoreline — 0–0, 2–1, 3–2 and so on.',
  },
  {
    title: 'Sum the matrix into markets',
    body: 'Every published market is a region of that one matrix. Home win is the cells where the home side scored more; Over 2.5 is every cell totalling three or more goals; both teams to score is every cell where neither side is on zero. Because they come from the same matrix, the published markets can never contradict each other.',
  },
];

export default async function MethodologyPage() {
  const stats = await getCoverageStats();

  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Methodology</h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          There is no secret sauce here, and that is the point. A prediction you
          cannot interrogate is indistinguishable from a guess, so this page
          describes exactly how every number on the site is produced.
        </p>
      </header>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          A statistical model, not machine learning
        </h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          The engine is a Poisson goal-expectation model — the same well-understood
          approach that underpins most football pricing. It was chosen over a
          machine-learning model deliberately: its reasoning can be shown on every
          match page, it is straightforward to debug, and it is easy to demonstrate
          that it is not simply guessing.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Machine learning is not ruled out, but it has to earn its place. It will
          only replace this model once there is enough tracked history to backtest
          properly, and only if it beats this baseline on data it has never seen.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">How a prediction is made</h2>
        <ol className="mt-4 space-y-5">
          {STEPS.map((step, i) => (
            <li key={step.title} className="flex gap-4">
              <span
                aria-hidden
                className="mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-surface-hi text-xs font-semibold tabular-nums"
              >
                {i + 1}
              </span>
              <div>
                <h3 className="text-sm font-semibold">{step.title}</h3>
                <p className="mt-1.5 text-[15px] leading-relaxed text-fg-muted">{step.body}</p>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          What the confidence number means
        </h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          The percentage shown next to a pick is the model&rsquo;s own probability
          for that outcome. It is not a separate star rating, a marketing number,
          or a rounded-up version of something less flattering. If the model makes
          a pick at 64%, then 64% is genuinely what it thinks — which is why the{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            accuracy page
          </Link>{' '}
          can check whether picks published at 70% really do land about 70% of the
          time.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">How grading works</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Every prediction is written down before kickoff, tagged with the model
          version that produced it, and graded automatically once the match
          finishes. There is no human in the loop deciding which picks count.
          Losing predictions are never removed, re-scored, or quietly reworded —
          they stay on the record permanently, because a track record you can edit
          is not a track record.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          Model tips and analyst tips are not the same thing
        </h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Everything on this page describes the free daily tips, which come out
          of the model with no human involvement. The paid slips are different:
          they are written by people, who use judgement the model does not have
          and may disagree with it outright.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Both are graded the same way and neither can be edited after the fact.
          The model&rsquo;s record lives on the{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            accuracy page
          </Link>
          ; each analyst carries their own on their{' '}
          <Link href="/analysts" className="text-brand hover:underline">
            profile
          </Link>
          . Once a paid slip settles, its selections are published in full, so an
          analyst&rsquo;s record can be audited without paying for anything.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">What the model cannot see</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Being explicit about the limits matters more than looking clever:
        </p>
        <ul className="mt-4 space-y-2.5 text-[15px] leading-relaxed text-fg-muted">
          {[
            'Injuries, suspensions and squad rotation. A side missing its first-choice striker looks identical to the model.',
            'Motivation and context — dead rubbers, derbies, relegation six-pointers, a cup final three days later.',
            'Weather, travel and pitch condition, which matter more in some leagues than others.',
            'In-play events. A red card on eight minutes invalidates a pre-match number entirely.',
            'Newly promoted sides and early-season fixtures, where there is simply not enough history yet. These predictions are the least reliable on the site.',
          ].map((item) => (
            <li key={item} className="flex gap-3">
              <span aria-hidden className="mt-2 h-1 w-1 shrink-0 rounded-full bg-fg-dim" />
              {item}
            </li>
          ))}
        </ul>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">Markets covered</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          The Poisson matrix produces these markets natively, from one calculation:
        </p>
        <ul className="mt-4 flex flex-wrap gap-2">
          {MARKET_CODES.map((code) => (
            <li
              key={code}
              className="rounded-lg border border-line bg-surface px-2.5 py-1.5 text-xs text-fg-muted"
            >
              {MARKETS[code].displayName}
            </li>
          ))}
        </ul>
        <p className="mt-4 text-[15px] leading-relaxed text-fg-muted">
          Cards, corners and goalscorer markets are not published. They need
          separate data models rather than a goals model, and shipping them off the
          back of a goal-expectation matrix would be dishonest.
        </p>
      </section>

      <section className="mt-10 rounded-xl border border-line bg-surface p-5">
        <h2 className="text-sm font-semibold">Current build</h2>
        <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
          <dt className="text-fg-dim">Model version</dt>
          <dd className="text-right font-mono text-xs">{stats.modelVersion}</dd>
          <dt className="text-fg-dim">Leagues</dt>
          <dd className="text-right tabular-nums">{stats.leagues}</dd>
          <dt className="text-fg-dim">Teams tracked</dt>
          <dd className="text-right tabular-nums">{stats.teams}</dd>
          <dt className="text-fg-dim">Graded predictions</dt>
          <dd className="text-right tabular-nums">{stats.gradedPredictions.toLocaleString('en-GB')}</dd>
        </dl>
        <p className="mt-4 border-t border-line-soft pt-4 text-xs leading-relaxed text-fg-muted">
          This build runs on a simulated season rather than a live data feed, so
          the fixtures and results are not real. The model, the grading and the
          accuracy maths are the production logic — only the source of the football
          is standing in.
        </p>
      </section>
    </div>
  );
}
