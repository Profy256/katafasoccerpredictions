import { randomUUID } from 'node:crypto';
import type { NextRequest } from 'next/server';

import { hashToken, SESSION_COOKIE_NAME, userBySessionToken } from '@/server/auth/session';
import { loadConfig } from '@/server/config';
import { allowRequest } from '@/server/db';
import { AppError } from '@/server/domain/errors';
import type { User } from '@/server/domain/user';
import { errorResponse, problemResponse } from '@/server/http/render';
import { createLogger, type Logger } from '@/server/logging';

/**
 * The middleware chain, outermost first — port of backend/internal/api/middleware.go:
 *
 *   request id → logging → recovery → CORS → rate limit → session
 *
 * Recovery sits inside logging so a panic is still logged with its request id,
 * and inside request id so the response can carry one. Route handlers receive
 * a single ApiContext instead of a wrapped http.Handler, because Next.js
 * route files export one function per method; the chain is applied by withApi
 * around each export.
 */

export interface ApiContext {
  request: NextRequest;
  url: URL;
  requestId: string;
  ip: string;
  user: User | null;
  /** Dynamic path segments, e.g. { id } for /v1/matches/[id]. */
  params: Record<string, string>;
  log: Logger;
}

export type ApiHandler = (ctx: ApiContext) => Promise<Response> | Response;

// Public reads are limited per IP; writes per IP (there is no user yet when
// the limiter runs — see the note in the chain below).
const READ_LIMIT = 300;
const WRITE_LIMIT = 30;
const LIMIT_WINDOW_MS = 60_000;

/**
 * clientIP prefers X-Forwarded-For's first entry, which is the client as seen
 * by the edge. It is only trustworthy behind a proxy that overwrites it, which
 * is how this is deployed.
 */
function clientIP(request: NextRequest): string {
  const forwarded = request.headers.get('x-forwarded-for');
  if (forwarded) {
    const first = forwarded.split(',')[0];
    return first ? first.trim() : '';
  }
  const real = request.headers.get('x-real-ip');
  return real ?? 'unknown';
}

function allowedOrigins(): Set<string> {
  return new Set(loadConfig().allowedOrigins);
}

/**
 * CORS allows the configured Next.js origins only. There is no wildcard: most
 * reads are server-side, so the browser-facing surface is narrow by default.
 */
function applyCors(request: NextRequest, response: Response): void {
  const origin = request.headers.get('origin');
  if (origin && allowedOrigins().has(origin)) {
    response.headers.set('Access-Control-Allow-Origin', origin);
    response.headers.set('Access-Control-Allow-Credentials', 'true');
    response.headers.set('Vary', 'Origin');
  }
}

/** The preflight answer —204 plus the CORS headers, exactly as Go's cors middleware wrote it. */
function preflightResponse(request: NextRequest): Response {
  const response = new Response(null, { status: 204 });
  applyCors(request, response);
  response.headers.set('Access-Control-Allow-Methods', 'GET, POST, DELETE, OPTIONS');
  response.headers.set('Access-Control-Allow-Headers', 'Content-Type, X-Request-Id');
  response.headers.set('Access-Control-Max-Age', '600');
  return response;
}

/**
 * Rate limit bucket. Go resolves the session AFTER the limiter in the chain,
 * so the `user:` branch there never fires in practice and the bucket is
 * always per-IP — mirrored here deliberately rather than "fixed", because the
 * observable behaviour is what the diff harness compares.
 */
async function checkRateLimit(method: string, ip: string): Promise<RateLimitResult> {
  const isRead = method === 'GET' || method === 'HEAD' || method === 'OPTIONS';
  const limit = isRead ? READ_LIMIT : WRITE_LIMIT;

  try {
    const decision = await allowRequest(`ip:${ip}`, limit, LIMIT_WINDOW_MS);
    return decision;
  } catch {
    // A limiter that cannot reach Postgres must not take the site down with
    // it. Log and allow: availability beats a perfect limit on a read-mostly
    // public API.
    return { allowed: true, retryAfterSeconds: 0, unavailable: true };
  }
}

