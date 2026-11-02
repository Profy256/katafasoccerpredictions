import { createHash } from 'node:crypto';

/**
 * Response encoding: JSON bodies, ETags, cache headers, RFC 9457 problem+json.
 *
 * Port of backend/internal/api/render/render.go. The encoding is byte-for-byte
 * the one Go produces — compact JSON, HTML characters escaped, keys in
 * insertion order (construct objects in the Go struct's field order) — because
 * the Go↔TS diff harness compares raw bodies and an ETag that only differs
 * because of escaping would make every comparison a false failure.
 */

/** Namespaces the `type` field of problem responses. */
export const ERROR_BASE_URI = 'https://katafa.app/errors/';

/** Cache-Control values, one per endpoint class in docs/backend/API.md. */
export const CACHE = {
  /** leagues, packages, analysts */
  reference: 'public, max-age=3600',
  /** feed, match detail */
  feed: 'public, max-age=300',
  /** accuracy, settled ledger, analyst records */
  accuracy: 'public, max-age=900',
  /** today's shortlist */
  freeTips: 'public, max-age=600',
  /** coverage */
  stats: 'public, max-age=600',
  /**
   * Slip responses vary by whether the viewer owns each one, so they are never
   * shared-cacheable even though the metadata looks public.
   */
  private: 'private, no-store',
  none: 'no-store',
} as const;

export interface Problem {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
}

/**
 * Compact JSON with Go's HTML escaping (<, >, & as unicode escapes).
 */
export function encodeJSON(value: unknown): string {
  return JSON.stringify(value).replace(
    /[<>&]/g,
    (c) => `\\u${c.charCodeAt(0).toString(16).padStart(4, '0')}`,
  );
}

function weakETag(body: string): string {
  // First 16 bytes of SHA-256, base64url without padding — Go's
  // crypto/sha256 + encoding/base64.RawURLEncoding equivalent.
  const hash = createHash('sha256').update(body, 'utf8').digest();
  return `W/"${hash.subarray(0, 16).toString('base64url')}"`;
}

function etagMatches(header: string | null, etag: string): boolean {
  if (!header) return false;
  if (header.trim() === '*') return true;
  return header.split(',').some((candidate) => candidate.trim() === etag);
}

/**
 * A 200 with an ETag and the given Cache-Control.
 *
 * Published data is immutable, so ETag plus a max-age is safe here and lets
 * the Next.js data cache and any CDN do the work.
 */
export function jsonResponse(body: unknown, cacheControl: string, request: Request): Response {
  const encoded = encodeJSON(body);
  const etag = weakETag(encoded);

  const headers = new Headers({ ETag: etag });
  if (cacheControl !== '') headers.set('Cache-Control', cacheControl);

  // A conditional request that still matches costs the client nothing but a
  // round trip.
  if (etagMatches(request.headers.get('if-none-match'), etag)) {
    return new Response(null, { status: 304, headers });
  }

  headers.set('Content-Type', 'application/json; charset=utf-8');
  return new Response(encoded, { status: 200, headers });
}

/**
 * An explicit status with no caching. Used for writes, which are never
 * cacheable.
 */
export function statusResponse(status: number, body?: unknown): Response {
  const headers = new Headers({
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  });
  if (body === undefined) return new Response(null, { status, headers });
  return new Response(`${encodeJSON(body)}\n`, { status, headers });
}

export function noContentResponse(): Response {
  return new Response(null, {
    status: 204,
    headers: { 'Cache-Control': 'no-store' },
  });
}

const KIND_MAP: Record<
  string,
  { type: string; title: string; status: number; carriesDetail: boolean }
> = {
  'not-found': { type: 'not-found', title: 'Not found', status: 404, carriesDetail: false },
  unauthorized: {
    type: 'unauthorized',
    title: 'Authentication required',
    status: 401,
    carriesDetail: false,
  },
  forbidden: { type: 'forbidden', title: 'Not permitted', status: 403, carriesDetail: false },
  conflict: { type: 'conflict', title: 'State conflict', status: 409, carriesDetail: true },
  unprocessable: {
    type: 'unprocessable',
    title: 'Request rejected',
    status: 422,
    carriesDetail: true,
  },
  'invalid-request': {
    type: 'invalid-request',
    title: 'Invalid request',
    status: 400,
    carriesDetail: true,
  },
};

/**
 * Maps a domain error onto a problem+json response.
 *
 * Only the sentinel kinds carry their message to the client. Anything else is
 * a 500 with a generic detail, because an unexpected error's text tends to
 * contain a query, a column name, or a path.
 */
export function errorResponse(
  error: unknown,
  request: Request,
  log?: (message: string, error: unknown) => void,
): Response {
  if (isAppLikeError(error)) {
    const map = KIND_MAP[error.kind];
    // Key order is Go's struct order: type, title, status, detail, instance.
    const problem: Problem = {
      type: ERROR_BASE_URI + map.type,
      title: map.title,
      status: map.status,
    };
    if (map.carriesDetail) problem.detail = stripPrefix(error.message, error.kind);
    problem.instance = new URL(request.url).pathname;
    return writeProblem(problem);
  }

  log?.('unhandled error', error);
  return writeProblem({
    type: ERROR_BASE_URI + 'internal',
    title: 'Something went wrong',
    status: 500,
    instance: new URL(request.url).pathname,
  });
}

/** Writes a specific problem, for cases the sentinels do not cover. */
export function problemResponse(
  request: Request,
  status: number,
  kind: string,
  title: string,
  detail?: string,
): Response {
  // Key order is Go's struct order: type, title, status, detail, instance.
  const problem: Problem = {
    type: ERROR_BASE_URI + kind,
    title,
    status,
  };
  if (detail !== undefined && detail !== '') problem.detail = detail;
  problem.instance = new URL(request.url).pathname;
  return writeProblem(problem);
}

function writeProblem(problem: Problem): Response {
  return new Response(`${encodeJSON(problem)}\n`, {
    status: problem.status,
    headers: {
      'Content-Type': 'application/problem+json; charset=utf-8',
      'Cache-Control': 'no-store',
    },
  });
}

/**
 * Strips the sentinel prefix so the client sees the specific detail rather
 * than "conflict: slip … is not open".
 */
function stripPrefix(message: string, kind: string): string {
  const prefixes: Record<string, string> = {
    conflict: 'conflict: ',
    unprocessable: 'unprocessable: ',
    'invalid-request': 'validation failed: ',
  };
  const prefix = prefixes[kind];
  return prefix && message.startsWith(prefix) ? message.slice(prefix.length) : message;
}

interface AppLikeError {
  kind: string;
  message: string;
}

function isAppLikeError(error: unknown): error is AppLikeError {
  return (
    typeof error === 'object' &&
    error !== null &&
    'kind' in error &&
    'message' in error &&
    typeof (error as AppLikeError).kind === 'string'
  );
}
