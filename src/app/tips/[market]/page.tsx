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

export async function generateMetadata({
  params,
}: PageProps<'/tips/[market]'>): Promise<Metadata> {
  const { market: slug } = await params;
  const code = marketFromSlug(slug);
  if (!code) return { title: 'Market not found' };
  return {
    title: `${MARKETS[code].displayName} tips`,
    description: `Free daily ${MARKETS[code].displayName} predictions with the model's confidence and a public accuracy record.`,
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
