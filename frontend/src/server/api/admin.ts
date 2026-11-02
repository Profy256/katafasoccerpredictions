import { query, queryOne, withTransaction } from '@/server/db';
import { AppError } from '@/server/domain/errors';
import { parseUUID, parseDateOnly } from '@/server/api/query';
import { parseMarketCode } from '@/server/domain/market';
import { formatTimestamptz } from '@/server/http/time';
import { statusResponse } from '@/server/http/render';
import type { ApiHandler } from '@/server/http/middleware';

const MAX_BODY_BYTES = 256 * 1024;

async function decodeObject(request: Request, allowed: string[]): Promise<Record<string, unknown>> {
  const raw = await request.text();
  if (new TextEncoder().encode(raw).byteLength > MAX_BODY_BYTES) {
    throw AppError.invalid('request body too large');
  }
  if (raw === '') throw AppError.invalid('a JSON body is required');

  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw AppError.invalid('invalid character in JSON');
  }
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw AppError.invalid('JSON body must be an object');
  }
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) throw AppError.invalid(`unknown field ${JSON.stringify(key)}`);
  }
  return value as Record<string, unknown>;
}

function requiredString(body: Record<string, unknown>, key: string): string {
  const value = body[key];
  if (typeof value !== 'string') throw AppError.invalid(`${key} must be a string`);
  return value;
}

function optionalString(body: Record<string, unknown>, key: string): string | null {
  const value = body[key];
  if (value === undefined || value === null) return null;
  if (typeof value !== 'string') throw AppError.invalid(`${key} must be a string`);
  return value;
}

function requiredInt(body: Record<string, unknown>, key: string): number {
  const value = body[key];
  if (typeof value !== 'number' || !Number.isSafeInteger(value)) {
    throw AppError.invalid(`${key} must be an integer`);
  }
  return value;
}

function decimalString(body: Record<string, unknown>, key: string, minimum: number): string {
  const value = requiredString(body, key);
  if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value) || Number(value) < minimum) {
    throw AppError.invalid(`${key} must be a decimal string ${minimum === 1 ? 'of at least 1' : 'greater than 1'}`);
  }
  return value;
}

function requiredDateTime(body: Record<string, unknown>, key: string): string {
  const value = requiredString(body, key);
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)) {
    throw AppError.invalid(`${key} must be an RFC3339 timestamp`);
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) throw AppError.invalid(`${key} must be an RFC3339 timestamp`);
  return date.toISOString();
}

function optionalUUID(body: Record<string, unknown>, key: string, label: string): string | null {
  const value = body[key];
  if (value === undefined || value === null) return null;
  if (typeof value !== 'string') throw AppError.invalid(`${key} must be a UUID`);
  return parseUUID(value, label);
}

function auditJSON(value: unknown): string {
  return JSON.stringify(value);
}

function analystSlug(name: string): string {
  const slug = name
    .replace(/[A-Z]/g, (char) => char.toLowerCase())
    .replace(/[^a-z0-9 _-]/g, '')
    .replace(/[ _-]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return slug || 'analyst';
}

export const handleAdminCreateAnalyst: ApiHandler = async (ctx) => {
  const body = await decodeObject(ctx.request, ['name', 'handle', 'initials', 'bio']);
  const name = requiredString(body, 'name');
  const handle = requiredString(body, 'handle');
  const initials = requiredString(body, 'initials');
  const bio = body.bio === undefined || body.bio === null ? '' : requiredString(body, 'bio');
  if (name === '') throw AppError.invalid('name is required');
  if (handle === '') throw AppError.invalid('handle is required');
  if (initials === '') throw AppError.invalid('initials is required');

  const slug = analystSlug(name);
  const analyst = await withTransaction(async (client) => {
    let inserted;
    try {
      inserted = await client.query<{ id: string; joined_at: string }>(
        `INSERT INTO analysts (slug, name, handle, initials, bio, joined_at, is_active)
         VALUES ($1, $2, $3, $4, $5, now(), TRUE)
         RETURNING id, joined_at`,
        [slug, name, handle, initials, bio],
      );
    } catch (error) {
      if (typeof error === 'object' && error !== null && 'code' in error && error.code === '23505') {
        throw AppError.conflict('an analyst with that name or handle already exists');
      }
      throw error;
    }
    const row = inserted.rows[0];
    const result = {
      id: row.id,
      slug,
      name,
      handle,
      initials,
      bio,
      packages: null,
      joinedAt: row.joined_at,
    };
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, after)
       VALUES ('admin', $1, 'analyst.created', 'analyst', $2, $3::jsonb)`,
      [ctx.user!.id, row.id, JSON.stringify({ handle, name })],
    );
    return result;
  });

  return statusResponse(201, analyst);
};

export const handleAdminDeactivateAnalyst: ApiHandler = async (ctx) => {
  const analystID = parseUUID(ctx.params.id ?? '', 'analyst id');
  await withTransaction(async (client) => {
    const changed = await client.query(
      'UPDATE analysts SET is_active = FALSE WHERE id = $1 RETURNING id',
      [analystID],
    );
    if (changed.rowCount === 0) throw AppError.notFound();
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id)
       VALUES ('admin', $1, 'analyst.deactivated', 'analyst', $2)`,
      [ctx.user!.id, analystID],
    );
  });
  return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } });
};

