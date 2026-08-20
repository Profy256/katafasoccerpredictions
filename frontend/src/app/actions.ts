'use server';

import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';
import { revalidatePath } from 'next/cache';
import { ApiError, SESSION_COOKIE, apiPost } from '@/api/http';
import type { PurchaseResult, SessionUser } from '@/api/types';
import { AD_GATES_COOKIE_NAME, getSeenAdGates } from '@/lib/ad-gates';
import { AD_GATE_TTL_SECONDS } from '@/lib/ads';

/**
 * Writes, against the Go API.
 *
 * The session is an opaque token the API issues; this file's only job on the
 * auth routes is to relay the `Set-Cookie` it returns to the browser. Nothing
 * here decides whether anyone is signed in or has paid — those answers come
 * from the API on every read.
 */

function readString(formData: FormData, key: string): string {
  const value = formData.get(key);
  return typeof value === 'string' ? value.trim() : '';
}

/** A relative path only. An open redirect is a phishing primitive. */
function safeNext(raw: string, fallback: string): string {
  return raw.startsWith('/') && !raw.startsWith('//') ? raw : fallback;
}

/**
 * Relays the API's session cookie to the browser.
 *
 * The API sets the real attributes (HttpOnly, Secure, SameSite, Max-Age); this
 * copies the token across the server-to-server hop, since the browser never
 * talks to the API directly. Attributes are re-stated rather than parsed out
 * of the header: getting them from one place we control beats string-parsing
 * a header we do not.
 */
async function relaySessionCookie(response: Response): Promise<void> {
  const header = response.headers.get('set-cookie');
  if (!header) return;

  const match = new RegExp(`${SESSION_COOKIE}=([^;]*)`).exec(header);
  if (!match) return;

  const token = match[1];
  const store = await cookies();

  // An empty value is the API expiring the session on logout.
  if (!token) {
    store.delete(SESSION_COOKIE);
    return;
  }

  const maxAge = /max-age=(\d+)/i.exec(header);
  store.set(SESSION_COOKIE, token, {
    httpOnly: true,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
    path: '/',
    ...(maxAge ? { maxAge: Number(maxAge[1]) } : {}),
  });
}

/** Turns an ApiError into a message on the form's own page. */
function redirectWithError(path: string, error: unknown): never {
  const message =
    error instanceof ApiError ? error.detail || error.message : 'Something went wrong';
  redirect(`${path}?error=${encodeURIComponent(message)}`);
}

/**
 * Records that the viewer has seen the interstitial for a market, so the gate
 * lets them through.
 *
 * Still acknowledged on click. Wire this to the ad network's completion
 * callback when one is chosen — see AdSlot. Until then the gate is disabled
 * anyway: `ADS_ENABLED` is false, so `isMarketGated` returns false for every
 * market and this action is unreachable from the UI.
 */
export async function acknowledgeAdGateAction(formData: FormData) {
  const market = readString(formData, 'market');
  const next = readString(formData, 'next');
  if (!market) return;

  const store = await cookies();
  const seen = await getSeenAdGates();
  seen.add(market);

  store.set(AD_GATES_COOKIE_NAME, encodeURIComponent([...seen].join(',')), {
    httpOnly: true,
    sameSite: 'lax',
    path: '/',
    maxAge: AD_GATE_TTL_SECONDS,
  });

  if (next) redirect(safeNext(next, '/'));
}

export async function signInAction(formData: FormData) {
  const email = readString(formData, 'email');
  const password = readString(formData, 'password');
  const next = safeNext(readString(formData, 'next'), '/pro');

  if (!email || !password) {
    redirect('/login?error=' + encodeURIComponent('Email and password are both required'));
  }

  let response: Response;
  try {
    ({ response } = await apiPost<{ user: SessionUser }>('/v1/auth/login', {
      email,
      password,
    }));
  } catch (error) {
    redirectWithError('/login', error);
  }

  await relaySessionCookie(response);
  redirect(next);
}

export async function registerAction(formData: FormData) {
  const email = readString(formData, 'email');
  const password = readString(formData, 'password');
  const name = readString(formData, 'name') || email.split('@')[0];
  const phone = readString(formData, 'phone');
  const next = safeNext(readString(formData, 'next'), '/pro');

  if (!email || !password) {
    redirect('/login?error=' + encodeURIComponent('Email and password are both required'));
  }

  let response: Response;
  try {
    ({ response } = await apiPost<{ user: SessionUser }>('/v1/auth/register', {
      email,
      password,
      name,
      ...(phone ? { phone } : {}),
    }));
  } catch (error) {
    redirectWithError('/login', error);
  }

  await relaySessionCookie(response);
  redirect(next);
}

export async function signOutAction() {
  try {
    await apiPost<void>('/v1/auth/logout');
  } catch {
    // The server-side session may already be gone. Clearing the local cookie
    // is what the user asked for either way.
  }
  (await cookies()).delete(SESSION_COOKIE);
  redirect('/');
}

/**
 * Starts a MarzPay mobile money collection for one slip.
 *
 * Returns as soon as the collection is in flight — the payer then approves a
 * prompt on their handset. The slip does *not* unlock here: it unlocks when
 * the entitlement query starts returning rows, which happens once the payment
 * completes via the webhook or, if that is lost, via reconciliation. Unlocking
 * optimistically would hand out picks for money that never arrived.
 */
export async function purchaseSlipAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  const phone = readString(formData, 'phone');
  if (!slipId) return;

  const target = `/pro/slips/${slipId}`;
  if (!phone) {
    redirect(`${target}?error=${encodeURIComponent('A mobile money number is required')}`);
  }

  let result: PurchaseResult;
  try {
    ({ data: result } = await apiPost<PurchaseResult>(
      `/v1/slips/${encodeURIComponent(slipId)}/purchase`,
      { phone_number: phone },
    ));
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      redirect(`/login?next=${encodeURIComponent(target)}`);
    }
    redirectWithError(target, error);
  }

  revalidatePath('/pro');
  revalidatePath(target);

  // The trace code is carried back so the page can show it: it is what appears
  // in the payer's SMS, and what support matches a payment from.
  redirect(`${target}?pending=${encodeURIComponent(result.traceCode)}`);
}
