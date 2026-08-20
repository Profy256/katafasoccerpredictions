'use client';

import Image from 'next/image';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

const NAV = [
  { href: '/', label: 'Free tips' },
  { href: '/pro', label: 'Pro' },
  { href: '/fixtures', label: 'Fixtures' },
  { href: '/accuracy', label: 'Accuracy' },
] as const;

export function SiteHeader({ signedIn }: { signedIn: boolean }) {
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-20 border-b border-line bg-canvas/85 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:gap-6 sm:px-6">
        {/* The label lives on the link because the wordmark below disappears
            at narrow widths, leaving the decorative mark as the only child. */}
        <Link
          href="/"
          aria-label="Katafa home"
          className="flex shrink-0 items-center gap-2.5"
        >
          <Image
            src="/brand/katafa-mark-64.png"
            alt=""
            aria-hidden
            width={64}
            height={64}
            priority
            className="h-8 w-8 shrink-0"
          />
          {/* The mark alone carries the brand on narrow screens — the wordmark
              is what pushes the nav off the edge at 390px. */}
          <span className="hidden text-[15px] font-semibold tracking-tight sm:inline">
            Katafa
          </span>
        </Link>

        <nav className="flex items-center gap-1" aria-label="Main">
          {NAV.map((item) => {
            // Only '/' needs an exact match; the rest own their subtrees.
            const active =
              item.href === '/' ? pathname === '/' : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-current={active ? 'page' : undefined}
                className={`rounded-lg px-2.5 py-1.5 text-[13px] transition-colors sm:px-3 sm:text-sm ${
                  active
                    ? 'bg-surface-hi text-fg'
                    : 'text-fg-muted hover:bg-surface hover:text-fg'
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="ml-auto flex items-center gap-3">
          <span className="hidden text-xs text-fg-dim lg:block">All times UTC</span>
          <Link
            href={signedIn ? '/pro' : '/login'}
            className="hidden rounded-lg border border-line px-2.5 py-1.5 text-[13px] text-fg-muted hover:border-brand/40 hover:text-brand sm:block"
          >
            {signedIn ? 'My slips' : 'Sign in'}
          </Link>
        </div>
      </div>
    </header>
  );
}