export const handleAdminCreateSlip: ApiHandler = async (ctx) => {
  const body = await decodeObject(ctx.request, [
    'packageCode', 'title', 'priceUgx', 'totalOdds', 'tipCount', 'analystIds',
  ]);
  const packageCode = requiredString(body, 'packageCode');
  if (!['ordinary', 'vip', 'akatambula'].includes(packageCode)) {
    throw AppError.invalid(`unknown package code ${JSON.stringify(packageCode)}`);
  }
  const title = requiredString(body, 'title');
  if (title === '') throw AppError.invalid('title is required');
  const priceUgx = requiredInt(body, 'priceUgx');
  if (priceUgx <= 0) throw AppError.invalid('priceUgx must be positive');
  const totalOdds = decimalString(body, 'totalOdds', 1);
  const tipCount = requiredInt(body, 'tipCount');
  if (tipCount <= 0) throw AppError.invalid('tipCount must be positive');
  if (!Array.isArray(body.analystIds) || body.analystIds.length === 0) {
    throw AppError.invalid('at least one analyst is required');
  }
  const analystIDs = body.analystIds.map((value) => {
    if (typeof value !== 'string') throw AppError.invalid('analystIds must contain UUIDs');
    return parseUUID(value, 'analyst id');
  });

  const result = await withTransaction(async (client) => {
    const inserted = await client.query<{ id: string }>(
      `INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
       VALUES ($1, $2, $3, $4::numeric, $5, $6) RETURNING id`,
      [packageCode, title, priceUgx, totalOdds, tipCount, ctx.user!.id],
    );
    const id = inserted.rows[0].id;
    for (const analystID of analystIDs) {
      await client.query(
        `INSERT INTO slip_analysts (slip_id, analyst_id) VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
        [id, analystID],
      );
    }
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, after)
       VALUES ('admin', $1, 'slip.created', 'slip', $2, $3::jsonb)`,
      [ctx.user!.id, id, auditJSON({ package: packageCode, price_ugx: priceUgx, title })],
    );
    return { id, status: 'draft' };
  });
  return statusResponse(201, result);
};

interface ValidatedTip {
  analystID: string;
  matchID: string | null;
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  marketCode: string | null;
  selectionValue: string | null;
  odds: string;
  kickoffAt: string;
  note: string | null;
  position: number;
}

function validateTip(body: Record<string, unknown>): ValidatedTip {
  const analystID = optionalUUID(body, 'analystId', 'analyst id');
  if (!analystID) throw AppError.invalid('analystId is required');
  const matchID = optionalUUID(body, 'matchId', 'match id');
  const fixtureLabel = requiredString(body, 'fixtureLabel');
  const marketLabel = requiredString(body, 'marketLabel');
  const selectionLabel = requiredString(body, 'selectionLabel');
  if (fixtureLabel === '' || marketLabel === '' || selectionLabel === '') {
    throw AppError.invalid('fixtureLabel, marketLabel and selectionLabel are required');
  }
  const rawMarketCode = optionalString(body, 'marketType');
  let marketCode: string | null = null;
  if (rawMarketCode !== null) {
    marketCode = parseMarketCode(rawMarketCode);
    if (!marketCode) throw AppError.invalid(`unknown market code ${JSON.stringify(rawMarketCode)}`);
  }
  const selectionValue = optionalString(body, 'selectionValue');
  const structured = [matchID, marketCode, selectionValue].filter((value) => value !== null).length;
  if (structured !== 0 && structured !== 3) {
    throw AppError.invalid('a tip needs matchId, marketType and selectionValue together, or none of them');
  }
  if (marketCode && selectionValue && !validSelection(marketCode, selectionValue)) {
    throw AppError.invalid(`${JSON.stringify(selectionValue)} is not a selection ${marketCode} can settle to`);
  }
  return {
    analystID,
    matchID,
    fixtureLabel,
    marketLabel,
    selectionLabel,
    marketCode,
    selectionValue,
    odds: decimalString(body, 'odds', 1 + Number.EPSILON),
    kickoffAt: requiredDateTime(body, 'kickoffAt'),
    note: optionalString(body, 'note'),
    position: requiredInt(body, 'position'),
  };
}

function validSelection(market: string, value: string): boolean {
  const allowed: Record<string, string[]> = {
    ONE_X_TWO: ['HOME', 'DRAW', 'AWAY'],
    DOUBLE_CHANCE: ['1X', '12', 'X2'],
    BTTS: ['YES', 'NO'],
    OVER_UNDER_1_5: ['OVER', 'UNDER'],
    OVER_UNDER_2_5: ['OVER', 'UNDER'],
    OVER_UNDER_3_5: ['OVER', 'UNDER'],
  };
  return allowed[market]?.includes(value) ?? false;
}

async function insertTip(
  client: import('pg').PoolClient,
  slipID: string,
  tip: ValidatedTip,
): Promise<string> {
  const status = await client.query<{ status: string }>('SELECT status FROM slips WHERE id = $1', [slipID]);
  if (status.rowCount === 0) throw AppError.notFound();
  if (status.rows[0].status !== 'draft') throw AppError.conflict(`slip ${slipID} is not a draft`);
  const inserted = await client.query<{ id: string }>(
    `INSERT INTO tips (slip_id, analyst_id, match_id, fixture_label, market_label,
                       selection_label, market_code, selection_value, odds, kickoff_at, note, position)
     VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::numeric, $10, $11, $12)
     RETURNING id`,
    [slipID, tip.analystID, tip.matchID, tip.fixtureLabel, tip.marketLabel, tip.selectionLabel,
      tip.marketCode, tip.selectionValue, tip.odds, tip.kickoffAt, tip.note, tip.position],
  );
  return inserted.rows[0].id;
}

export const handleAdminAddTip: ApiHandler = async (ctx) => {
  const slipID = parseUUID(ctx.params.id ?? '', 'slip id');
  const body = await decodeObject(ctx.request, [
    'analystId', 'matchId', 'fixtureLabel', 'marketLabel', 'selectionLabel', 'marketType',
    'selectionValue', 'odds', 'kickoffAt', 'note', 'position',
  ]);
  const tip = validateTip(body);
  if (tip.position <= 0) throw AppError.invalid('position must be positive');
  const id = await withTransaction(async (client) => {
    const tipID = await insertTip(client, slipID, tip);
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, after)
       VALUES ('admin', $1, 'tip.added', 'tip', $2, $3::jsonb)`,
      [ctx.user!.id, tipID, auditJSON({ position: tip.position, slip_id: slipID })],
    );
    return tipID;
  });
  return statusResponse(201, { id });
};

export const handleAdminBulkAddTips: ApiHandler = async (ctx) => {
  const slipID = parseUUID(ctx.params.id ?? '', 'slip id');
  const body = await decodeObject(ctx.request, ['analystId', 'tips']);
  const analystID = optionalUUID(body, 'analystId', 'analyst id');
  if (!analystID) throw AppError.invalid('analystId is required');
  if (!Array.isArray(body.tips) || body.tips.length === 0) {
    throw AppError.invalid('at least one tip is required');
  }
  const tips = body.tips.map((value, index) => {
    if (typeof value !== 'object' || value === null || Array.isArray(value)) {
      throw AppError.invalid(`row ${index + 1} must be an object`);
    }
    const item = value as Record<string, unknown>;
    for (const key of Object.keys(item)) {
      if (!['fixtureLabel', 'marketLabel', 'selectionLabel', 'odds', 'kickoffAt', 'note'].includes(key)) {
        throw AppError.invalid(`unknown field ${JSON.stringify(key)}`);
      }
    }
    const fixtureLabel = requiredString(item, 'fixtureLabel');
    const marketLabel = requiredString(item, 'marketLabel');
    const selectionLabel = requiredString(item, 'selectionLabel');
    if (fixtureLabel === '' || marketLabel === '' || selectionLabel === '') {
      throw AppError.invalid(`row ${index + 1} is missing a fixture, market, or selection`);
    }
    if (typeof item.kickoffAt !== 'string' || item.kickoffAt === '') {
      throw AppError.invalid(`row ${index + 1} is missing a kickoff time`);
    }
    let kickoffAt: string;
    try {
      kickoffAt = requiredDateTime(item, 'kickoffAt');
    } catch {
      throw AppError.invalid(`row ${index + 1} has an invalid kickoff time`);
    }
    let odds: string;
    try {
      odds = decimalString(item, 'odds', 1 + Number.EPSILON);
    } catch {
      throw AppError.invalid(`row ${index + 1}'s odds must be a decimal string greater than 1`);
    }
    return {
      analystID,
      matchID: null,
      fixtureLabel,
      marketLabel,
      selectionLabel,
      marketCode: null,
      selectionValue: null,
      odds,
      kickoffAt,
      note: optionalString(item, 'note'),
      position: index + 1,
    } satisfies ValidatedTip;
  });

  const ids = await withTransaction(async (client) => {
    const count = await client.query<{ count: string }>('SELECT count(*)::text AS count FROM tips WHERE slip_id = $1', [slipID]);
    const base = Number(count.rows[0].count);
    const insertedIDs: string[] = [];
    for (let index = 0; index < tips.length; index++) {
      insertedIDs.push(await insertTip(client, slipID, { ...tips[index], position: base + index + 1 }));
    }
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, after)
       VALUES ('admin', $1, 'tips.bulk_added', 'slip', $2, $3::jsonb)`,
      [ctx.user!.id, slipID, auditJSON({ count: insertedIDs.length })],
    );
    return insertedIDs;
  });
  return statusResponse(201, { ids, count: ids.length });
};

export const handleAdminPublishSlip: ApiHandler = async (ctx) => {
  const slipID = parseUUID(ctx.params.id ?? '', 'slip id');
  await withTransaction(async (client) => {
    const slip = await client.query<{ status: string; tip_count: number; actual_tips: string }>(
      `SELECT s.status, s.tip_count, (SELECT count(*)::text FROM tips t WHERE t.slip_id = s.id) AS actual_tips
       FROM slips s WHERE s.id = $1`,
      [slipID],
    );
    if (slip.rowCount === 0) throw AppError.notFound();
    const row = slip.rows[0];
    const actualTips = Number(row.actual_tips);
    if (row.status !== 'draft') throw AppError.conflict(`slip ${slipID} is already published`);
    if (actualTips === 0) throw AppError.conflict(`slip ${slipID} has no tips`);
    if (actualTips !== row.tip_count) {
      throw AppError.conflict(`slip ${slipID} claims ${row.tip_count} tips but carries ${actualTips}`);
    }
    const started = await client.query<{ count: string }>(
      'SELECT count(*)::text AS count FROM tips WHERE slip_id = $1 AND kickoff_at <= now()',
      [slipID],
    );
    const startedCount = Number(started.rows[0].count);
    if (startedCount > 0) {
      throw AppError.conflict(`slip ${slipID} contains ${startedCount} selection(s) whose match has already kicked off`);
    }
    await client.query("UPDATE slips SET status = 'open', published_at = now() WHERE id = $1 AND status = 'draft'", [slipID]);
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id)
       VALUES ('admin', $1, 'slip.published', 'slip', $2)`,
      [ctx.user!.id, slipID],
    );
  });
  return statusResponse(200, { id: slipID, status: 'open' });
};

