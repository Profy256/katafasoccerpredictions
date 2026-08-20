import { apiGet, apiGetOrNull, apiSend, query } from './http';
import type {
  Analyst,
  AdminIngestStatus,
  MarketType,
  PaymentLedgerRow,
  PaymentTrace,
  RevenueReport,
  SessionUser,
  SlipWithTips,
} from './types';

/**
 * Everything this app calls on the Go API. Transport only — validation and
 * every rule that matters (immutability, kickoff cutoffs, entitlement) lives
 * in the backend; a rejected request here is just relayed as an ApiError for
 * the calling server action to show back to the admin.
 *
 * See docs/backend/API.md for the endpoint reference.
 */

export async function getSession(): Promise<SessionUser | null> {
  const body = await apiGetOrNull<{ user: SessionUser }>('/v1/auth/me');
  return body?.user ?? null;
}

/** Signed in *and* role = admin. Every page but /login gates on this. */
export async function requireAdminSession(): Promise<SessionUser | null> {
  const user = await getSession();
  return user && user.role === 'admin' ? user : null;
}

export async function getAnalysts(): Promise<Analyst[]> {
  return apiGet<Analyst[]>('/v1/analysts');
}

export async function getMarkets(): Promise<MarketType[]> {
  return apiGet<MarketType[]>('/v1/markets');
}

export async function getSlip(id: string): Promise<SlipWithTips | null> {
  try {
    return await apiGet<SlipWithTips>(`/v1/slips/${encodeURIComponent(id)}`);
  } catch (error) {
    if ((error as { status?: number }).status === 404) return null;
    throw error;
  }
}

export async function getIngestStatus(): Promise<AdminIngestStatus> {
  return apiGet<AdminIngestStatus>('/v1/admin/ingest/status');
}

export async function getRevenue(from?: string, to?: string): Promise<RevenueReport> {
  return apiGet<RevenueReport>(`/v1/admin/revenue${query({ from, to })}`);
}

export async function getPaymentLedger(
  from?: string,
  to?: string,
  limit?: number,
): Promise<{ from: string; to: string; rows: PaymentLedgerRow[] }> {
  return apiGet(`/v1/admin/payments${query({ from, to, limit })}`);
}

export async function getPaymentTrace(traceCode: string): Promise<PaymentTrace | null> {
  try {
    return await apiGet<PaymentTrace>(`/v1/admin/payments/${encodeURIComponent(traceCode)}`);
  } catch (error) {
    if ((error as { status?: number }).status === 404) return null;
    throw error;
  }
}

/* -------------------------------------------------------------------- *
 * Writes. Re-exported here so pages that only read don't need to know
 * these exist; actions.ts is what forms POST to.
 * -------------------------------------------------------------------- */

export { apiSend };
