import { cookies } from 'next/headers';

/**
 * Which ad interstitials this viewer has already sat through.
 *
 * This is the one piece of the old `src/lib/session.ts` that legitimately
 * stays a cookie. It is not an entitlement and grants nothing: the worst a
 * forged value can do is skip an advert, which costs an impression rather than
 * revenue. Everything else in that file — sign-in, purchases — moved to the
 * API, because those cookies were forgeable and did grant something.
 */

const AD_GATES_COOKIE = 'katafa_seen_ads';

/** Market codes whose interstitial this viewer has already seen. */
export async function getSeenAdGates(): Promise<Set<string>> {
  const store = await cookies();
  const raw = store.get(AD_GATES_COOKIE)?.value;
  if (!raw) return new Set();
  return new Set(decodeURIComponent(raw).split(',').filter(Boolean));
}

export const AD_GATES_COOKIE_NAME = AD_GATES_COOKIE;