export const handleAdminDeleteSlip: ApiHandler = async (ctx) => {
  const slipID = parseUUID(ctx.params.id ?? '', 'slip id');
  await withTransaction(async (client) => {
    const deleted = await client.query('DELETE FROM slips WHERE id = $1 AND status = ' + "'draft'" + ' RETURNING id', [slipID]);
    if (deleted.rowCount === 0) throw AppError.conflict(`slip ${slipID} is not a deletable draft`);
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id)
       VALUES ('admin', $1, 'slip.deleted', 'slip', $2)`,
      [ctx.user!.id, slipID],
    );
  });
  return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } });
};

export const handleAdminSettleTip: ApiHandler = async (ctx) => {
  const tipID = parseUUID(ctx.params.id ?? '', 'tip id');
  const body = await decodeObject(ctx.request, ['wasCorrect', 'actualOutcome', 'reason']);
  const wasCorrect = body.wasCorrect === undefined || body.wasCorrect === null ? false : body.wasCorrect;
  if (typeof wasCorrect !== 'boolean') throw AppError.invalid('wasCorrect must be a boolean');
  const actualOutcome = requiredString(body, 'actualOutcome');
  const reason = requiredString(body, 'reason');
  if (actualOutcome === '') throw AppError.invalid('actualOutcome is required');
  if (reason === '') throw AppError.invalid('reason is required');

  await withTransaction(async (client) => {
    const tip = await client.query<{ slip_id: string; kickoff_past: boolean; settled: boolean }>(
      `SELECT t.slip_id, t.kickoff_at <= now() AS kickoff_past,
              EXISTS (SELECT 1 FROM tip_results r WHERE r.tip_id = t.id) AS settled
       FROM tips t WHERE t.id = $1`,
      [tipID],
    );
    if (tip.rowCount === 0) throw AppError.notFound();
    const row = tip.rows[0];
    if (row.settled) throw AppError.conflict(`tip ${tipID} is already settled`);
    if (!row.kickoff_past) throw AppError.conflict(`tip ${tipID} has not kicked off yet`);
    await client.query(
      `INSERT INTO tip_results (tip_id, was_correct, actual_outcome, settled_by, settled_by_user)
       VALUES ($1, $2, $3, 'admin', $4)
       ON CONFLICT (tip_id) DO NOTHING`,
      [tipID, wasCorrect, actualOutcome, ctx.user!.id],
    );
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, reason, after)
       VALUES ('admin', $1, 'tip.settled_by_admin', 'tip', $2, $3, $4::jsonb)`,
      [ctx.user!.id, tipID, reason, auditJSON({ actual_outcome: actualOutcome, slip_id: row.slip_id, was_correct: wasCorrect })],
    );
  });
  return statusResponse(200, { id: tipID, settledBy: 'admin' });
};

