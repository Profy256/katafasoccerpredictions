import type { MarketCode } from '@/api/types';
import { DEFAULT_MARKET } from './markets';

/**
 * Advertising seam.
 *
 * No ad network is wired up yet. Everything here exists so that turning ads on
 * later is a config change in this file plus an implementation inside
 * `AdSlot`, rather than an edit to every page.
 *
 * Two separate mechanisms live here, and they are not the same thing:
 *
 *  - **Slots** are ordinary banner placements rendered inline on a page.
 *  - **Gating** is the interstitial that must be watched *before* a market's
 *    tips are shown. This is why each market is its own route: a gate needs a
 *    navigation boundary to sit on, which client-side tab state cannot give it.
 */

/** Master switch. While false, no slot renders and no market is gated. */
export const ADS_ENABLED = false;

export type AdSlotId =
  | 'feed-top'
  | 'feed-inline'
  | 'market-top'
  | 'slip-detail';

/**
 * Reserved height per slot, used to hold layout space so enabling ads later
 * does not push content around. Values are typical IAB-ish sizes.
 */
export const AD_SLOT_SIZES: Record<AdSlotId, { minHeight: number; label: string }> = {
  'feed-top': { minHeight: 90, label: 'Leaderboard' },
  'feed-inline': { minHeight: 250, label: 'In-feed' },
  'market-top': { minHeight: 90, label: 'Leaderboard' },
  'slip-detail': { minHeight: 250, label: 'In-content' },
};

/**
 * Markets that require watching an interstitial before their tips are shown.
 *
 * The landing market is excluded on purpose: gating the page a user lands on
 * would block the free tier behind an ad before they have seen anything, which
 * is a good way to lose the visit entirely. Add codes here to gate them, e.g.
 * `['BTTS', 'OVER_UNDER_2_5']`.
 */
export const AD_GATED_MARKETS: MarketCode[] = [];

/** How long a viewed gate is remembered before the ad is shown again. */
export const AD_GATE_TTL_SECONDS = 60 * 60 * 6;

export function isMarketGated(market: MarketCode): boolean {
  if (!ADS_ENABLED) return false;
  // Never gate the landing market, whatever the config says.
  if (market === DEFAULT_MARKET) return false;
  return AD_GATED_MARKETS.includes(market);
}

/**
 * Whether this request should be interrupted by the gate.
 * `seen` comes from the viewer's acknowledged-gates cookie.
 */
export function shouldShowGate(market: MarketCode, seen: Set<string>): boolean {
  return isMarketGated(market) && !seen.has(market);
}
