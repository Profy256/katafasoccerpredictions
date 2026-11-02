import { query, queryOne } from '@/server/db';
import { AppError } from '@/server/domain/errors';
import type {
  Analyst,
  APISlip,
  APITip,
  APITipResult,
  Package,
  PackageCode,
  SettledBy,
  SlipWithTips,
} from '@/server/domain/pro';
import { analysts, packages } from '@/server/api/pro';
import { encodeSlipCursor, parseSlipsQuery, type SlipCursor, type SlipFilters } from '@/server/api/query';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz, formatZeroTime } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * Slips: the list and the single slip with its picks. Port of
 * backend/internal/postgres/slips.go and handlers_slips.go.
 *
 * The paywall is a SQL boundary. An unpaid viewer's query returns zero tip
 * rows *from the database* — there is no filtering step that could be
 * forgotten and no serialisation path where tips exist in memory next to a
 * boolean. Settled slips are deliberately public: that is what makes an
 * analyst's record evidence rather than a claim.
 */

/** uuid.Nil — the viewer id an anonymous request passes to the paywall. */
const NIL_UUID = '00000000-0000-0000-0000-000000000000';

interface APISlipRow {
  id: string;
  package_code: PackageCode;
  title: string;
  price_ugx: string;
  total_odds: string;
  tip_count: number;
  status: string;
  published_at: string | null;
  won_tips: number | null;
  settled_odds: string | null;
  analyst_ids: string[];
}

// The COALESCE needs an alias: node-pg keys the row by the column name Go's
// positional Scan doesn't care about ("coalesce" here), so without it the
// analyst ids would silently read as undefined.
const SLIP_COLUMNS = `
    SELECT s.id, s.package_code, s.title, s.price_ugx, s.total_odds::text,
           s.tip_count, s.status, s.published_at, s.won_tips, s.settled_odds::text,
           COALESCE(array_agg(sa.analyst_id) FILTER (WHERE sa.analyst_id IS NOT NULL), '{}')
             AS analyst_ids
    FROM slips s
    LEFT JOIN slip_analysts sa ON sa.slip_id = s.id`;

function scanSlip(row: APISlipRow): APISlip {
  const slip: APISlip = {
    id: row.id,
    packageCode: row.package_code,
    title: row.title,
    analystIds: row.analyst_ids ?? [],
    // Go leaves the zero time when the column is null (visible only to an
    // admin looking at a draft), and the zero time marshals as this.
    publishedAt: row.published_at ? formatTimestamptz(row.published_at) : formatZeroTime(),
    priceUgx: Number(row.price_ugx),
    status: row.status,
    tipCount: row.tip_count,
    totalOdds: Number(row.total_odds),
  };

  // wonTips and settledOdds are pointer fields with omitempty in Go: null
  // means the key is absent entirely, not present as JSON null.
  if (row.won_tips !== null) slip.wonTips = row.won_tips;
  if (row.settled_odds !== null) slip.settledOdds = Number(row.settled_odds);

  // The frontend's Slip status is 'open' | 'settled'. A void slip — every
  // leg called off — presents as settled with zero winning tips, which is
  // what it is: a finished product that returned nothing, and refunded.
  if (slip.status === 'void') {
    slip.status = 'settled';
    if (slip.wonTips === undefined) slip.wonTips = 0;
  }
  return slip;
}

interface SlipListPage {
  rows: APISlipRow[];
  limit: number;
  hasMore: boolean;
}

async function querySlipList(f: SlipFilters): Promise<SlipListPage> {
  let limit = f.limit;
  if (limit <= 0 || limit > 100) limit = 30;

  const args: unknown[] = [];
  const where = [`s.status <> 'draft'`, 's.published_at IS NOT NULL'];

  if (f.packageCode !== '') {
    args.push(f.packageCode);
    where.push(`s.package_code = $${args.length}`);
  }
  if (f.status !== '') {
    args.push(f.status);
    where.push(`s.status = $${args.length}`);
  }
  if (f.analystID !== null) {
    args.push(f.analystID);
    where.push(
      `EXISTS (SELECT 1 FROM slip_analysts sa WHERE sa.slip_id = s.id AND sa.analyst_id = $${args.length})`,
    );
  }
  if (f.cursor !== null) {
    args.push(f.cursor.publishedAt, f.cursor.id);
    where.push(`(s.published_at, s.id) < ($${args.length - 1}, $${args.length})`);
  }
  args.push(limit + 1);

  const rows = await query<APISlipRow>(
    `${SLIP_COLUMNS}
     WHERE ${where.join(' AND ')}
     GROUP BY s.id
     ORDER BY s.published_at DESC, s.id DESC
     LIMIT $${args.length}`,
    args,
  );
  // The extra row proves there is another page without a second query.
  return { rows, limit, hasMore: rows.length > limit };
}

/** The item list alone — also what an analyst's recentSlips uses. */
export async function listSlips(f: SlipFilters): Promise<APISlip[]> {
  const page = await querySlipList(f);
  return page.rows.slice(0, page.limit).map(scanSlip);
}

export const handleSlips: ApiHandler = async (ctx) => {
  const filters = parseSlipsQuery(ctx.url);
  const page = await querySlipList(filters);
  const items = page.rows.slice(0, page.limit).map(scanSlip);

  const body: { items: APISlip[]; nextCursor?: string } = { items };
  // The cursor points at the last row *returned*, not the lookahead row.
  if (page.hasMore && items.length > 0) {
    const last = items[items.length - 1];
    body.nextCursor = encodeSlipCursor({ publishedAt: last.publishedAt, id: last.id });
  }

  return jsonResponse(
    body,
    // The response is no-store despite carrying only metadata, because
    // whether the viewer owns each slip varies per request and a shared cache
    // would serve one viewer's view to another.
    CACHE.none,
    ctx.request,
  );
};

