import type { Metadata } from 'next';
import Link from 'next/link';

export const metadata: Metadata = {
  title: 'Sure Bets: What They Really Are (and What to Check For)',
  description:
    'What people mean by a "sure bet" — arbitrage, value betting and model probability explained honestly, plus how to tell a real edge from a tipster\'s sales pitch.',
  alternates: { canonical: '/sure-bet' },
};

/**
 * Targets the "sure bet" cluster. Distinct from "sure win": searchers here
 * are often asking about arbitrage or about tips that feel near-certain.
 */
export default function SureBetPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          &ldquo;Sure bet&rdquo;: the three things people mean, honestly
          explained
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          The phrase hides three very different ideas. Two of them are real
          concepts worth understanding. The third is how your money leaves
          your pocket.
        </p>
      </header>

      <div className="mt-10 space-y-10 text-[15px] leading-relaxed text-fg-muted">
        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            1. Arbitrage — the only mathematical sure bet, and why you&rsquo;ll
            rarely find it
          </h2>
          <p className="mt-3">
            A genuine sure bet exists when two bookmakers price the same event
            so differently that backing every outcome guarantees profit whatever
            the result. This is called arbitrage, it is real, and it has almost
            nothing to do with prediction.
          </p>
          <p className="mt-3">
            It also barely exists in practice: margins move within minutes,
            stakes get limited fast, and one leg being voided while the other
            stands can turn a &ldquo;risk-free&rdquo; bet into a loss. Nobody
            sells arbitrage opportunities — they use them.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            2. Value bets — what serious punters actually chase
          </h2>
          <p className="mt-3">
            A value bet is not a sure bet. It is a bet where you believe the
            true probability is higher than the odds imply. If your model says
            a team wins 50% of the time and the odds imply 40%, taking that bet
            repeatedly should profit over time — even though any single bet can
            still lose.
          </p>
          <p className="mt-3">
            This is exactly how our predictions are meant to be used: every pick
            comes with an indicative price derived from the model&rsquo;s own
            view, so you can compare it against any real price yourself. Whether
            the model&rsquo;s judgement is honest is not a matter of trust — it
            is on the record at{' '}
            <Link href="/accuracy" className="text-brand hover:underline">
              /accuracy
            </Link>
            , including our{' '}
            <Link href="/accuracy" className="text-brand hover:underline">
              calibration check
            </Link>{' '}
            of whether higher-confidence picks really do win more often.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            3. &ldquo;Sure bet&rdquo; as a sales pitch — the one to avoid
          </h2>
          <p className="mt-3">
            When a tipping site calls its picks sure bets, it is doing one of
            two things: misunderstanding probability, or selling certainty it
            cannot have. The test is always the same — show me every pick you
            have ever made, wins and losses, timestamped before kickoff. Scam
            operations survive precisely because most buyers never ask.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            The honest summary
          </h2>
          <p className="mt-3">
            No football prediction is a sure thing — see{' '}
            <Link href="/sure-win" className="text-brand hover:underline">
              why a sure win cannot exist
            </Link>
            . What exists instead is an edge you can verify: honest
            probabilities, published before kickoff, graded automatically, with
            losses kept on the record. That is what our{' '}
            <Link href="/" className="text-brand hover:underline">
              free daily predictions
            </Link>{' '}
            are, and it is the only version of this product that survives being
            checked.
          </p>
        </section>
      </div>
    </div>
  );
}
