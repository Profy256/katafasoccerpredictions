/**
 * Hit/miss indicator.
 *
 * The good and critical status colours sit ~4.1 ΔE apart under deuteranopia —
 * far too close to carry meaning by themselves. Every badge therefore ships a
 * glyph AND the word, with colour as reinforcement only. Do not reduce this to
 * a coloured dot.
 */
export function OutcomeBadge({
  correct,
  size = 'md',
}: {
  correct: boolean;
  size?: 'sm' | 'md';
}) {
  const small = size === 'sm';
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-md font-medium ${
        small ? 'px-1.5 py-0.5 text-[11px]' : 'px-2 py-1 text-xs'
      } ${
        correct
          ? 'bg-good/15 text-good-text'
          : 'bg-crit/15 text-crit-text'
      }`}
    >
      <svg
        aria-hidden
        viewBox="0 0 16 16"
        className={small ? 'h-3 w-3' : 'h-3.5 w-3.5'}
        fill="none"
        stroke="currentColor"
        strokeWidth="2.25"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {correct ? <path d="M3.5 8.5 6.5 11.5 12.5 4.5" /> : <path d="M4 4l8 8M12 4l-8 8" />}
      </svg>
      {correct ? 'Hit' : 'Miss'}
    </span>
  );
}
