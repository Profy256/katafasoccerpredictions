import { FreeTipsView } from '@/components/FreeTipsView';
import { DEFAULT_MARKET } from '@/lib/markets';

/**
 * The landing page shows the default market's free tips. Every other market
 * has its own route under /tips, which is what lets an ad gate sit in front of
 * it later without gating the page people arrive on.
 */
export const dynamic = 'force-dynamic';

export default function HomePage() {
  return <FreeTipsView market={DEFAULT_MARKET} />;
}
