import { createHash, randomBytes } from 'node:crypto';

import { exec, queryOne } from '@/server/db';
import type { Role, User } from '@/server/domain/user';
import { formatTimestamptz } from '@/server/http/time';

/**
 * Opaque session tokens hashed at rest, not JWTs — a purchase must be
 * revocable immediately, and a stolen JWT stays valid until it expires. Slip
 * access is worth money, so "revoke now" has to mean now.
 *
 * Port of backend/internal/auth/session.go and the session queries in
 * backend/internal/postgres/users.go. The token is a fresh 32-byte random
 * value, base64url without padding; what is stored is its sha256.
 */

export const SESSION_COOKIE_NAME = 'katafa_session';

/** How long a session lasts before it must be re-established. */
export const SESSION_TTL_SECONDS = 30 * 24 * 60 * 60;

const TOKEN_BYTES = 32;

export function newToken(): { token: string; hash: Buffer } {
  const token = randomBytes(TOKEN_BYTES).toString('base64url');
  return { token, hash: hashToken(token) };
}

/** What is stored and what lookups compare against. */
export function hashToken(token: string): Buffer {
  return createHash('sha256').update(token, 'utf8').digest();
}

interface SessionRow {
  id: string;
  email: string;
  password_hash: string;
  name: string;
  phone: string | null;
  role: string;
  created_at: string;
}

/**
 * Resolves a session to its user, rejecting expired and revoked ones in the
 * query. Revocation must take effect immediately — entitlement to a paid slip
 * is worth money.
 *
 * @returns null when the token is unknown, expired or revoked.
 */
export async function userBySessionToken(tokenHash: Buffer): Promise<User | null> {
  const row = await queryOne<SessionRow>(
    `
    SELECT u.id, u.email, u.password_hash, u.name, u.phone, u.role, u.created_at
    FROM sessions s
    JOIN users u ON u.id = s.user_id
    WHERE s.token_hash = $1
      AND s.expires_at > now()
      AND s.revoked_at IS NULL`,
    [tokenHash],
  );
  if (!row) return null;

  return {
    id: row.id,
    email: row.email,
    passwordHash: row.password_hash,
    name: row.name,
    phone: row.phone,
    role: row.role as Role,
    createdAt: formatTimestamptz(row.created_at),
  };
}

export async function createSession(
  tokenHash: Buffer,
  userId: string,
  userAgent: string,
  expiresAt: Date,
): Promise<void> {
  await exec(
    `
    INSERT INTO sessions (token_hash, user_id, user_agent, expires_at)
    VALUES ($1,$2,$3,$4)`,
    [tokenHash, userId, userAgent, expiresAt.toISOString()],
  );
}

export async function revokeSession(tokenHash: Buffer): Promise<void> {
  await exec(
    `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
    [tokenHash],
  );
}

export interface CookieOptions {
  domain: string;
  /**
   * False only in local development, where there is no TLS. Derived from the
   * environment rather than exposed as its own setting, so it cannot be
   * switched off in production by editing one value.
   */
  secure: boolean;
}

/**
 * Serialises a cookie the way Go's net/http does — attribute order included,
 * so a Set-Cookie captured from either implementation is byte-identical.
 *
 * HttpOnly so script cannot read it, SameSite=Lax so it survives ordinary
 * top-level navigation from the site while not riding along on cross-site
 * POSTs.
 */
function serializeCookie(
  name: string,
  value: string,
  opts: CookieOptions,
  expires: Date,
  maxAge: number,
): string {
  const parts = [`${name}=${value}`, 'Path=/'];
  if (opts.domain !== '') parts.push(`Domain=${opts.domain}`);
  parts.push(`Expires=${expires.toUTCString()}`);
  parts.push(`Max-Age=${maxAge}`);
  parts.push('HttpOnly');
  if (opts.secure) parts.push('Secure');
  parts.push('SameSite=Lax');
  return parts.join('; ');
}

export function sessionCookie(token: string, opts: CookieOptions): string {
  return serializeCookie(
    SESSION_COOKIE_NAME,
    token,
    opts,
    new Date(Date.now() + SESSION_TTL_SECONDS * 1000),
    SESSION_TTL_SECONDS,
  );
}

/** Expires the cookie on logout. */
export function clearSessionCookie(opts: CookieOptions): string {
  return serializeCookie(SESSION_COOKIE_NAME, '', opts, new Date(0), -1);
}
