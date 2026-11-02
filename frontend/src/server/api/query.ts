import { AppError } from '@/server/domain/errors';
import { parseMarketCode, type MarketCode } from '@/server/domain/market';
import { parsePackageCode } from '@/server/domain/pro';
import { parseRegion } from '@/server/domain/readmodel';

/**
 * Query parameter parsing — port of the parse helpers in
 * backend/internal/api/handlers_public.go and cursor.go.
 *
 * Multi-value parameters are comma-separated, not repeated keys, and an
 * unknown value is a 400 rather than a silent skip: a typo'd market code
 * returning the unfiltered feed is worse than an error.
 */

const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

export function parseUUID(raw: string, label: string): string {
  if (!UUID_RE.test(raw)) {
    // Go's %q quoting — JSON.stringify produces the same form for these inputs.
    throw AppError.invalid(`${JSON.stringify(raw)} is not a valid ${label}`);
  }
  return raw.toLowerCase();
}

export function parseUUIDList(raw: string, label: string): string[] {
  if (raw === '') return [];
  const out: string[] = [];
  for (const part of raw.split(',')) {
    const trimmed = part.trim();
    if (trimmed === '') continue;
    out.push(parseUUID(trimmed, label));
  }
  return out;
}

export function parseMarketList(raw: string): MarketCode[] {
  if (raw === '') return [];
  const out: MarketCode[] = [];
  for (const part of raw.split(',')) {
    const trimmed = part.trim();
    if (trimmed === '') continue;
    const market = parseMarketCode(trimmed);
    if (!market) throw AppError.invalid(`unknown market code ${JSON.stringify(trimmed)}`);
    out.push(market);
  }
  return out;
}

/**
 * strconv.ParseFloat's behaviour for the forms an API client can send: null on
 * a parse error, NaN and ±Inf for the literals Go accepts. Anything Go would
 * reject (empty, spaces, hex floats) is rejected here too, except hex floats
 * which no client sends.
 */
function parseGoFloat(raw: string): number | null {
  if (raw === 'NaN' || raw === '+NaN' || raw === '-NaN') return Number.NaN;
  if (raw === 'Inf' || raw === '+Inf') return Number.POSITIVE_INFINITY;
  if (raw === '-Inf') return Number.NEGATIVE_INFINITY;
  if (!/^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/.test(raw)) return null;
  return Number(raw);
}

/** strconv.Atoi: optional sign, digits only, must fit a Go int. */
function parseGoInt(raw: string): number | null {
  if (!/^[+-]?\d+$/.test(raw)) return null;
  const value = Number(raw);
  return Number.isSafeInteger(value) ? value : null;
}

export interface FeedFilters {
  leagueIDs: string[];
  markets: MarketCode[];
  /** 0 means no filter. NaN is accepted the way Go's range checks accepted it. */
  minConfidence: number;
  /** "" or "all" means every region. */
  region: string;
}

export function parseFeedFilters(url: URL): FeedFilters {
  const query = url.searchParams;

  const leagueIDs = parseUUIDList(query.get('leagues') ?? '', 'league id');
  const markets = parseMarketList(query.get('markets') ?? '');

  let minConfidence = 0;
  const rawConfidence = query.get('min_confidence') ?? '';
  if (rawConfidence !== '') {
    const value = parseGoFloat(rawConfidence);
    if (value === null || value < 0 || value > 100) {
      throw AppError.invalid('min_confidence must be between 0 and 100');
    }
    minConfidence = value;
  }

  let region = '';
  const rawRegion = query.get('region') ?? '';
  if (rawRegion !== '' && rawRegion !== 'all') {
    const parsed = parseRegion(rawRegion);
    if (!parsed) throw AppError.invalid(`unknown region ${JSON.stringify(rawRegion)}`);
    region = parsed;
  }

  return { leagueIDs, markets, minConfidence, region };
}

export interface LedgerFilters {
  leagueIDs: string[];
  markets: MarketCode[];
  outcome: string;
  limit: number;
  cursor: LedgerCursor | null;
}

export function parseLedgerFilters(url: URL): LedgerFilters {
  const query = url.searchParams;

  const leagueIDs = parseUUIDList(query.get('leagues') ?? '', 'league id');
  const markets = parseMarketList(query.get('markets') ?? '');

  const outcome = query.get('outcome') ?? '';
  if (outcome !== '' && outcome !== 'all' && outcome !== 'hit' && outcome !== 'miss') {
    throw AppError.invalid('outcome must be all, hit or miss');
  }

  let limit = 0;
  const rawLimit = query.get('limit') ?? '';
  if (rawLimit !== '') {
    const parsed = parseGoInt(rawLimit);
    if (parsed === null || parsed <= 0) {
      throw AppError.invalid('limit must be a positive integer');
    }
    limit = parsed;
  }

  let cursor: LedgerCursor | null = null;
  const rawCursor = query.get('cursor') ?? '';
  if (rawCursor !== '') {
    cursor = decodeLedgerCursor(rawCursor);
  }

  return { leagueIDs, markets, outcome, limit, cursor };
}