/**
 * One slip with its tips, if and only if the viewer is entitled to them.
 *
 * viewerID is NIL_UUID for an anonymous viewer. The entitlement is folded
 * into the tips query's WHERE clause — see tipsForSlip.
 */
export async function slipDetail(
  slipID: string,
  viewerID: string,
  isAdmin: boolean,
): Promise<SlipWithTips> {
  // A draft slip is a 404 rather than a 403, so unpublished slip ids are not
  // discoverable by probing.
  const visibility = isAdmin ? 'TRUE' : `s.status <> 'draft'`;

  const row = await queryOne<APISlipRow>(
    `${SLIP_COLUMNS}
     WHERE s.id = $1 AND ${visibility}
     GROUP BY s.id`,
    [slipID],
  );
  if (!row) throw AppError.notFound();
  const slip = scanSlip(row);

  const [tips, results, allAnalysts, allPackages] = await Promise.all([
    tipsForSlip(slipID, viewerID, isAdmin),
    tipResultsForSlip(slipID),
    analysts(),
    packages(),
  ]);

  const byID = new Map(allAnalysts.map((a) => [a.id, a]));
  const slipAnalysts: Analyst[] = [];
  for (const id of slip.analystIds) {
    const a = byID.get(id);
    if (a) slipAnalysts.push(a);
  }

  let pkg: Package = {
    code: slip.packageCode,
    name: '',
    tagline: '',
    description: '',
    typicalPriceUgx: 0,
    highlights: [],
  };
  for (const p of allPackages) {
    if (p.code === slip.packageCode) pkg = p;
  }

  const out: SlipWithTips = {
    ...slip,
    tips,
    analysts: slipAnalysts,
    package: pkg,
    // Unlocked describes what happened, rather than deciding it: the query
    // above already returned tips or did not.
    unlocked: tips.length > 0 || slip.tipCount === 0,
  };
  if (results.length > 0) out.results = results;
  return out;
}

/**
 * The paywall.
 *
 * Settled slips are deliberately public — that is what turns an analyst's
 * record into evidence rather than a claim, and it means their losing slips
 * are visible too. Only slips still open are paywalled.
 */
async function tipsForSlip(
  slipID: string,
  viewerID: string,
  isAdmin: boolean,
): Promise<APITip[]> {
  const rows = await query<{
    id: string;
    slip_id: string;
    analyst_id: string;
    match_id: string | null;
    fixture_label: string;
    market_label: string;
    selection_label: string;
    market_code: string | null;
    selection_value: string | null;
    odds: string;
    kickoff_at: string;
    note: string | null;
  }>(
    `
    SELECT t.id, t.slip_id, t.analyst_id, t.match_id, t.fixture_label,
           t.market_label, t.selection_label, t.market_code, t.selection_value,
           t.odds::text, t.kickoff_at, t.note
    FROM tips t
    JOIN slips s ON s.id = t.slip_id
    WHERE t.slip_id = $1
      AND (
            s.status IN ('settled','void')        -- settled slips are public
         OR $3                                    -- admins author them
         OR EXISTS (SELECT 1 FROM purchases p
                    WHERE p.slip_id = s.id
                      AND p.user_id = $2
                      AND p.status  = 'paid')     -- a refund revokes access
          )
    ORDER BY t.position`,
    [slipID, viewerID, isAdmin],
  );

  return rows.map((row) => {
    const tip: APITip = {
      id: row.id,
      slipId: row.slip_id,
      analystId: row.analyst_id,
      matchId: row.match_id,
      fixtureLabel: row.fixture_label,
      marketLabel: row.market_label,
      selectionLabel: row.selection_label,
      marketType: row.market_code as APITip['marketType'],
      selectionValue: row.selection_value,
      odds: Number(row.odds),
      kickoffAt: formatTimestamptz(row.kickoff_at),
    };
    if (row.note !== null) tip.note = row.note;
    return tip;
  });
}

/**
 * Not paywalled. Results only exist for settled tips, and a settled slip is
 * public.
 */
async function tipResultsForSlip(slipID: string): Promise<APITipResult[]> {
  const rows = await query<{
    tip_id: string;
    was_correct: boolean;
    actual_outcome: string;
    settled_at: string;
    settled_by: SettledBy;
  }>(
    `
    SELECT r.tip_id, r.was_correct, r.actual_outcome, r.settled_at, r.settled_by
    FROM tip_results r
    JOIN tips t ON t.id = r.tip_id
    WHERE t.slip_id = $1
    ORDER BY t.position`,
    [slipID],
  );
  return rows.map((row) => ({
    tipId: row.tip_id,
    wasCorrect: row.was_correct,
    actualOutcome: row.actual_outcome,
    settledAt: formatTimestamptz(row.settled_at),
    settledBy: row.settled_by,
  }));
}

export const handleSlip: ApiHandler = async (ctx) => {
  const id = parseSlipUUID(ctx.params.id ?? '');
  const viewerID = ctx.user?.id ?? NIL_UUID;
  const isAdmin = ctx.user?.role === 'admin';
  return jsonResponse(await slipDetail(id, viewerID, isAdmin), CACHE.private, ctx.request);
};

function parseSlipUUID(raw: string): string {
  if (!/^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(raw)) {
    throw AppError.invalid(`${JSON.stringify(raw)} is not a valid slip id`);
  }
  return raw.toLowerCase();
}

export type { SlipCursor };
