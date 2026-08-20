import type { Metadata } from 'next';
import { notFound, redirect } from 'next/navigation';
import { FreeTipsView } from '@/components/FreeTipsView';
import { MARKETS, DEFAULT_MARKET, marketFromSlug } from '@/lib/markets';

export const dynamic = 'force-dynamic';

/** Pre-render the tab targets; the set is fixed and tiny. */
export function generateStaticParams() {
  return Object.values(MARKETS)
    .filter((m) => m.code !== DEFAULT_MARKET)
    .map((m) => ({ market: m.slug }));
}

/**
 * Search-friendly aliases per market, folded into the description so the
 * page matches how people actually phrase the query ("BTTS predictions",
 * "over 2.5 goals tips") without the title itself turning into a keyword
 * list. Only the six markets the model actually publishes — never a market
 * (correct score, Asian handicap) this product doesn't cover.
 */
const MARKET_ALIASES: Record<string, string> = {
  ONE_X_TWO: 'match predictions and 1X2 tips',
  DOUBLE_CHANCE: 'double chance predictions',
  BTTS: 'both teams to score (BTTS) predictions',
  OVER_UNDER_1_5: 'over/under 1.5 goals predictions',
  OVER_UNDER_2_5: 'over/under 2.5 goals predictions',
  OVER_UNDER_3_5: 'over/under 3.5 goals predictions',
};

export async function generateMetadata({
  params,
}: PageProps<'/tips/[market]'>): Promise<Metadata> {
  const { market: slug } = await params;
  const code = marketFromSlug(slug);
  if (!code) return { title: 'Market not found' };
  return {
    title: `${MARKETS[code].displayName} Predictions Today`,
    description: `Free daily ${MARKET_ALIASES[code]} for today's football matches, with the model's confidence and a public, unfiltered accuracy record.`,
    alternates: { canonical: `/tips/${MARKETS[code].slug}` },
  };
}

export default async function MarketTipsPage({ params }: PageProps<'/tips/[market]'>) {
  const { market: slug } = await params;
  const code = marketFromSlug(slug);
  if (!code) notFound();

  // The default market lives at '/', so keep a single canonical URL for it.
  if (code === DEFAULT_MARKET) redirect('/');

  return <FreeTipsView market={code} />;
}
