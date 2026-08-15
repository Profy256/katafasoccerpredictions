import { ADS_ENABLED, AD_SLOT_SIZES, type AdSlotId } from '@/lib/ads';

/**
 * A banner advertising placement.
 *
 * Renders nothing at all while `ADS_ENABLED` is false — an empty bordered box
 * saying "advertisement" is worse than no box. When ads are switched on, put
 * the network's embed inside the reserved container below; the height is fixed
 * up front so switching them on does not reflow the page around them.
 */
export function AdSlot({ id, className = '' }: { id: AdSlotId; className?: string }) {
  if (!ADS_ENABLED) return null;

  const { minHeight, label } = AD_SLOT_SIZES[id];

  return (
    <aside
      aria-label="Advertisement"
      data-ad-slot={id}
      className={`flex items-center justify-center overflow-hidden rounded-xl border border-line-soft bg-surface/60 ${className}`}
      style={{ minHeight }}
    >
      {/* Replace with the ad network embed. Keep the wrapper so the reserved
          height, the label and the aria-label stay consistent across slots. */}
      <span className="text-[10px] uppercase tracking-wider text-fg-dim">
        {label} advertisement
      </span>
    </aside>
  );
}
