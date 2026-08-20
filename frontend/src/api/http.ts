import { cookies } from 'next/headers';

/**
 * Transport for the Go API.
 *
 * Everything in `client.ts` goes through here. The split that matters is
 * between the two request helpers below:
 *
 *  - `apiGet` is for public reads. It never touches cookies, which is what
 *    keeps the calling route statically cacheable — reading cookies in a
 *    server component opts the whole route into dynamic rendering.
 *  - `apiGetPrivate` forwards the session cookie and is always `no-store`. Use
 *    it only where the response genuinely varies by viewer.
 *
 * Getting that the wrong way round is not a style question: a cached response
 * from a private endpoint is one viewer's paid slip served to another.
 */

/** The session cookie the Go API issues. Must match `auth.SessionCookieName`. */
export const SESSION_COOKIE = 'katafa_session';

/**
 * Where the API lives.
 *
 * Server-side only, so this is deliberately not `NEXT_PUBLIC_`: the API URL can
 * be an internal address the browser never needs to resolve. It has no default
 * — a silent fallback to localhost in production produces a site that renders
 * empty rather than one that fails loudly.
 */
function baseURL(): string {
  const url = process.env.KATAFA_API_URL;
  if (!url) {
    throw new Error(
      'KATAFA_API_URL is not set. Point it at the Go API, e.g. http://localhost:8080',
    );
  }
  return url.replace(/\/+$/, '');
}

/** Cache policies, mirroring the Cache-Control the API sends. */
export const REVALIDATE = {
  /** Leagues, markets, packages, analysts — changes on a deploy, not a match. */
  reference: 3600,
  /** The fixture feed. */
  feed: 300,
  /** Accuracy and the settled ledger. */
  accuracy: 900,
  /** The frozen daily shortlist. */
  freeTips: 600,
  /** Landing-page counts. */
  stats: 600,
} as const;

/**
 * An error carrying the API's problem+json detail.
 *
 * `status` is kept because callers branch on it — a 404 from `/slips/{id}` is
 * a page that should render `notFound()`, not an error page.
 */
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

/** A public read. No cookies, so the route stays cacheable. */
export async function apiGet<T>(
  path: string,
  options: { revalidate?: number; tags?: string[] } = {},
): Promise<T> {
  const response = await fetch(baseURL() + path, {
    headers: { Accept: 'application/json' },
    next: {
      revalidate: options.revalidate ?? REVALIDATE.feed,
      ...(options.tags ? { tags: options.tags } : {}),
    },
  });
  if (!response.ok) throw await toError(response);
  return (await response.json()) as T;
}

/**
 * A read that varies by viewer. Forwards the session cookie and never caches.
 */
export async function apiGetPrivate<T>(path: string): Promise<T> {
  const response = await fetch(baseURL() + path, {
    headers: await authHeaders(),
    cache: 'no-store',
  });
  if (!response.ok) throw await toError(response);
  return (await response.json()) as T;
}

/**
 * `apiGetPrivate` for endpoints where "not signed in" is an ordinary answer
 * rather than a failure — `/auth/me` and `/purchases` are both 401 for a
 * logged-out visitor, which is not an error worth rendering.
 */
export async function apiGetPrivateOrNull<T>(path: string): Promise<T | null> {
  try {
    return await apiGetPrivate<T>(path);
  } catch (error) {
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
      return null;
    }
    throw error;
  }
}

export async function authHeaders(): Promise<HeadersInit> {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  return {
    Accept: 'application/json',
    ...(token ? { Cookie: `${SESSION_COOKIE}=${token}` } : {}),
  };
}

/**
 * A write. Returns the parsed body and the raw response, because the auth
 * routes need the `Set-Cookie` header off the latter.
 */
export async function apiPost<T>(
  path: string,
  body?: unknown,
): Promise<{ data: T; response: Response }> {
  const response = await fetch(baseURL() + path, {
    method: 'POST',
    headers: {
      ...(await authHeaders()),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  });
  if (!response.ok) throw await toError(response);

  // 204 has no body to parse.
  const data =
    response.status === 204 ? (undefined as T) : ((await response.json()) as T);
  return { data, response };
}

/**
 * Builds a query string, dropping empty values.
 *
 * Multi-value parameters are comma-separated rather than repeated keys,
 * matching what the API parses. An empty array is omitted entirely — sending
 * `markets=` would be an unknown-value 400 rather than "no filter".
 */
export function query(params: Record<string, string | number | string[] | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined) continue;
    if (Array.isArray(value)) {
      if (value.length === 0) continue;
      search.set(key, value.join(','));
      continue;
    }
    if (value === '') continue;
    search.set(key, String(value));
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : '';
}
