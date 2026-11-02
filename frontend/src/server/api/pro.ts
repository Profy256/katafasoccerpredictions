import { query } from '@/server/db';
import type { Analyst, Package, PackageCode } from '@/server/domain/pro';
import { CACHE, jsonResponse } from '@/server/http/render';
import { formatTimestamptz } from '@/server/http/time';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * Pro reference data: packages and analysts. Both rows change rarely, so the
 * responses are cached for an hour — port of Packages and Analysts in
 * backend/internal/postgres/reference.go.
 */

export async function packages(): Promise<Package[]> {
  const rows = await query<{
    code: PackageCode;
    name: string;
    tagline: string;
    description: string;
    typical_price_ugx: string;
    highlights: string[];
  }>(
    `
    SELECT code, name, tagline, description, typical_price_ugx, highlights
    FROM packages ORDER BY sort_order`,
  );
  return rows.map((row) => ({
    code: row.code,
    name: row.name,
    tagline: row.tagline,
    description: row.description,
    // BIGINT arrives as a string from node-pg; UGX is int64 shillings and
    // serialises as a plain number, same as Go's int64.
    typicalPriceUgx: Number(row.typical_price_ugx),
    highlights: row.highlights ?? [],
  }));
}

export const handlePackages: ApiHandler = async (ctx) =>
  jsonResponse(await packages(), CACHE.reference, ctx.request);

/**
 * Every active analyst, with the packages they have actually published slips
 * in — derived rather than stored, so it cannot drift from the record.
 */
export async function analysts(): Promise<Analyst[]> {
  const rows = await query<{
    id: string;
    slug: string;
    name: string;
    handle: string;
    initials: string;
    bio: string;
    joined_at: string;
    packages: PackageCode[] | null;
  }>(
    `
    SELECT a.id, a.slug, a.name, a.handle, a.initials, a.bio, a.joined_at,
           COALESCE(
             array_agg(DISTINCT s.package_code) FILTER (WHERE s.id IS NOT NULL),
             '{}'
           ) AS packages
    FROM analysts a
    LEFT JOIN tips  t ON t.analyst_id = a.id
    LEFT JOIN slips s ON s.id = t.slip_id AND s.status <> 'draft'
    WHERE a.is_active
    GROUP BY a.id
    ORDER BY a.name`,
  );
  // Go only ever appends to a nil slice, so an analyst with no published
  // packages keeps `packages` nil — JSON null, not [].
  return rows.map((row) => ({
    id: row.id,
    slug: row.slug,
    name: row.name,
    handle: row.handle,
    initials: row.initials,
    bio: row.bio,
    packages: row.packages && row.packages.length > 0 ? row.packages : null,
    joinedAt: formatTimestamptz(row.joined_at),
  }));
}

export const handleAnalysts: ApiHandler = async (ctx) =>
  jsonResponse(await analysts(), CACHE.reference, ctx.request);
