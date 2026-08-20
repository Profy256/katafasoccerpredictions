import type { Metadata } from 'next';
import Link from 'next/link';
import { getSession } from '@/api/client';
import { registerAction, signInAction, signOutAction } from '@/app/actions';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = { title: 'Sign in' };

/** Mirrors the API's own floor. Stated here so the rejection is not a surprise. */
const MIN_PASSWORD_LENGTH = 10;

export default async function LoginPage({ searchParams }: PageProps<'/login'>) {
  const params = await searchParams;
  const session = await getSession();

  const rawNext = Array.isArray(params.next) ? params.next[0] : params.next;
  // Only ever redirect within this site.
  const next = rawNext && rawNext.startsWith('/') && !rawNext.startsWith('//') ? rawNext : '/pro';

  const rawError = Array.isArray(params.error) ? params.error[0] : params.error;
  const mode = (Array.isArray(params.mode) ? params.mode[0] : params.mode) === 'register'
    ? 'register'
    : 'signin';

  if (session) {
    return (
      <div className="mx-auto max-w-md px-4 py-16 sm:px-6">
        <h1 className="text-2xl font-semibold tracking-tight">Already signed in</h1>
        <p className="mt-3 text-sm leading-relaxed text-fg-muted">
          You are signed in as <span className="font-medium text-fg">{session.email}</span>.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link
            href="/pro"
            className="rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90"
          >
            Go to slips
          </Link>
          <form action={signOutAction}>
            <button
              type="submit"
              className="rounded-lg border border-line px-3 py-2 text-sm text-fg-muted hover:text-fg"
            >
              Sign out
            </button>
          </form>
        </div>
      </div>
    );
  }

  const registering = mode === 'register';

  return (
    <div className="mx-auto max-w-md px-4 py-16 sm:px-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {registering ? 'Create an account' : 'Sign in'}
      </h1>
      <p className="mt-3 text-sm leading-relaxed text-fg-muted">
        An account is only needed to buy and keep track of slips. The free daily
        tips and the whole accuracy record stay open to everyone.
      </p>

      <form
        action={registering ? registerAction : signInAction}
        className="mt-8 space-y-4"
      >
        <input type="hidden" name="next" value={next} />

        {registering && (
          <div>
            <label htmlFor="name" className="block text-xs font-medium text-fg-muted">
              Name
            </label>
            <input
              id="name"
              name="name"
              type="text"
              autoComplete="name"
              placeholder="Your name"
              className="mt-1.5 w-full rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
            />
          </div>
        )}

        <div>
          <label htmlFor="email" className="block text-xs font-medium text-fg-muted">
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            required
            autoComplete="email"
            placeholder="you@example.com"
            className="mt-1.5 w-full rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
          />
        </div>

        <div>
          <label htmlFor="password" className="block text-xs font-medium text-fg-muted">
            Password
          </label>
          <input
            id="password"
            name="password"
            type="password"
            required
            minLength={registering ? MIN_PASSWORD_LENGTH : undefined}
            autoComplete={registering ? 'new-password' : 'current-password'}
            placeholder="••••••••"
            className="mt-1.5 w-full rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
          />
          {registering && (
            <p className="mt-1.5 text-[11px] text-fg-dim">
              At least {MIN_PASSWORD_LENGTH} characters. Length is what helps —
              there are no composition rules.
            </p>
          )}
        </div>

        {registering && (
          <div>
            <label htmlFor="phone" className="block text-xs font-medium text-fg-muted">
              Mobile money number <span className="text-fg-dim">(optional)</span>
            </label>
            <input
              id="phone"
              name="phone"
              type="tel"
              autoComplete="tel"
              placeholder="0772 123 456"
              className="mt-1.5 w-full rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
            />
            <p className="mt-1.5 text-[11px] text-fg-dim">
              Saved for convenience only. You confirm the number on every purchase.
            </p>
          </div>
        )}

        {rawError && <p className="text-xs text-crit-text">{rawError}</p>}

        <button
          type="submit"
          className="w-full rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          {registering ? 'Create account' : 'Sign in'}
        </button>
      </form>

      <p className="mt-6 text-xs text-fg-muted">
        {registering ? (
          <>
            Already have an account?{' '}
            <Link
              href={`/login?next=${encodeURIComponent(next)}`}
              className="font-medium text-brand hover:underline"
            >
              Sign in
            </Link>
          </>
        ) : (
          <>
            New here?{' '}
            <Link
              href={`/login?mode=register&next=${encodeURIComponent(next)}`}
              className="font-medium text-brand hover:underline"
            >
              Create an account
            </Link>
          </>
        )}
      </p>
    </div>
  );
}