export const handleAdminCorrectMatch: ApiHandler = async (ctx) => {
  const matchID = parseUUID(ctx.params.id ?? '', 'match id');
  const body = await decodeObject(ctx.request, ['homeScore', 'awayScore', 'reason']);
  const homeScore = requiredInt(body, 'homeScore');
  const awayScore = requiredInt(body, 'awayScore');
  const reason = requiredString(body, 'reason');
  if (reason === '') throw AppError.invalid('reason is required');
  if (homeScore < 0 || awayScore < 0) throw AppError.invalid('scores cannot be negative');

  const flagged = await withTransaction(async (client) => {
    const match = await client.query<{ status: string; home_score: number | null; away_score: number | null }>(
      'SELECT status, home_score, away_score FROM matches WHERE id = $1', [matchID],
    );
    if (match.rowCount === 0) throw AppError.notFound();
    const before = match.rows[0];
    if (before.status !== 'finished') throw AppError.conflict(`match ${matchID} is not finished`);
    await client.query('UPDATE matches SET home_score = $2, away_score = $3 WHERE id = $1', [matchID, homeScore, awayScore]);
    const reviews = await client.query(
      `INSERT INTO correction_reviews (match_id, prediction_id, reason)
       SELECT $1, p.id, $2 FROM predictions p WHERE p.match_id = $1`,
      [matchID, reason],
    );
    await client.query(
      `INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, reason, before, after)
       VALUES ('admin', $1, 'match.score_corrected', 'match', $2, $3, $4::jsonb, $5::jsonb)`,
      [ctx.user!.id, matchID, reason,
        auditJSON({ away_score: before.away_score, home_score: before.home_score }),
        auditJSON({ away_score: awayScore, home_score: homeScore })],
    );
    return reviews.rowCount ?? 0;
  });
  return statusResponse(200, {
    id: matchID,
    note: 'Existing results are unchanged. Flagged predictions need a published correction.',
    predictionsFlagged: flagged,
  });
};

