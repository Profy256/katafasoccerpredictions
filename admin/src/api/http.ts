import { cookies } from 'next/headers';

/**
 * Transport for the Go API. Mirrors frontend/src/api/http.ts, trimmed to what
 * this app needs.
 *
 * Every request here carries the session cookie and is `no-store` — there is
 * no public, cacheable read in an admin panel. The API itself decides who is
 * an admin; this file's job is only to relay the cookie and surface its
 * errors.
 */

/** The session cookie the Go API issues. Must match `auth.SessionCookieName`. */
export const SESSION_COOKIE = 'katafa_session';

function baseURL(): string {
  const url = process.env.KATAFA_API_URL;
  if (!url) {
    throw new Error(
      'KATAFA_API_URL is not set. Point it at the Go API, e.g. http://localhost:8080',
    );
  }
  return url.replace(/\/+$/, '');
}

export class ApiError extends Error {
  readonly status: number;
  readonly detail: string;
  readonly type: string;

  constructor(status: number, title: string, detail: string, type: string) {
    super(detail ? `${title}: ${detail}` : title);
    this.name = 'ApiError';
    this.status = status;
    this.detail = detail;
    this.type = type;
  }
}

interface Problem {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
}

async function toError(response: Response): Promise<ApiError> {
  let problem: Problem = {};
  try {
    problem = (await response.json()) as Problem;
  } catch {
    // A non-JSON error body (a proxy's 502 page, say) is not worth failing
    // twice over. The status is the useful part.
  }
  return new ApiError(
    response.status,
    problem.title || response.statusText || 'Request failed',
    problem.detail || '',
    problem.type || 'about:blank',
  );
}

export async function authHeaders(): Promise<HeadersInit> {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  return {
    Accept: 'application/json',
    ...(token ? { Cookie: `${SESSION_COOKIE}=${token}` } : {}),
  };
}

/** A read that varies by admin. Always forwards the cookie, never caches. */
export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(baseURL() + path, {
    headers: await authHeaders(),
    cache: 'no-store',
  });
  if (!response.ok) throw await toError(response);
  return (await response.json()) as T;
}

/** `apiGet` for endpoints where "not signed in" is not an error worth rendering. */
export async function apiGetOrNull<T>(path: string): Promise<T | null> {
  try {
    return await apiGet<T>(path);
  } catch (error) {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
      return null;
    }
    throw error;
  }
}

/**
 * A write. Returns the parsed body and the raw response, because the auth
 * routes need the `Set-Cookie` header off the latter.
 */
export async function apiSend<T>(
  method: 'POST' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<{ data: T; response: Response }> {
  const response = await fetch(baseURL() + path, {
    method,
    headers: {
      ...(await authHeaders()),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  });
  if (!response.ok) throw await toError(response);

  const data = response.status === 204 ? (undefined as T) : ((await response.json()) as T);
  return { data, response };
}

export function query(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue;
    search.set(key, String(value));
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : '';
}
