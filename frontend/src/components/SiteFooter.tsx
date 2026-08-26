import Image from 'next/image';
import Link from 'next/link';
import { getLeagues } from '@/api/client';
import { leagueSlugMap } from '@/lib/leagues';

/**
 * Footer navigation doubles as the site's crawl spine: every league landing
 * page is reachable from every page on the site, which is what lets Google
 * discover /leagues/* without waiting to trip over a fixture in that
 * competition first. League data comes from the same cached call the pages
 * use; if the API is down, the section simply disappears rather than taking
 * the whole page with it.
 */
export async function SiteFooter() {
  const leagues = await getLeagues().catch(() => []);
  const leagueEntries = [...leagueSlugMap(leagues).entries()];

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
            <Link href="/fixtures" className="text-fg-muted hover:text-fg">
              Fixtures
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

        {leagueEntries.length > 0 && (
          <nav className="mt-8 border-t border-line-soft pt-6" aria-label="Leagues">
            <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
              Predictions by league
            </p>
            <ul className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-sm">
              {leagueEntries.map(([slug, league]) => (
                <li key={league.id}>
                  <Link
                    href={`/leagues/${slug}`}
                    className="text-fg-muted hover:text-brand hover:underline"
                  >
                    {league.name} predictions
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        )}

        <p className="mt-8 border-t border-line-soft pt-6 text-xs leading-relaxed text-fg-dim">
          Predictions are statistical estimates, not advice, and carry no
          guarantee of any outcome. Nothing here is a betting service. 18+.
        </p>
      </div>
    </footer>
  );
}
