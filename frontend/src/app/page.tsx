import type { Metadata } from 'next';
import { FreeTipsView } from '@/components/FreeTipsView';
import { DEFAULT_MARKET } from '@/lib/markets';

/**
 * The landing page shows the default market's free tips. Every other market
 * has its own route under /tips, which is what lets an ad gate sit in front of
 * it later without gating the page people arrive on.
 */
export const dynamic = 'force-dynamic';

// Overrides the root layout's default title/description with copy specific
// to what this page actually is: today's free shortlist, not the brand name.
export const metadata: Metadata = {
  title: "Today's Free Football Predictions",
  description:
    "Free football predictions and soccer tips for today's matches — Match Result, Double Chance, Both Teams To Score and Over/Under goals — with a public accuracy record for every pick, including the ones that missed.",
  alternates: { canonical: '/' },
};

export default function HomePage() {
  return <FreeTipsView market={DEFAULT_MARKET} />;
}
