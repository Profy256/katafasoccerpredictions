import Link from 'next/link';
import type { ReactNode } from 'react';
import type { SessionUser } from '@/api/types';
import { signOutAction } from '@/app/actions';

const NAV: Array<{ href: string; label: string }> = [
  { href: '/', label: 'Dashboard' },
  { href: '/slips', label: 'Slips' },
  { href: '/revenue', label: 'Revenue' },
  { href: '/payments', label: 'Payments' },
];

/**
 * The chrome every authenticated page shares: nav, signed-in-as, sign out.
 *
 * Not in layout.tsx — the layout doesn't know whether there's a session
 * without a request-scoped read, and /login must render without this chrome.
 * Each protected page fetches its own admin via `requireAdminSession` and
 * wraps its content here.
 */
export function Shell({ user, children }: { user: SessionUser; children: ReactNode }) {
  return (
    <div className="min-h-full">
      <header className="border-b border-line">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3 sm:px-6">
          <span className="text-sm font-semibold tracking-tight">Katafa admin</span>
          <nav className="flex flex-wrap gap-4 text-sm text-fg-muted">
            {NAV.map((item) => (
              <Link key={item.href} href={item.href} className="hover:text-fg">
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-xs text-fg-dim">
            <span>{user.email}</span>
            <form action={signOutAction}>
              <button type="submit" className="text-fg-muted hover:text-fg">
                Sign out
              </button>
            </form>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-8 sm:px-6">{children}</main>
    </div>
  );
}
