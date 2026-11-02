import { query } from '@/server/db';
import type { League, Team } from '@/server/domain/readmodel';
import type { MarketType } from '@/server/domain/market';
import { CACHE, jsonResponse } from '@/server/http/render';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * Reference data: leagues, teams and market types. These rows change rarely,
 * so the responses are cached for an hour and the frontend fetches them once
 * per session.
 *
 * SQL is ported verbatim from backend/internal/postgres/reference.go.
 */

interface LeagueRow {
  id: string;
  name: string;
  short_name: string;
  country: string;
  country_code: string;
  tier: number;
  region: string;
  slug: string;
  is_active: boolean;
}

export async function leagues(): Promise<League[]> {
  const rows = await query<LeagueRow>(
    `
    SELECT id, name, short_name, country, country_code, tier, region, slug, is_active
    FROM leagues
    WHERE is_active AND is_published
    ORDER BY tier, country, name`,
  );
  // Field order matches the Go struct so the JSON keys serialise in the same
  // order. `slug` and `is_active` are selected for the WHERE clause but are
  // `json:"-"` in Go and never appear in the response.
  return rows.map((row) => ({
    id: row.id,
    name: row.name,
    shortName: row.short_name,
    country: row.country,
    countryCode: row.country_code,
    tier: row.tier,
    region: row.region,
  }));
}

export const handleLeagues: ApiHandler = async (ctx) =>
  jsonResponse(await leagues(), CACHE.reference, ctx.request);

interface TeamRow {
  id: string;
  league_id: string;
  slug: string;
  name: string;
  short_name: string;
}

export async function teams(): Promise<Team[]> {
  const rows = await query<TeamRow>(
    `SELECT id, league_id, slug, name, short_name FROM teams ORDER BY name`,
  );
  return rows.map((row) => ({
    id: row.id,
    leagueId: row.league_id,
    name: row.name,
    shortName: row.short_name,
  }));
}

interface MarketRow {
  code: string;
  display_name: string;
  short_name: string;
  tab_label: string;
  slug: string;
  outcomes: MarketType['outcomes'];
}

export async function marketTypes(): Promise<MarketType[]> {
  const rows = await query<MarketRow>(
    `
    SELECT code, display_name, short_name, tab_label, slug, outcomes
    FROM market_types ORDER BY sort_order`,
  );
  return rows.map((row) => ({
    code: row.code as MarketType['code'],
    displayName: row.display_name,
    shortName: row.short_name,
    tabLabel: row.tab_label,
    slug: row.slug,
    // Go decodes the JSONB into a struct and re-marshals it, so the key
    // order in the response is the struct's field order — not whatever order
    // the row happens to store.
    outcomes: row.outcomes.map((outcome) => ({
      value: outcome.value,
      label: outcome.label,
      shortLabel: outcome.shortLabel,
    })),
  }));
}

export const handleMarkets: ApiHandler = async (ctx) =>
  jsonResponse(await marketTypes(), CACHE.reference, ctx.request);
