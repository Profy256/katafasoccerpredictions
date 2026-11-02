import { Pool, types, type PoolClient, type QueryResultRow } from 'pg';

/**
 * The connection pool. Postgres is the only datastore: it also carries the
 * rate limiter and the job lease, so there is one thing to back up and one
 * thing to restore.
 *
 * Port of backend/internal/postgres/db.go. The pool is deliberately much
 * smaller than Go's (16): this code runs in serverless function instances,
 * each with its own pool, and Neon's connection limit is the ceiling that
 * traffic spikes hit first. `allowExitOnIdle` lets a suspended instance drop
 * its connections instead of holding them.
 */

// Keep date/time columns as the raw strings Postgres emits instead of parsing
// them into JS Dates: Date rounds milliseconds and loses the exact fractional
// digits Go's time.Time preserves, which would break both the ETag and the
// Go↔TS diff harness. Responses format them in server/http/time.ts.
types.setTypeParser(1082, (value: string) => value); // date
types.setTypeParser(1114, (value: string) => value); // timestamp
types.setTypeParser(1184, (value: string) => value); // timestamptz

const MAX_CONNS = Number(process.env.DB_POOL_MAX ?? 4);

interface Globals {
  katafaPool?: Pool;
}

const globalScope = globalThis as unknown as Globals;

export function getPool(): Pool {
  if (!globalScope.katafaPool) {
    globalScope.katafaPool = new Pool({
      connectionString: process.env.DATABASE_URL,
      max: MAX_CONNS,
      idleTimeoutMillis: 30_000,
      connectionTimeoutMillis: 10_000,
      allowExitOnIdle: true,
      // Every timestamp in this system is TIMESTAMPTZ in UTC. Pinning the
      // session timezone means a server with a local TZ set cannot shift a
      // kickoff time, which would silently move a settlement window.
      options: '-c timezone=UTC',
      application_name: 'katafa',
    });
  }
  return globalScope.katafaPool;
}

export async function ping(): Promise<void> {
  const client = await getPool().connect();
  try {
    await client.query('SELECT 1');
  } finally {
    client.release();
  }
}

export async function query<T extends QueryResultRow = QueryResultRow>(
  sql: string,
  params: unknown[] = [],
): Promise<T[]> {
  const result = await getPool().query<T>(sql, params as unknown[]);
  return result.rows;
}

export async function queryOne<T extends QueryResultRow = QueryResultRow>(
  sql: string,
  params: unknown[] = [],
): Promise<T | null> {
  const rows = await query<T>(sql, params);
  return rows[0] ?? null;
}

export async function exec(sql: string, params: unknown[] = []): Promise<void> {
  await getPool().query(sql, params as unknown[]);
}

/**
 * Runs fn inside a transaction, rolling back on error.
 *
 * This is what makes settlement exactly-once rather than approximately-once:
 * the write and its completion marker commit in the same transaction, so a
 * crash mid-batch replays cleanly instead of leaving half a matchday graded.
 */
export async function withTransaction<T>(fn: (client: PoolClient) => Promise<T>): Promise<T> {
  const client = await getPool().connect();
  try {
    await client.query('BEGIN');
    const result = await fn(client);
    await client.query('COMMIT');
    return result;
  } catch (error) {
    await client.query('ROLLBACK').catch(() => undefined);
    throw error;
  } finally {
    client.release();
  }
}

/** What GET /readyz needs: can we reach Postgres at all. */
export async function pingDatabase(): Promise<{ ok: boolean; reason?: string }> {
  try {
    await ping();
  } catch {
    return { ok: false, reason: 'database unreachable' };
  }

  try {
    const version = await queryOne<{ applied: string | null }>(
      'SELECT max(version_id)::text AS applied FROM goose_db_version',
    );
    if (!version || version.applied === null) {
      return { ok: false, reason: 'migrations pending' };
    }
    const missing = await queryOne<{ missing: string | null }>(
      `SELECT to_regclass('public.predictions')::text AS missing`,
    );
    if (!missing?.missing) {
      return { ok: false, reason: 'migrations pending' };
    }
  } catch {
    return { ok: false, reason: 'migration state unknown' };
  }

  return { ok: true };
}

export interface RateLimitDecision {
  allowed: boolean;
  retryAfterSeconds: number;
}

/**
 * Fixed-window rate limiter kept in Postgres, so it survives a redeploy and
 * works across serverless instances without Redis.
 *
 * Port of backend/internal/postgres/users.go AllowRequest — the SQL is
 * verbatim, including the fact that a denied request still increments the
 * counter (so a client hammering the limit keeps paying for it).
 */
export async function allowRequest(
  bucket: string,
  limit: number,
  windowMs: number,
): Promise<RateLimitDecision> {
  const windowStartMs = Math.floor(Date.now() / windowMs) * windowMs;
  const windowStart = new Date(windowStartMs).toISOString();

  const rows = await query<{ count: string }>(
    `
    INSERT INTO rate_limits (bucket, window_start, count)
    VALUES ($1,$2,1)
    ON CONFLICT (bucket, window_start)
    DO UPDATE SET count = rate_limits.count + 1
    RETURNING count`,
    [bucket, windowStart],
  );
  const count = Number(rows[0].count);
  if (count > limit) {
    const remainingMs = windowStartMs + windowMs - Date.now();
    return { allowed: false, retryAfterSeconds: Math.floor(remainingMs / 1000) + 1 };
  }
  return { allowed: true, retryAfterSeconds: 0 };
}