export const handleAdminIngestStatus: ApiHandler = async () => {
  const statuses = await query<{
    provider: string;
    requests_used: number;
    throttled_at: string | null;
    last_fetched_at: string | null;
    leagues: number;
  }>(
    `SELECT p.provider, COALESCE(b.requests_used, 0) AS requests_used, b.throttled_at,
            (SELECT max(fetched_at) FROM provider_payloads pp WHERE pp.provider = p.provider) AS last_fetched_at,
            (SELECT count(*)::int FROM competition_sources cs
              WHERE cs.provider = p.provider AND cs.is_primary) AS leagues
     FROM (VALUES ('football-data'), ('api-football')) AS p(provider)
     LEFT JOIN provider_budget b
       ON b.provider = p.provider AND b.day = (now() AT TIME ZONE 'UTC')::date`,
  );
  const ungraded = await queryOne<{ count: number }>(
    `SELECT count(*)::int AS count FROM predictions p
     JOIN matches m ON m.id = p.match_id
     LEFT JOIN prediction_results r ON r.prediction_id = p.id
     LEFT JOIN prediction_voids v ON v.prediction_id = p.id
     WHERE r.prediction_id IS NULL AND v.prediction_id IS NULL
       AND m.kickoff_at < now() - interval '6 hours'`,
  );
  const reviews = await queryOne<{ count: number }>(
    'SELECT count(*)::int AS count FROM correction_reviews WHERE resolved_at IS NULL',
  );
  const providers = statuses.map((row) => {
    const provider: Record<string, unknown> = {
      provider: row.provider,
      requestsUsed: row.requests_used,
      dailyLimit: row.provider === 'api-football' ? 100 : 0,
    };
    if (row.throttled_at !== null) provider.throttledAt = formatTimestamptz(row.throttled_at);
    if (row.last_fetched_at !== null) provider.lastFetchedAt = formatTimestamptz(row.last_fetched_at);
    provider.leagues = row.leagues;
    provider.ungradedPastKickoff = ungraded?.count ?? 0;
    return provider;
  });
  return statusResponse(200, {
    openCorrectionReviews: reviews?.count ?? 0,
    providers,
  });
};

