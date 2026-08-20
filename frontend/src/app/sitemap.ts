import type { MetadataRoute } from 'next';
import { getAnalysts, getSlips } from '@/api/client';
import { MARKETS, marketHref, DEFAULT_MARKET } from '@/lib/markets';

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/**
 * Every URL that's genuinely public and worth a crawl.
 *
 * Deliberately excludes anything gated on `/login` or draft/unpublished
 * anything — those aren't discoverable by design (a draft slip 404s rather
 * than 403s specifically so its id never leaks, and this must not be the
 * thing that leaks it). Settled slips *are* included: non-negotiable #7 is
 * that settled history stays public, and a settled slip page is exactly the
 * kind of evergreen, trust-building content worth indexing — it's a graded
 * result, not a pitch.
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const staticRoutes: MetadataRoute.Sitemap = [
    { url: SITE_URL, changeFrequency: 'hourly', priority: 1 },
    { url: `${SITE_URL}/fixtures`, changeFrequency: 'hourly', priority: 0.8 },
    { url: `${SITE_URL}/accuracy`, changeFrequency: 'daily', priority: 0.8 },
    { url: `${SITE_URL}/methodology`, changeFrequency: 'monthly', priority: 0.6 },
    { url: `${SITE_URL}/faq`, changeFrequency: 'monthly', priority: 0.7 },
    { url: `${SITE_URL}/analysts`, changeFrequency: 'daily', priority: 0.6 },
    { url: `${SITE_URL}/pro`, changeFrequency: 'daily', priority: 0.7 },
  ];

  const marketRoutes: MetadataRoute.Sitemap = Object.values(MARKETS)
    .filter((m) => m.code !== DEFAULT_MARKET)
    .map((m) => ({
      url: `${SITE_URL}${marketHref(m.code)}`,
      changeFrequency: 'hourly',
      priority: 0.7,
    }));

  const [analysts, openSlips, settledSlips] = await Promise.all([
    getAnalysts().catch(() => []),
    getSlips({ status: 'open' }).catch(() => []),
    getSlips({ status: 'settled', limit: 500 }).catch(() => []),
  ]);

  const analystRoutes: MetadataRoute.Sitemap = analysts.map((a) => ({
    url: `${SITE_URL}/analysts/${a.slug}`,
    changeFrequency: 'weekly',
    priority: 0.5,
  }));

  const slipRoutes: MetadataRoute.Sitemap = [...openSlips, ...settledSlips].map((s) => ({
    url: `${SITE_URL}/pro/slips/${s.id}`,
    changeFrequency: s.status === 'open' ? 'hourly' : 'yearly',
    priority: s.status === 'open' ? 0.6 : 0.3,
  }));

  return [...staticRoutes, ...marketRoutes, ...analystRoutes, ...slipRoutes];
}
