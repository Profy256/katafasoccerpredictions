import type { Metadata } from 'next';
import Link from 'next/link';
import { getAccuracySummary } from '@/api/client';
import { formatCount, formatRate } from '@/lib/format';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'Football Predictions FAQ',
  description:
    "Straight answers about football predictions: whether a \"sure win\" exists, what a confidence percentage actually promises, how accurate Katafa's picks are, and what each betting market means.",
  alternates: { canonical: '/faq' },
};

/**
 * Targets the "sure win / 100% sure / guaranteed" search cluster on purpose
 * — that volume is real — but answers it honestly instead of claiming it.
 * See AGENTS.md's non-negotiables: the product's only asset is a verifiable
 * track record, and a guaranteed-outcome claim is the one thing that would
 * make this page contradict every other page on the site.
 */
const FAQS = [
  {
    q: 'Is there such a thing as a "sure win" in football predictions?',
    a: "No, and any site telling you otherwise is selling a claim it can't back. Football has a genuine element of chance — a deflection, a red card, a refereeing decision — that no model, human tipster, or amount of data removes. What a good model can do is estimate a probability honestly and be checked against reality afterwards, which is what the accuracy page is for.",
  },
  {
    q: 'How confident is the model in each pick?',
    a: "Every published pick carries an internal probability, but we deliberately show you only the selection itself. What we publish instead is the audit: the accuracy page groups every graded pick by the confidence it was made at and shows whether higher-confidence picks really do win more often. A model whose 70%-confidence picks lose most of the time would not survive being checked like that.",
  },
  {
    q: "How accurate are Katafa's football predictions?",
    a: 'accuracy-live',
  },
  {
    q: 'What is a Both Teams To Score (BTTS) prediction?',
    a: 'A prediction on whether both sides will score at least one goal each in the match — "Yes" or "No" — independent of who wins. It resolves on goals scored, not on the final result.',
  },
  {
    q: 'What is a Double Chance tip?',
    a: 'A single selection that covers two of the three possible results: Home-or-Draw, Draw-or-Away, or Home-or-Away. It carries lower odds than a straight match-result pick because it wins more often.',
  },
  {
    q: 'What does an Over/Under 2.5 goals prediction mean?',
    a: 'A prediction on total goals scored by both teams combined, against a line of 2.5. "Over 2.5" wins if the match produces 3 or more goals; "Under 2.5" wins if it produces 2 or fewer. Katafa also publishes 1.5 and 3.5 goal lines.',
  },
  {
    q: 'Are the free daily tips different from the paid Pro slips?',
    a: "Yes. The free shortlist comes straight out of the statistical model with no human involved. Pro slips are hand-built by individual analysts, who bring judgement the model doesn't have and sometimes disagree with it outright. Both are graded the same automated way, and neither can be edited after publishing.",
  },
  {
    q: 'Is Katafa a betting site?',
    a: 'No. Katafa publishes predictions and, on the Pro tier, sells access to an analyst’s picks. It does not take bets, hold stakes, or pay out winnings. Any wagering is done elsewhere, entirely at your own judgement and risk.',
  },
  {
    q: 'What does 1X2 mean in football predictions?',
    a: '1X2 is the three-way match-result market: 1 means the home team wins, X means the match is a draw, and 2 means the away team wins. It is the most common football prediction market and the one our free shortlist leads with.',
  },
  {
    q: 'What is an Over 1.5 or Over 3.5 prediction?',
    a: 'Over/Under markets predict total goals against a fixed line. "Over 1.5" wins when the match produces at least 2 goals; "Under 2.5" wins when it produces 2 or fewer; "Over 3.5" needs 4 or more. Higher lines are harder to hit but pay more.',
  },
  {
    q: 'How are predictions graded?',
    a: 'Automatically. When a final score arrives, a deterministic function reads it and marks each published pick correct or incorrect — no human decides the outcome, and nothing is re-scored afterwards. The same function applies to every pick ever published.',
  },
  {
    q: 'Do you delete losing predictions?',
    a: 'No — that is the point of the record. Published predictions are immutable and every graded pick stays on the ledger, wins and losses alike. A site that only shows winners is not keeping a record; it is keeping advertisements.',
  },
  {
    q: 'How often are predictions published?',
    a: 'The free daily shortlist is selected and frozen once a day at 05:00 UTC, before any of that day’s matches kick off. Individual fixtures are priced as soon as the next rounds are ingested, which is typically several days ahead.',
  },
  {
    q: 'Which leagues does Katafa cover?',
    a: 'Europe’s top five competitions (Premier League, La Liga, Serie A, Bundesliga, Ligue 1) plus East African top flights including Uganda and Kenya — see the full league list linked in the footer. Coverage grows as history accumulates, because the model reads from our own stored results.',
  },
  {
    q: 'Are the predictions really free?',
    a: 'The daily model shortlist across all six markets is free to read. Pro slips — hand-built by named analysts — are sold individually in Ugandan shillings, and each analyst’s graded record is public whether you buy or not.',
  },
  {
    q: 'What happens to a prediction if a match is postponed?',
    a: 'A postponed match is not scored as a loss: its predictions simply wait until the rescheduled fixture is played and then grade against that result. If a match is cancelled outright, the picks are voided — removed from the record entirely rather than counted either way.',
  },
  {
    q: 'How much history does the model need for a prediction?',
    a: 'At least 40 prior matches per team in our database before a fixture is priced. That honesty about sample size matters: a probability built on six matches is a guess wearing a suit.',
  },
  {
    q: 'Why are kick-off times shown in UTC?',
    a: 'All timestamps on this site are UTC so that grading windows and publication times are unambiguous everywhere we have readers, from Kampala to Madrid. Convert to East Africa Time by adding 3 hours.',
  },
  {
    q: 'What is expected goals (xG)?',
    a: 'Expected goals measures the number of goals a team "should" score given the quality and quantity of its chances, adjusted for opponent and venue. It is the core input to our Poisson model — every published pick derives from each side’s xG expectation.',
  },
  {
    q: 'What is calibration?',
    a: 'Calibration checks whether stated probabilities match reality: picks published at 70% should win about 70% of the time. We publish our calibration chart publicly because an uncalibrated confidence number is just decoration.',
  },
  {
    q: 'Can I get a refund on a Pro slip?',
    a: 'If every selection on a slip is voided — for example all its matches were cancelled — the purchase is refunded automatically. A losing tip is not a void: slips that lose are settled as losses, not refunded.',
  },
  {
    q: 'Is there such a thing as a sure bet?',
    a: 'A true sure bet exists only in arbitrage between bookmakers, not in prediction. What tipping sites call sure bets is usually marketing. See our guide to what "sure bet" actually means for the three different things people intend by it.',
  },
] as const;

