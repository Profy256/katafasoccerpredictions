'use client';

import Link from 'next/link';
import { useEffect, useRef } from 'react';
import type { MarketCode } from '@/api/types';
import { MARKET_CODES } from '@/api/types';
import { MARKETS, marketHref } from '@/lib/markets';
import { isMarketGated } from '@/lib/ads';

/**
 * Market selector for the free tips.
 *
 * These are links to separate routes, not client-side tab state. That is
 * deliberate: gating a market behind an ad needs a real navigation boundary to
 * hang off, and separate URLs also make each market shareable and indexable.
 */
export function MarketTabs({ active }: { active: MarketCode }) {
  const scrollerRef = useRef<HTMLElement>(null);

  // On narrow screens the strip scrolls, and the active market is often off
  // the right edge on arrival — so the page gives no clue which one you are
  // reading. Nudge it into view. Setting scrollLeft directly rather than
  // calling scrollIntoView keeps this from scrolling the whole page too.
  useEffect(() => {
    const scroller = scrollerRef.current;
    const current = scroller?.querySelector<HTMLElement>('[aria-current="page"]');
    if (!scroller || !current) return;

    const centreActiveTab = () => {
      const centred =
        current.offsetLeft - (scroller.clientWidth - current.offsetWidth) / 2;
      const max = scroller.scrollWidth - scroller.clientWidth;
      scroller.scrollLeft = Math.max(0, Math.min(centred, max));
    };

    // Re-centre whenever the strip's width actually changes, rather than
    // guessing when layout has settled: the tabs grow when the webfont swaps
    // in, and a position measured against fallback metrics would leave the
    // active tab off to one side.
    const list = scroller.firstElementChild;
    let lastWidth = 0;
    const observer = new ResizeObserver(() => {
      if (scroller.scrollWidth === lastWidth) return;
      lastWidth = scroller.scrollWidth;
      centreActiveTab();
    });
    if (list) observer.observe(list);

    // Once the reader scrolls the strip themselves, stop moving it under them.
    const stop = () => observer.disconnect();
    scroller.addEventListener('pointerdown', stop, { once: true });

    centreActiveTab();

    return () => {
      observer.disconnect();
      scroller.removeEventListener('pointerdown', stop);
    };
  }, [active]);

  return (
    <nav
      ref={scrollerRef}
      aria-label="Tip markets"
      className="-mx-4 overflow-x-auto sm:mx-0"
    >
      {/* Padding sits on the scrolling content, not the scroll container:
          a container's trailing padding is dropped at the end of a horizontal
          scroll, which clips the last tab. */}
      <ul className="flex w-max min-w-full gap-1.5 border-b border-line px-4 pb-3 sm:px-0">
        {MARKET_CODES.map((code) => {
          const market = MARKETS[code];
          const isActive = code === active;
          return (
            <li key={code}>
              <Link
                href={marketHref(code)}
                aria-current={isActive ? 'page' : undefined}
                className={`flex items-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-2 text-[13px] transition-colors ${
                  isActive
                    ? 'border-brand bg-brand/15 font-medium text-fg'
                    : 'border-line bg-surface text-fg-muted hover:border-brand/40 hover:text-fg'
                }`}
              >
                {market.tabLabel}
                {/* Tells the reader an ad stands between them and this market,
                    rather than springing it on them after the click. */}
                {isMarketGated(code) && !isActive && (
                  <span
                    title="Shows a short ad first"
                    className="rounded bg-surface-hi px-1 py-0.5 text-[9px] font-medium uppercase tracking-wider text-fg-dim"
                  >
                    Ad
                  </span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