interface RateLimitResult {
  allowed: boolean;
  retryAfterSeconds: number;
  unavailable?: boolean;
}

async function resolveSession(request: NextRequest): Promise<User | null> {
  const cookie = request.cookies.get(SESSION_COOKIE_NAME);
  if (!cookie || cookie.value === '') return null;

  try {
    return await userBySessionToken(hashToken(cookie.value));
  } catch (error) {
    // A failed lookup must not reject the request: endpoints that require a
    // user say so themselves, and endpoints where auth is optional need the
    // request to continue either way.
    console.error(JSON.stringify({
      time: new Date().toISOString(),
      level: 'error',
      msg: 'session lookup',
      error: error instanceof Error ? error.message : String(error),
    }));
    return null;
  }
}

/**
 * Wraps a handler in the middleware chain and returns a Next.js route export.
 *
 * `routeContext` is the second argument Next passes to route handlers; its
 * `params` is a Promise whose shape depends on the route (Next 16) — dynamic
 * segments arrive as individual strings, catch-alls as string arrays. Handlers
 * that need a segment read it by name and validate it, so the cast here is
 * the boundary between Next's per-route types and our uniform context.
 */
export function withApi(handler: ApiHandler) {
  return async (
    request: NextRequest,
    routeContext?: { params?: Promise<unknown> },
  ): Promise<Response> => {
    const startedAt = Date.now();
    const requestId = request.headers.get('X-Request-Id') || randomUUID();
    const ip = clientIP(request);
    const url = new URL(request.url);
    const log = createLogger({ requestId, ip });

    // CORS preflight is answered before the limiter, so a browser's OPTIONS
    // costs the client nothing — same as Go.
    if (request.method === 'OPTIONS') {
      const preflight = preflightResponse(request);
      preflight.headers.set('X-Request-Id', requestId);
      log.info('request', {
        method: request.method,
        path: url.pathname,
        status: preflight.status,
        durationMs: Date.now() - startedAt,
      });
      return preflight;
    }

    let response: Response;
    try {
      const limit = await checkRateLimit(request.method, ip);
      if (!limit.allowed) {
        response = problemResponse(
          request,
          429,
          'rate-limited',
          'Too many requests',
          `Retry in ${limit.retryAfterSeconds} seconds`,
        );
        response.headers.set('Retry-After', String(limit.retryAfterSeconds));
      } else {
        const user = await resolveSession(request);
        const params = (routeContext?.params ? await routeContext.params : {}) as Record<
          string,
          string
        >;
        response = await handler({ request, url, requestId, ip, user, params, log });
      }
    } catch (error) {
      response = errorResponse(error, request, (message, err) => log.error(message, { err }));
    }

    response.headers.set('X-Request-Id', requestId);
    applyCors(request, response);
    log.info('request', {
      method: request.method,
      path: url.pathname,
      status: response.status,
      durationMs: Date.now() - startedAt,
    });
    return response;
  };
}

/** Export as OPTIONS on every /v1 route so preflight matches Go's behaviour. */
export const withOptions = withApi(async () => new Response(null, { status: 204 }));

/** Wraps a handler that needs a session. */
export function requireUser(handler: ApiHandler): ApiHandler {
  return async (ctx) => {
    if (!ctx.user) {
      return errorResponse(AppError.unauthorized(), ctx.request);
    }
    return handler(ctx);
  };
}

/** Wraps a handler that needs role = 'admin'. */
export function requireAdmin(handler: ApiHandler): ApiHandler {
  return async (ctx) => {
    if (!ctx.user) {
      return errorResponse(AppError.unauthorized(), ctx.request);
    }
    if (ctx.user.role !== 'admin') {
      return errorResponse(AppError.forbidden(), ctx.request);
    }
    return handler(ctx);
  };
}
