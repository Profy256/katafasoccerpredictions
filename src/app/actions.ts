'use server';

import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';
import { revalidatePath } from 'next/cache';
import {
  AD_GATES_COOKIE_NAME,
  PURCHASES_COOKIE_NAME,
  SESSION_COOKIE_NAME,
  getOwnedSlipIds,
  getSeenAdGates,
} from '@/lib/session';
import { AD_GATE_TTL_SECONDS } from '@/lib/ads';

/**
 * Stub auth and stub checkout. See `src/lib/session.ts` — no credential is
 * verified and no money moves. Real sign-in and MarzPay collection both belong
 * on the Go API; these actions are placeholders so the flow is walkable.
 */

const YEAR_SECONDS = 60 * 60 * 24 * 365;

function readString(formData: FormData, key: string): string {
  const value = formData.get(key);
  return typeof value === 'string' ? value.trim() : '';
}

/**
 * Records that the viewer has seen the interstitial for a market, so the gate
 * lets them through. Called by the gate's continue button.
 *
 * When a real ad network is wired up this must only be called once the network
 * confirms an impression — not simply because the button was clicked.
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

  if (next.startsWith('/') && !next.startsWith('//')) redirect(next);
}

export async function signInAction(formData: FormData) {
  const email = readString(formData, 'email');
  const name = readString(formData, 'name') || email.split('@')[0];
  const next = readString(formData, 'next') || '/pro';

  if (!email) redirect('/login?error=missing');

  const store = await cookies();
  store.set(SESSION_COOKIE_NAME, encodeURIComponent(JSON.stringify({ email, name })), {
    httpOnly: true,
    sameSite: 'lax',
    path: '/',
    maxAge: YEAR_SECONDS,
  });

  redirect(next);
}

export async function signOutAction() {
  const store = await cookies();
  store.delete(SESSION_COOKIE_NAME);
  store.delete(PURCHASES_COOKIE_NAME);
  redirect('/');
}

/**
 * Stands in for a MarzPay mobile-money collection. On the real thing this
 * would create a pending payment, hand off to MarzPay, and only unlock the
 * slip once the callback confirms settlement.
 */
export async function purchaseSlipAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  if (!slipId) return;

  const store = await cookies();
  const owned = await getOwnedSlipIds();
  owned.add(slipId);

  store.set(PURCHASES_COOKIE_NAME, encodeURIComponent([...owned].join(',')), {
    httpOnly: true,
    sameSite: 'lax',
    path: '/',
    maxAge: YEAR_SECONDS,
  });

  revalidatePath('/pro');
  revalidatePath(`/pro/slips/${slipId}`);
}
