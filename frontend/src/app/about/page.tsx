import type { Metadata } from 'next';
import Link from 'next/link';

export const metadata: Metadata = {
  title: 'About Katafa Football Predictions',
  description:
    'Who publishes Katafa Football Predictions, how the picks are produced and graded, the corrections policy, and how the free model and the paid analyst slips are kept apart.',
  alternates: { canonical: '/about' },
};

/**
 * The entity page.
 *
 * A site that publishes predictions people may stake money on is judged on
 * who stands behind it, and Google reads the same signal: an About page that
 * names the publisher, the method and the corrections policy is the cheapest
 * trust asset a young domain can own. Everything asserted here is enforced
 * elsewhere in the system — the immutability rule is a database constraint,
 * not a promise on a marketing page.
 *
 * Contact details are env-driven rather than hardcoded: an address that
 * bounces is worse than none, and this file should not be the place a real
 * inbox gets invented.
 */
const CONTACT_EMAIL = process.env.NEXT_PUBLIC_CONTACT_EMAIL;

const POLICIES = [
  {
    title: 'Predictions are published before kickoff, or not at all',
    body: 'Every pick carries the timestamp it was written at, and the database refuses to accept a prediction for a match that has already started. A pick written after the whistle is not a prediction, and there is no code path that can create one.',
  },
  {
    title: 'Published predictions are never edited',
    body: 'Once a pick is public it is immutable. If something was wrong — a mispriced fixture, a fixture that was never real — the correction is published as a new entry alongside the original, with the original left standing. Nothing is quietly rewritten after the result is known.',
  },
  {
    title: 'Grading is automatic and mechanical',
    body: 'A settled pick is graded by reading the final score and applying a fixed rule per market. No judgement is involved, no model decides whether its own pick counted, and no human can mark a losing pick as a win.',
  },
  {
    title: 'The accuracy record includes everything',
    body: 'The published hit rate covers every settled prediction with no exclusions — no dropping the bad weeks, no "excluding postponed", no selecting a flattering date range. That is the only version of the number worth publishing, because it is the only one that can be checked.',
  },
  {
    title: 'Losing picks stay up',
    body: 'Settled predictions and settled analyst slips remain public permanently, wins and losses alike. History is what makes a record auditable rather than a claim, so it is never paywalled and never pruned.',
  },
];

export default function AboutPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          About Katafa Football Predictions
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Katafa Football Predictions publishes statistical predictions for
          football fixtures and keeps a public, automatically graded record of
          how they turned out. The record is the product. The predictions are
          just what gets measured.
        </p>
      </header>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">What Katafa is</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Two things sit on one dataset. The free side is a daily shortlist
          drawn from a Poisson goal-expectation model that prices every fixture
          we cover across six markets — match result, double chance, both teams
          to score, and over/under 1.5, 2.5 and 3.5 goals. The paid side is
          hand-built slips from human analysts, sold individually.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          These are kept deliberately separate, and so are their records. A
          model pick is a number produced by{' '}
          <Link href="/methodology" className="text-brand hover:underline">
            a published method
          </Link>{' '}
          you can check line by line. An analyst pick is a person&rsquo;s
          opinion. Both are graded the same way and{' '}
          <Link href="/accuracy" className="text-brand hover:underline">
            both records are public
          </Link>
          , but they are never presented as the same kind of claim.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">Editorial policy</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          These are not house style preferences. Each one is enforced in the
          database or the settlement code, because a rule that depends on
          everyone remembering it is not a rule.
        </p>
        <ol className="mt-5 space-y-5">
          {POLICIES.map((policy, i) => (
            <li key={policy.title} className="flex gap-4">
              <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-line text-xs font-medium tabular-nums text-fg-muted">
                {i + 1}
              </span>
              <div>
                <h3 className="text-[15px] font-semibold">{policy.title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-fg-muted">
                  {policy.body}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">Corrections</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Data providers occasionally report a wrong scoreline, and a fixture
          is sometimes abandoned or replayed after we have already graded it.
          Correcting a finished score is the one operation that can touch
          settled history, and it is deliberately not automatic: it requires a
          stated reason, it is written to an audit log, and it cannot silently
          rewrite a grade. Every prediction the correction affects is flagged
          and re-published as a correction next to the original, with the
          original left visible — including when that turns a recorded win into
          a recorded loss.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          If you believe a result or a graded pick on this site is wrong, tell
          us and we will check it against the source.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">
          What Katafa is not
        </h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Katafa is not a bookmaker, is not affiliated with one, and does not
          accept or place bets. Odds shown next to a free tip are derived from
          the model&rsquo;s own probability with a typical margin applied — they
          are there so a reader can judge whether a price is worth taking, and
          they are labelled as indicative rather than scraped from anyone&rsquo;s
          book.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Nothing published here is financial advice or a guarantee of any
          outcome. There is no such thing as a fixed match or a sure win, and
          we do not sell one —{' '}
          <Link href="/fixed-matches" className="text-brand hover:underline">
            we have written at length about why
          </Link>
          . A model that is right around 60% of the time is right about that
          often, which means it is wrong about four times in ten. Anyone
          staking money on that should be staking money they can lose. 18+.
        </p>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-semibold tracking-tight">Contact</h2>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          Corrections, data disputes, press and partnership enquiries all go to
          the same place.
        </p>
        {CONTACT_EMAIL ? (
          <p className="mt-3 text-[15px] leading-relaxed">
            <a
              href={`mailto:${CONTACT_EMAIL}`}
              className="text-brand hover:underline"
            >
              {CONTACT_EMAIL}
            </a>
          </p>
        ) : (
          <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
            Reach us through the{' '}
            <Link href="/faq" className="text-brand hover:underline">
              FAQ
            </Link>
            .
          </p>
        )}
      </section>

      <nav className="mt-12 border-t border-line pt-6" aria-label="Related">
        <ul className="flex flex-wrap gap-x-5 gap-y-2 text-sm">
          <li>
            <Link href="/methodology" className="text-brand hover:underline">
              How the model works
            </Link>
          </li>
          <li>
            <Link href="/accuracy" className="text-brand hover:underline">
              Accuracy record
            </Link>
          </li>
          <li>
            <Link href="/faq" className="text-brand hover:underline">
              FAQ
            </Link>
          </li>
          <li>
            <Link href="/predictions/today" className="text-brand hover:underline">
              Today&rsquo;s predictions
            </Link>
          </li>
        </ul>
      </nav>
    </div>
  );
}
