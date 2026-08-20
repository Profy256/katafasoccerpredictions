import Link from 'next/link';

/**
 * Rendered per request rather than prerendered.
 *
 * The root layout reads the session to decide what the header shows. A
 * statically generated 404 would bake in whatever that was at build time —
 * which is "signed out", for everyone, forever. It also means the build no
 * longer needs the API reachable just to emit this page.
 */
export const dynamic = 'force-dynamic';

export default function NotFound() {
  return (
    <div className="mx-auto flex max-w-2xl flex-col items-start px-4 py-20 sm:px-6">
      <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">404</p>
      <h1 className="mt-2 text-2xl font-semibold tracking-tight">Nothing here</h1>
      <p className="mt-3 text-[15px] leading-relaxed text-fg-muted">
        That page does not exist. Fixtures drop off the feed once they have been
        played, but their predictions stay on the accuracy record.
      </p>
      <div className="mt-6 flex gap-3">
        <Link
          href="/"
          className="rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90"
        >
          Back to predictions
        </Link>
        <Link
          href="/accuracy"
          className="rounded-lg border border-line px-3 py-2 text-sm text-fg-muted hover:text-fg"
        >
          Accuracy record
        </Link>
      </div>
    </div>
  );
}
