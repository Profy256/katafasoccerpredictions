import type { MetadataRoute } from 'next';

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

/**
 * Everything here is public by design (non-negotiable #7 — settled slips are
 * never paywalled from view, only the tips underneath an *open* one are).
 * There's nothing to Disallow: the SQL entitlement boundary already keeps a
 * crawler from ever seeing a paid tip it hasn't earned, the same way it stops
 * an unpaid browser. /login has nothing worth de-indexing either — it's a
 * plain form, not a dead end.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: '*',
      allow: '/',
    },
    sitemap: `${SITE_URL}/sitemap.xml`,
  };
}
