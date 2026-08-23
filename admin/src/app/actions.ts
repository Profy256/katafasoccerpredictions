'use server';

import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';
import { revalidatePath } from 'next/cache';
import { ApiError, SESSION_COOKIE } from '@/api/http';
import { apiSend } from '@/api/client';
import type { SessionUser } from '@/api/types';

/**
 * Writes, against the Go API. Every admin route requires role = 'admin' and
 * writes audit_log server-side — this file only relays requests and the
 * session cookie, and turns a rejected write back into a message on the
 * page that sent it. It decides nothing.
 */

function readString(formData: FormData, key: string): string {
  const value = formData.get(key);
  return typeof value === 'string' ? value.trim() : '';
}

async function relaySessionCookie(response: Response): Promise<void> {
  const header = response.headers.get('set-cookie');
  if (!header) return;

  const match = new RegExp(`${SESSION_COOKIE}=([^;]*)`).exec(header);
  if (!match) return;

  const token = match[1];
  const store = await cookies();

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

function redirectWithError(path: string, error: unknown): never {
  const message =
    error instanceof ApiError ? error.detail || error.message : 'Something went wrong';
  redirect(`${path}?error=${encodeURIComponent(message)}`);
}

export async function signInAction(formData: FormData) {
  const email = readString(formData, 'email');
  const password = readString(formData, 'password');

  if (!email || !password) {
    redirect('/login?error=' + encodeURIComponent('Email and password are both required'));
  }

  let response: Response;
  try {
    ({ response } = await apiSend<{ user: SessionUser }>('POST', '/v1/auth/login', {
      email,
      password,
    }));
  } catch (error) {
    redirectWithError('/login', error);
  }

  await relaySessionCookie(response);
  redirect('/');
}

export async function signOutAction() {
  try {
    await apiSend<void>('POST', '/v1/auth/logout');
  } catch {
    // The server-side session may already be gone; clearing the cookie is
    // what was asked for either way.
  }
  (await cookies()).delete(SESSION_COOKIE);
  redirect('/login');
}

export async function createAnalystAction(formData: FormData) {
  const name = readString(formData, 'name');
  const handle = readString(formData, 'handle');
  const initials = readString(formData, 'initials');
  const bio = readString(formData, 'bio');

  try {
    await apiSend('POST', '/v1/admin/analysts', { name, handle, initials, bio });
  } catch (error) {
    redirectWithError('/analysts', error);
  }

  revalidatePath('/analysts');
  redirect('/analysts');
}

export async function deactivateAnalystAction(formData: FormData) {
  const analystId = readString(formData, 'analystId');

  try {
    await apiSend('DELETE', `/v1/admin/analysts/${encodeURIComponent(analystId)}`);
  } catch (error) {
    redirectWithError('/analysts', error);
  }

  revalidatePath('/analysts');
  redirect('/analysts');
}

export async function createSlipAction(formData: FormData) {
  const packageCode = readString(formData, 'packageCode');
  const title = readString(formData, 'title');
  const priceUgx = readString(formData, 'priceUgx');
  const totalOdds = readString(formData, 'totalOdds');
  const tipCount = readString(formData, 'tipCount');
  const analystIds = formData.getAll('analystIds').map(String).filter(Boolean);

  let result: { id: string };
  try {
    ({ data: result } = await apiSend<{ id: string }>('POST', '/v1/admin/slips', {
      packageCode,
      title,
      priceUgx: Number(priceUgx),
      totalOdds,
      tipCount: Number(tipCount),
      analystIds,
    }));
  } catch (error) {
    redirectWithError('/slips', error);
  }

  redirect(`/slips/${result.id}`);
}

export async function addTipAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  const analystId = readString(formData, 'analystId');
  const fixtureLabel = readString(formData, 'fixtureLabel');
  const marketLabel = readString(formData, 'marketLabel');
  const selectionLabel = readString(formData, 'selectionLabel');
  const marketType = readString(formData, 'marketType');
  const selectionValue = readString(formData, 'selectionValue');
  const matchId = readString(formData, 'matchId');
  const odds = readString(formData, 'odds');
  const kickoffAt = readString(formData, 'kickoffAt');
  const note = readString(formData, 'note');
  const position = readString(formData, 'position');

  // Structured (matchId + marketType + selectionValue) or none of them — the
  // API rejects a partial set, so don't send one.
  const structured = Boolean(matchId && marketType && selectionValue);

  try {
    await apiSend('POST', `/v1/admin/slips/${encodeURIComponent(slipId)}/tips`, {
      analystId,
      fixtureLabel,
      marketLabel,
      selectionLabel,
      odds,
      kickoffAt: new Date(kickoffAt).toISOString(),
      position: Number(position),
      ...(structured ? { matchId, marketType, selectionValue } : {}),
      ...(note ? { note } : {}),
    });
  } catch (error) {
    redirectWithError(`/slips/${slipId}`, error);
  }

  revalidatePath(`/slips/${slipId}`);
  redirect(`/slips/${slipId}`);
}

