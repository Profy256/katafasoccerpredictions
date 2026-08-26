import type { Metadata } from 'next';
import Link from 'next/link';

export const metadata: Metadata = {
  title: 'Sure Win Football Predictions: What Is Actually Possible',
  description:
    'Searching for sure win football predictions? Here is an honest answer: why a guaranteed win cannot exist, what a confidence percentage really promises, and how to check any tipster\'s record — including ours.',
  alternates: { canonical: '/sure-win' },
};

/**
 * Targets the "sure win / 100% sure / guaranteed win" search cluster — real
 * volume, mostly served by scam sites. The page answers the intent honestly
 * instead of making the claim: a guaranteed-outcome promise would contradict
 * this site's own graded record, which is its only asset. See AGENTS.md.
 */
export default function SureWinPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Sure win predictions don&rsquo;t exist. Here&rsquo;s what does.
        </h1>
        <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
          You searched for a sure win. Every honest person in football will give
          you the same answer — but there is something better than a promise:
          a record you can check.
        </p>
      </header>

      <div className="mt-10 space-y-10 text-[15px] leading-relaxed text-fg-muted">
        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            Why no football prediction can ever be &ldquo;sure&rdquo;
          </h2>
          <p className="mt-3">
            A football match contains events nobody can model: a deflection, a
            red card in the 20th minute, a VAR decision, a goalkeeper&rsquo;s
            best day of the season. These are not edge cases — they decide
            matches every single day, in every league in the world.
          </p>
          <p className="mt-3">
            A statistical model can estimate that one side wins, say, 60% of
            the time. That is genuinely useful knowledge. But it mathematically
            guarantees nothing about <em>this</em> match: 40% of the time, that
            same prediction loses. Anyone selling you certainty is either
            misunderstanding probability or lying to you.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            What a confidence percentage actually promises
          </h2>
          <p className="mt-3">
            When a model publishes a pick at 70%, the honest claim is narrow:{' '}
            <em>picks like this should win about 70% of the time over a large
            sample</em>. Which means roughly 3 in 10 of them lose. A 90% pick
            still loses 1 time in 10. That is not a weakness of the model — it
            is what a probability <em>is</em>.
          </p>
          <p className="mt-3">
            This is testable, and it should be tested. If a site says 70% but
            wins 45% of the time over hundreds of picks, its numbers are
            marketing, not statistics. We publish our own calibration check so
            you can run exactly this test on us.
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            How to check any tipster — including us
          </h2>
          <ul className="mt-3 list-disc space-y-2 pl-5">
            <li>
              <strong className="text-fg">Ask for every pick, wins and losses.</strong>{' '}
              A screenshot of ten wins proves nothing; selection bias is the
              oldest trick in tipping. Only a complete ledger means anything.
            </li>
            <li>
              <strong className="text-fg">Check picks existed before kickoff.</strong>{' '}
              A prediction written after the whistle isn&rsquo;t a prediction.
              Ours are timestamped before kickoff and cannot be edited once
              published.
            </li>
            <li>
              <strong className="text-fg">Count the losses yourself.</strong>{' '}
              Don&rsquo;t trust a headline hit rate — count through the graded
              history.
            </li>
          </ul>
          <p className="mt-3">
            Our full record — every graded prediction, misses included — is on
            the{' '}
            <Link href="/accuracy" className="text-brand hover:underline">
              accuracy page
            </Link>
            , and how the probabilities are produced is explained on the{' '}
            <Link href="/methodology" className="text-brand hover:underline">
              methodology page
            </Link>
            .
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            Red flags of a &ldquo;guaranteed win&rdquo; scam
          </h2>
          <ul className="mt-3 list-disc space-y-2 pl-5">
            <li>Claims of 100% accuracy, fixed scores for sale, or &ldquo;insider information&rdquo;.</li>
            <li>No public history — or a history with suspiciously no losses.</li>
            <li>Pressure to pay quickly by mobile money before a match starts.</li>
            <li>Refusing to show losing days, or re-posting tips after results are known.</li>
          </ul>
          <p className="mt-3">
            If someone genuinely had certain knowledge of a result, they would
            bet it themselves quietly — not sell it to strangers at pocket
            money prices. Read more in{' '}
            <Link href="/fixed-matches" className="text-brand hover:underline">
              our guide to fixed-match scams
            </Link>
            .
          </p>
        </section>

        <section>
          <h2 className="text-lg font-semibold tracking-tight text-fg">
            What we offer instead of certainty
          </h2>
          <p className="mt-3">
            Honest probabilities, published before kickoff, graded automatically
            against the real result — with every miss left permanently on the
            record. Free daily shortlists come straight from the{' '}
            <Link href="/" className="text-brand hover:underline">
              model
            </Link>
            . That is the whole product. It wins less often than a scam claims,
            and more honestly than anything else you&rsquo;ll find.
          </p>
          <p className="mt-3 border-t border-line-soft pt-6">
            More questions? The{' '}
            <Link href="/faq" className="text-brand hover:underline">
              FAQ
            </Link>{' '}
            covers what each market means, how grading works, and why we never
            delete a losing pick.
          </p>
        </section>
      </div>
    </div>
  );
}