/**
 * Cursors are opaque to clients and encode (settled_at, id), base64url.
 *
 * Cursor rather than offset pagination on the endpoints that grow without
 * bound: offset pagination over a table that gains rows daily shows duplicates
 * across pages, because rows inserted between two requests shift everything
 * down.
 */
export interface LedgerCursor {
  settledAt: string;
  id: string;
}

function decodeLedgerCursor(raw: string): LedgerCursor {
  const fail = (): never => {
    throw AppError.invalid('malformed cursor');
  };

  const decoded = Buffer.from(raw, 'base64url').toString('utf8');
  const separator = decoded.indexOf('|');
  if (separator === -1) fail();

  const at = decoded.slice(0, separator);
  const id = decoded.slice(separator + 1);
  if (
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(at) ||
    !UUID_RE.test(id)
  ) {
    fail();
  }
  return { settledAt: at, id: id.toLowerCase() };
}

export function encodeLedgerCursor(cursor: { settledAt: string; id: string } | null): string {
  if (!cursor) return '';
  return Buffer.from(`${cursor.settledAt}|${cursor.id}`, 'utf8').toString('base64url');
}

/**
 * Slip list filters — port of handleSlips' query parsing in
 * backend/internal/api/handlers_slips.go.
 */
export interface SlipFilters {
  packageCode: string;
  status: string;
  analystID: string | null;
  limit: number;
  cursor: SlipCursor | null;
}

export interface SlipCursor {
  publishedAt: string;
  id: string;
}

export function parseSlipsQuery(url: URL): SlipFilters {
  const query = url.searchParams;

  const packageCode = query.get('package') ?? '';
  if (packageCode !== '' && !parsePackageCode(packageCode)) {
    throw AppError.invalid(`unknown package code ${JSON.stringify(packageCode)}`);
  }

  const status = query.get('status') ?? '';
  if (status !== '' && status !== 'open' && status !== 'settled') {
    throw AppError.invalid('status must be open or settled');
  }

  const analystRaw = query.get('analyst') ?? '';
  const analystID = analystRaw !== '' ? parseUUID(analystRaw, 'analyst id') : null;

  let limit = 0;
  const rawLimit = query.get('limit') ?? '';
  if (rawLimit !== '') {
    const parsed = parseGoInt(rawLimit);
    if (parsed === null || parsed <= 0) {
      throw AppError.invalid('limit must be a positive integer');
    }
    limit = parsed;
  }

  let cursor: SlipCursor | null = null;
  const rawCursor = query.get('cursor') ?? '';
  if (rawCursor !== '') {
    cursor = decodeSlipCursor(rawCursor);
  }

  return { packageCode, status, analystID, limit, cursor };
}

function decodeSlipCursor(raw: string): SlipCursor {
  const fail = (): never => {
    throw AppError.invalid('malformed cursor');
  };

  const decoded = Buffer.from(raw, 'base64url').toString('utf8');
  const separator = decoded.indexOf('|');
  if (separator === -1) fail();

  const at = decoded.slice(0, separator);
  const id = decoded.slice(separator + 1);
  if (
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(at) ||
    !UUID_RE.test(id)
  ) {
    fail();
  }
  return { publishedAt: at, id: id.toLowerCase() };
}

export function encodeSlipCursor(cursor: SlipCursor | null): string {
  if (!cursor) return '';
  return Buffer.from(`${cursor.publishedAt}|${cursor.id}`, 'utf8').toString('base64url');
}

/**
 * time.Parse(time.DateOnly, raw): strict `YYYY-MM-DD`. Go rejects a wrong
 * shape and a month outside 1–12; an out-of-range day normalises (Feb 30 →
 * Mar 2) because Go only range-checks the month, mirrored here.
 *
 * Returns the canonical date string, or null on a parse error.
 */
export function parseDateOnly(raw: string): string | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (month < 1 || month > 12) return null;
  const normalized = new Date(Date.UTC(year, month - 1, day));
  if (normalized.getUTCFullYear() !== year) return null;
  return normalized.toISOString().slice(0, 10);
}
