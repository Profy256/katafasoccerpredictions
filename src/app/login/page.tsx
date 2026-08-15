import type { Metadata } from 'next';
import Link from 'next/link';
import { redirect } from 'next/navigation';
import { signInAction, signOutAction } from '@/app/actions';
import { getSession } from '@/lib/session';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = { title: 'Sign in' };

export default async function LoginPage({ searchParams }: PageProps<'/login'>) {
  const params = await searchParams;
  const session = await getSession();

  const rawNext = Array.isArray(params.next) ? params.next[0] : params.next;
  // Only ever redirect within this site.
  const next = rawNext && rawNext.startsWith('/') && !rawNext.startsWith('//') ? rawNext : '/pro';
  const hasError = params.error === 'missing';

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

  async function submit(formData: FormData) {
    'use server';
    await signInAction(formData);
    redirect(next);
  }

  return (
    <div className="mx-auto max-w-md px-4 py-16 sm:px-6">
      <h1 className="text-2xl font-semibold tracking-tight">Sign in</h1>
      <p className="mt-3 text-sm leading-relaxed text-fg-muted">
        An account is only needed to buy and keep track of slips. The free daily
        tips and the whole accuracy record stay open to everyone.
      </p>

      <form action={submit} className="mt-8 space-y-4">
        <input type="hidden" name="next" value={next} />

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
            autoComplete="current-password"
            placeholder="••••••••"
            className="mt-1.5 w-full rounded-lg border border-line bg-canvas px-3 py-2.5 text-sm outline-none placeholder:text-fg-dim focus:border-brand"
          />
          <p className="mt-1.5 text-[11px] text-fg-dim">
            Not checked in this build — see the notice below.
          </p>
        </div>

        {hasError && (
          <p className="text-xs text-crit-text">Enter an email address to continue.</p>
        )}

        <button
          type="submit"
          className="w-full rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Continue
        </button>
      </form>

      <p className="mt-6 rounded-lg border border-line bg-surface p-3 text-xs leading-relaxed text-fg-muted">
        <span className="font-semibold text-warn">Placeholder sign-in.</span> There
        is no user database yet, so any email is accepted and the password is
        ignored. Real accounts arrive with the Go backend, alongside MarzPay
        checkout.
      </p>
    </div>
  );
}