export default async function FaqPage() {
  const accuracy = await getAccuracySummary();
  const accuracyAnswer = `As of now, ${formatRate(accuracy.overall.hitRate)} of every graded prediction has landed, across ${formatCount(accuracy.overall.total)} settled picks — every one of them counted, wins and losses alike. That number moves; the live figure and its breakdown by market and league are always on the accuracy page.`;

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    mainEntity: FAQS.map((item) => ({
      '@type': 'Question',
      name: item.q,
      acceptedAnswer: {
        '@type': 'Answer',
        text: item.a === 'accuracy-live' ? accuracyAnswer : item.a,
      },
    })),
  };

  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      {/* Static-shaped JSON-LD; the one dynamic value (accuracyAnswer) is
          server-rendered text, not markup. */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />

      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Football predictions FAQ
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Including the question a lot of sites won&rsquo;t answer honestly.
        </p>
      </header>

      <dl className="mt-10 space-y-8">
        {FAQS.map((item) => (
          <div key={item.q}>
            <dt className="text-lg font-semibold tracking-tight">{item.q}</dt>
            <dd className="mt-2 text-[15px] leading-relaxed text-fg-muted">
              {item.a === 'accuracy-live' ? accuracyAnswer : item.a}
            </dd>
          </div>
        ))}
      </dl>

      <p className="mt-10 border-t border-line-soft pt-6 text-sm text-fg-muted">
        More detail on how predictions are produced and graded is on the{' '}
        <Link href="/methodology" className="text-brand hover:underline">
          methodology page
        </Link>
        , and the full, unfiltered record is on the{' '}
        <Link href="/accuracy" className="text-brand hover:underline">
          accuracy page
        </Link>
        . If you arrived here looking for guaranteed outcomes, read{' '}
        <Link href="/sure-win" className="text-brand hover:underline">
          why sure wins don&rsquo;t exist
        </Link>
        , what{' '}
        <Link href="/sure-bet" className="text-brand hover:underline">
          a &ldquo;sure bet&rdquo; really is
        </Link>
        , and how{' '}
        <Link href="/fixed-matches" className="text-brand hover:underline">
          fixed-match scams work
        </Link>
        .
      </p>
    </div>
  );
}
