import Image from 'next/image';
import Link from 'next/link';

export function SiteFooter() {
  return (
    <footer className="mt-16 border-t border-line">
      <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
          <div className="max-w-md">
            <p className="flex items-center gap-2 text-sm font-semibold">
              <Image
                src="/brand/katafa-mark-64.png"
                alt=""
                aria-hidden
                width={64}
                height={64}
                className="h-6 w-6"
              />
              Katafa
            </p>
            <p className="mt-2 text-sm leading-relaxed text-fg-muted">
              Every prediction published here is graded against the real result
              and kept on the record — including the ones that lost.
            </p>
          </div>
          <nav className="flex flex-col gap-2 text-sm" aria-label="Footer">
            <Link href="/" className="text-fg-muted hover:text-fg">
              Predictions
            </Link>
            <Link href="/accuracy" className="text-fg-muted hover:text-fg">
              Accuracy record
            </Link>
            <Link href="/methodology" className="text-fg-muted hover:text-fg">
              How the model works
            </Link>
            <Link href="/faq" className="text-fg-muted hover:text-fg">
              FAQ
            </Link>
          </nav>
        </div>

        <p className="mt-8 border-t border-line-soft pt-6 text-xs leading-relaxed text-fg-dim">
          Predictions are statistical estimates, not advice, and carry no
          guarantee of any outcome. Nothing here is a betting service. 18+.
        </p>
      </div>
    </footer>
  );
}
