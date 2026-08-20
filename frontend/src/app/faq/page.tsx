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
    q: 'What does "confidence" or "probability" mean next to a pick?',
    a: "It's the model's own stated probability for that outcome, not a marketing score. A pick shown at 70% confidence should, over a large enough sample, land correctly about 70% of the time — no more, no less. That means roughly 3 in 10 picks published at 70% are expected to lose. A 90% pick still loses 1 time in 10. See how the number is produced on the methodology page.",
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
        .
      </p>
    </div>
  );
}