export const handleAdminRevenue: ApiHandler = async (ctx) => {
  const window = parseAdminWindow(ctx.url, 30);
  return statusResponse(200, await adminRevenue(window.from, window.to));
};

export const handleAdminPaymentLedger: ApiHandler = async (ctx) => {
  const window = parseAdminWindow(ctx.url, 7);
  const rawLimit = ctx.url.searchParams.get('limit');
  let limit = 200;
  if (rawLimit !== null) {
    if (!/^[+-]?\d+$/.test(rawLimit) || !Number.isSafeInteger(Number(rawLimit)) || Number(rawLimit) <= 0) {
      throw AppError.invalid('limit must be a positive integer');
    }
    limit = Number(rawLimit);
  }
  const rows = await adminPaymentLedger(window.from, window.to, limit);
  return statusResponse(200, { from: window.from, to: window.to, rows });
};

export const handleAdminPaymentTrace: ApiHandler = async (ctx) => {
  const raw = ctx.params.traceCode ?? '';
  const code = `KTF-${raw.trim().toUpperCase().replace(/^KTF-?/, '')}`;
  const trace = await queryOne<Record<string, unknown>>(
    `SELECT pt.trace_code AS "traceCode", pt.reference, pt.provider_uuid AS "providerUuid",
            pt.provider_txn_id AS "providerTxnId", pt.status, pt.amount_ugx AS "amountUgx",
            pt.phone_number AS "phoneNumber", pt.mobile_provider AS "mobileProvider",
            pt.created_at AS "createdAt", pt.settled_at AS "settledAt",
            p.id AS "purchaseId", p.status AS "purchaseStatus", p.paid_at AS "paidAt",
            s.id AS "slipId", s.title AS "slipTitle", s.package_code AS "packageCode",
            u.id AS "userId", u.email AS "userEmail", u.name AS "userName"
     FROM payment_transactions pt
     JOIN purchases p ON p.id = pt.purchase_id
     JOIN slips s ON s.id = p.slip_id
     JOIN users u ON u.id = p.user_id
     WHERE pt.trace_code = $1`,
    [code],
  );
  if (!trace) throw AppError.notFound();
  const out: Record<string, unknown> = {
    traceCode: trace.traceCode,
    reference: trace.reference,
  };
  for (const [source, target] of [
    ['providerUuid', 'providerUuid'], ['providerTxnId', 'providerTxnId'],
  ]) if (trace[source] !== null) out[target] = trace[source];
  Object.assign(out, {
    status: trace.status,
    amountUgx: Number(trace.amountUgx),
    phoneNumber: trace.phoneNumber,
  });
  if (trace.mobileProvider !== null) out.mobileProvider = trace.mobileProvider;
  out.createdAt = formatTimestamptz(String(trace.createdAt));
  if (trace.settledAt !== null) out.settledAt = formatTimestamptz(String(trace.settledAt));
  out.purchaseId = trace.purchaseId;
  out.purchaseStatus = trace.purchaseStatus;
  if (trace.paidAt !== null) out.paidAt = formatTimestamptz(String(trace.paidAt));
  Object.assign(out, {
    slipId: trace.slipId,
    slipTitle: trace.slipTitle,
    packageCode: trace.packageCode,
    userId: trace.userId,
    userEmail: trace.userEmail,
    userName: trace.userName,
  });
  return statusResponse(200, out);
};

