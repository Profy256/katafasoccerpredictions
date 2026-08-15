import { cookies } from 'next/headers';

/**
 * STUB AUTH AND STUB PURCHASES.
 *
 * There is no backend yet, so "signing in" writes a cookie and "buying" a slip
 * appends its id to another cookie. Nothing is verified, nothing is charged,
 * and anyone can forge either cookie.
 *
 * This exists so the locked/unlocked states are demoable end to end. When the
 * Go API lands, both reads below become authenticated calls and the writes
 * move behind MarzPay — see `docs` in the README. Do not ship this as-is.
 */

const SESSION_COOKIE = 'katafa_demo_session';
const PURCHASES_COOKIE = 'katafa_demo_purchases';

export interface Session {
  email: string;
  name: string;
}

export async function getSession(): Promise<Session | null> {
  const store = await cookies();
  const raw = store.get(SESSION_COOKIE)?.value;
  if (!raw) return null;
  try {
    const parsed = JSON.parse(decodeURIComponent(raw)) as Partial<Session>;
    if (!parsed.email) return null;
    return { email: parsed.email, name: parsed.name || parsed.email.split('@')[0] };
  } catch {
    return null;
  }
}

/** Slip ids the current viewer has "paid" for. */
export async function getOwnedSlipIds(): Promise<Set<string>> {
  const store = await cookies();
  const raw = store.get(PURCHASES_COOKIE)?.value;
  if (!raw) return new Set();
  return new Set(decodeURIComponent(raw).split(',').filter(Boolean));
}

const AD_GATES_COOKIE = 'katafa_seen_ads';

/** Market codes whose ad interstitial this viewer has already sat through. */
export async function getSeenAdGates(): Promise<Set<string>> {
  const store = await cookies();
  const raw = store.get(AD_GATES_COOKIE)?.value;
  if (!raw) return new Set();
  return new Set(decodeURIComponent(raw).split(',').filter(Boolean));
}

export const SESSION_COOKIE_NAME = SESSION_COOKIE;
export const PURCHASES_COOKIE_NAME = PURCHASES_COOKIE;
export const AD_GATES_COOKIE_NAME = AD_GATES_COOKIE;