interface BulkTipInput {
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  odds: string;
  kickoffAt: string;
  note?: string;
}

export async function bulkAddTipsAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  const analystId = readString(formData, 'analystId');
  const raw = readString(formData, 'tipsJson');

  let tips: BulkTipInput[];
  try {
    tips = JSON.parse(raw || '[]');
  } catch {
    redirectWithError(`/slips/${slipId}`, new Error('Could not read the pasted tips'));
    return;
  }

  try {
    await apiSend('POST', `/v1/admin/slips/${encodeURIComponent(slipId)}/tips/bulk`, {
      analystId,
      tips: tips.map((t) => ({
        fixtureLabel: t.fixtureLabel,
        marketLabel: t.marketLabel,
        selectionLabel: t.selectionLabel,
        odds: t.odds,
        kickoffAt: new Date(t.kickoffAt).toISOString(),
        ...(t.note ? { note: t.note } : {}),
      })),
    });
  } catch (error) {
    redirectWithError(`/slips/${slipId}`, error);
  }

  revalidatePath(`/slips/${slipId}`);
  redirect(`/slips/${slipId}`);
}

export async function publishSlipAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  try {
    await apiSend('POST', `/v1/admin/slips/${encodeURIComponent(slipId)}/publish`);
  } catch (error) {
    redirectWithError(`/slips/${slipId}`, error);
  }
  revalidatePath(`/slips/${slipId}`);
  redirect(`/slips/${slipId}`);
}

export async function deleteSlipAction(formData: FormData) {
  const slipId = readString(formData, 'slipId');
  try {
    await apiSend('DELETE', `/v1/admin/slips/${encodeURIComponent(slipId)}`);
  } catch (error) {
    redirectWithError(`/slips/${slipId}`, error);
  }
  redirect('/slips');
}

export async function settleTipAction(formData: FormData) {
  const tipId = readString(formData, 'tipId');
  const wasCorrect = readString(formData, 'wasCorrect') === 'true';
  const actualOutcome = readString(formData, 'actualOutcome');
  const reason = readString(formData, 'reason');

  try {
    await apiSend('POST', `/v1/admin/tips/${encodeURIComponent(tipId)}/settle`, {
      wasCorrect,
      actualOutcome,
      reason,
    });
  } catch (error) {
    redirectWithError(`/tips/${tipId}/settle`, error);
  }

  redirect(`/tips/${tipId}/settle?done=1`);
}

export async function lookupTraceAction(formData: FormData) {
  const traceCode = readString(formData, 'traceCode');
  if (!traceCode) redirect('/payments');
  redirect(`/payments/${encodeURIComponent(traceCode)}`);
}

export async function correctMatchAction(formData: FormData) {
  const matchId = readString(formData, 'matchId');
  const homeScore = readString(formData, 'homeScore');
  const awayScore = readString(formData, 'awayScore');
  const reason = readString(formData, 'reason');

  try {
    await apiSend('POST', `/v1/admin/matches/${encodeURIComponent(matchId)}/correct`, {
      homeScore: Number(homeScore),
      awayScore: Number(awayScore),
      reason,
    });
  } catch (error) {
    redirectWithError(`/matches/${matchId}/correct`, error);
  }

  redirect(`/matches/${matchId}/correct?done=1`);
}
