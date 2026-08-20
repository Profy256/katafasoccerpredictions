import type { Metadata } from 'next';
import { redirect } from 'next/navigation';
import { getSession } from '@/api/client';
import { signInAction } from '@/app/actions';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = { title: 'Sign in' };

/**
 * No self-registration here on purpose — there is no bootstrap-admin flag by
 * design (see DEPLOYMENT.md). An admin account is a normal account promoted
 * with `UPDATE users SET role = 'admin'`; this page only signs one in.
 */
export default async function LoginPage({ searchParams }: PageProps<'/login'>) {
  const params = await searchParams;
  const session = await getSession();

  if (session?.role === 'admin') redirect('/');

  const rawError = Array.isArray(params.error) ? params.error[0] : params.error;

  return (
    <div className="mx-auto max-w-md px-4 py-16 sm:px-6">
      <h1 className="text-2xl font-semibold tracking-tight">Katafa admin</h1>
      <p className="mt-3 text-sm leading-relaxed text-fg-muted">
        {session
          ? `Signed in as ${session.email}, which is not an admin account. Sign in with one that is.`
          : 'Sign in with an admin account.'}
      </p>

      <form action={signInAction} className="mt-8 space-y-4">
        <div>
          <label htmlFor="email">Email</label>
          <input id="email" name="email" type="email" required autoComplete="email" />
        </div>

        <div>
          <label htmlFor="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            required
            autoComplete="current-password"
          />
        </div>

        {rawError && <p className="text-xs text-crit-text">{rawError}</p>}

        <button
          type="submit"
          className="w-full rounded-lg bg-brand px-3 py-2.5 text-sm font-medium text-canvas hover:opacity-90"
        >
          Sign in
        </button>
      </form>
    </div>
  );
}