interface AdminWindow { from: string; to: string }

function parseAdminWindow(url: URL, defaultDays: number): AdminWindow {
  const today = new Date();
  const toDate = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() + 1));
  const defaultFrom = new Date(toDate);
  defaultFrom.setUTCDate(defaultFrom.getUTCDate() - defaultDays);
  const rawFrom = url.searchParams.get('from');
  const rawTo = url.searchParams.get('to');
  const from = rawFrom === null ? defaultFrom.toISOString().slice(0, 10) : parseDateOnly(rawFrom);
  const toDay = rawTo === null ? toDate.toISOString().slice(0, 10) : parseDateOnly(rawTo);
  if (from === null) throw AppError.invalid('from must be YYYY-MM-DD');
  if (toDay === null) throw AppError.invalid('to must be YYYY-MM-DD');
  const fromTimestamp = `${from}T00:00:00.000Z`;
  const toTimestamp = new Date(`${toDay}T00:00:00.000Z`);
  toTimestamp.setUTCDate(toTimestamp.getUTCDate() + 1);
  const to = toTimestamp.toISOString();
  if (to <= fromTimestamp) throw AppError.invalid('to is not after from');
  return { from: fromTimestamp, to };
}

async function adminRevenue(from: string, to: string): Promise<Record<string, unknown>> {
  const totals = await queryOne<{
    gross: string; refunded: string; pending: string; paid_count: number;
    refunded_count: number; pending_count: number; failed_count: number;
  }>(
    `SELECT COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'paid'), 0)::text AS gross,
            COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'refunded'), 0)::text AS refunded,
            COALESCE(sum(p.price_ugx) FILTER (WHERE p.status = 'pending'), 0)::text AS pending,
            count(*) FILTER (WHERE p.status = 'paid')::int AS paid_count,
            count(*) FILTER (WHERE p.status = 'refunded')::int AS refunded_count,
            count(*) FILTER (WHERE p.status = 'pending')::int AS pending_count,
            count(*) FILTER (WHERE p.status = 'failed')::int AS failed_count
     FROM purchases p WHERE COALESCE(p.paid_at, p.created_at) >= $1 AND COALESCE(p.paid_at, p.created_at) < $2`,
    [from, to],
  );
  const [byPackage, byAnalyst, bySlip, byDay, byProvider] = await Promise.all([
    revenueBuckets(`SELECT s.package_code AS key, pk.name AS label, count(*)::int AS purchases,
                           COALESCE(sum(p.price_ugx), 0)::text AS gross
                    FROM purchases p JOIN slips s ON s.id = p.slip_id
                    JOIN packages pk ON pk.code = s.package_code
                    WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
                    GROUP BY s.package_code, pk.name, pk.sort_order ORDER BY pk.sort_order`, from, to),
    revenueBuckets(`SELECT a.id::text AS key, a.name AS label, count(DISTINCT p.id)::int AS purchases,
                           COALESCE(sum(p.price_ugx), 0)::text AS gross
                    FROM purchases p JOIN slips s ON s.id = p.slip_id
                    JOIN slip_analysts sa ON sa.slip_id = s.id JOIN analysts a ON a.id = sa.analyst_id
                    WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
                    GROUP BY a.id, a.name ORDER BY sum(p.price_ugx) DESC`, from, to),
    revenueBuckets(`SELECT s.id::text AS key, s.title AS label, count(*)::int AS purchases,
                           COALESCE(sum(p.price_ugx), 0)::text AS gross
                    FROM purchases p JOIN slips s ON s.id = p.slip_id
                    WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
                    GROUP BY s.id, s.title ORDER BY sum(p.price_ugx) DESC LIMIT 50`, from, to),
    revenueBuckets(`SELECT to_char(date_trunc('day', p.paid_at), 'YYYY-MM-DD') AS key,
                           to_char(date_trunc('day', p.paid_at), 'YYYY-MM-DD') AS label,
                           count(*)::int AS purchases, COALESCE(sum(p.price_ugx), 0)::text AS gross
                    FROM purchases p WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
                    GROUP BY 1 ORDER BY 1`, from, to),
    revenueBuckets(`SELECT COALESCE(pt.mobile_provider, 'unknown') AS key,
                           COALESCE(pt.mobile_provider, 'unknown') AS label,
                           count(DISTINCT p.id)::int AS purchases, COALESCE(sum(p.price_ugx), 0)::text AS gross
                    FROM purchases p JOIN payment_transactions pt ON pt.purchase_id = p.id AND pt.status = 'completed'
                    WHERE p.status = 'paid' AND p.paid_at >= $1 AND p.paid_at < $2
                    GROUP BY 1 ORDER BY sum(p.price_ugx) DESC`, from, to),
  ]);
  const gross = BigInt(totals?.gross ?? '0');
  const refunded = BigInt(totals?.refunded ?? '0');
  return {
    from,
    to,
    grossUgx: Number(gross),
    refundedUgx: Number(refunded),
    netUgx: Number(gross - refunded),
    paidPurchases: totals?.paid_count ?? 0,
    refundedPurchases: totals?.refunded_count ?? 0,
    pendingPurchases: totals?.pending_count ?? 0,
    pendingUgx: Number(totals?.pending ?? '0'),
    failedPurchases: totals?.failed_count ?? 0,
    byPackage,
    byAnalyst,
    bySlip,
    byDay,
    byMobileProvider: byProvider,
  };
}

