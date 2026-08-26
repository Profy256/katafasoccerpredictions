import type { Metadata } from 'next';
import Link from 'next/link';

export const metadata: Metadata = {
  title: 'Fixed Matches for Sale? How the Scam Works',
  description:
    'Someone is selling "fixed matches" — correct scores, insider info, mobile money first. Here is how the scam works, why no real fix is ever sold, and what to check instead.',
  alternates: { canonical: '/fixed-matches' },
};

/**
 * Targets the "fixed matches / correct score fixed / rigged match" cluster.
 * High volume, almost entirely served by fraud. The page exists to capture
 * that traffic and talk people out of the scam, not to imply access — this
 * site publishes statistical probabilities and has no insider knowledge of
 * anything, which is stated plainly here.
 */
export default function FixedMatchesPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Nobody selling &ldquo;fixed matches&rdquo; has a fixed match
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          If you have been offered a guaranteed correct score by someone with
          &ldquo;inside information&rdquo;, read this before you send any
          money. It is a scam, it is a well-known one, and here is exactly how
          it works.
        </p>
      </header>

      <div className="mt-10 space-y-10 text-[15px] leading-relaxed text-fg-muted">
        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            The arithmetic that exposes every fixed-match seller
          </h2>
          <p className="mt-3">
            Ask one question: <em>why are they selling it?</em>
          </p>
          <p className="mt-3">
            A person who genuinely knew a correct score in advance could stake
            modestly, avoid attention, and multiply their money quietly —
            correct-score odds run from 6/1 to 100/1. Instead, the offer is:
            send them money first, often a small amount, often by mobile money,
            often with urgency (&ldquo;kickoff is at 4!&rdquo;). Someone holding
            a certainty does not need your deposit. The product being sold is
            not information — the deposit <em>is</em> the product.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            How the scam scales
          </h2>
          <ul className="mt-3 list-disc space-y-2 pl-5">
            <li>
              Send tip A to 100 people and tip B to another 100. One group
              wins, and to that group the seller looks like a genius. Repeat,
              keeping only the believers.
            </li>
            <li>
              Post dozens of different &ldquo;confirmed&rdquo; scores across
              Telegram and WhatsApp groups; screenshot only the one that hit.
            </li>
            <li>
              Charge again for the &ldquo;VIP fixed league&rdquo; after one
              lucky-looking free win. The upsell is the business model.
            </li>
          </ul>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            The parts nobody mentions
          </h2>
          <p className="mt-3">
            Match-fixing is a serious crime in Uganda, Kenya and virtually
            everywhere else — for the fixer and, in many jurisdictions, for
            anyone knowingly betting on it. Real fixes leak to a handful of
            insiders, not to strangers on social media. And betting operators
            void markets when manipulation is detected, so even a real leak
            usually ends with stakes confiscated rather than paid.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            What actually works: checkable honesty
          </h2>
          <p className="mt-3">
            We will be direct about what we are, since we ask you to distrust
            everyone else: Katafa has no inside information about any match.
            We publish{' '}
            <Link href="/" className="text-brand hover:underline">
              statistical football predictions
            </Link>{' '}
            generated from historical results by a documented model, timestamped
            before kickoff, then graded automatically against the final score —{' '}
            <Link href="/accuracy" className="text-brand hover:underline">
              including every miss
            </Link>
            . Our hit rate is around what an honest model produces: good, far
            from perfect, and fully auditable.
          </p>
          <p className="mt-3">
            That is the standard to hold any tipster to: complete history,
            published before kickoff, losses included. Anyone who fails it —
            especially anyone promising certainty — should get nothing but your
            goodbye.
          </p>
          <p className="mt-3 border-t border-line-soft pt-6">
            See also:{' '}
            <Link href="/sure-win" className="text-brand hover:underline">
              why sure-win predictions don&rsquo;t exist
            </Link>{' '}
            and the{' '}
            <Link href="/faq" className="text-brand hover:underline">
              full FAQ
            </Link>
            .
          </p>
        </section>
      </div>
    </div>
  );
}