async function revenueBuckets(sql: string, from: string, to: string): Promise<Array<Record<string, unknown>>> {
  const rows = await query<{ key: string; label: string; purchases: number; gross: string }>(sql, [from, to]);
  return rows.map((row) => ({ key: row.key, label: row.label, purchases: row.purchases, grossUgx: Number(row.gross) }));
}

async function adminPaymentLedger(from: string, to: string, requestedLimit: number): Promise<Array<Record<string, unknown>>> {
  const limit = requestedLimit <= 0 || requestedLimit > 1000 ? 200 : requestedLimit;
  const rows = await query<{
    trace_code: string; provider_txn_id: string | null; status: string; amount_ugx: string;
    phone_number: string; mobile_provider: string | null; package_code: string;
    title: string; email: string; created_at: string; settled_at: string | null;
  }>(
    `SELECT pt.trace_code, pt.provider_txn_id, pt.status, pt.amount_ugx::text AS amount_ugx,
            pt.phone_number, pt.mobile_provider, s.package_code, s.title, u.email,
            pt.created_at, pt.settled_at
     FROM payment_transactions pt JOIN purchases p ON p.id = pt.purchase_id
     JOIN slips s ON s.id = p.slip_id JOIN users u ON u.id = p.user_id
     WHERE pt.created_at >= $1 AND pt.created_at < $2
     ORDER BY pt.created_at DESC LIMIT $3`,
    [from, to, limit],
  );
  return rows.map((row) => {
    const result: Record<string, unknown> = {
      traceCode: row.trace_code,
      status: row.status,
      amountUgx: Number(row.amount_ugx),
      phoneNumber: row.phone_number,
    };
    if (row.provider_txn_id !== null) result.providerTxnId = row.provider_txn_id;
    if (row.mobile_provider !== null) result.mobileProvider = row.mobile_provider;
    Object.assign(result, {
      packageCode: row.package_code,
      slipTitle: row.title,
      userEmail: row.email,
      createdAt: formatTimestamptz(row.created_at),
    });
    if (row.settled_at !== null) result.settledAt = formatTimestamptz(row.settled_at);
    return result;
  });
}