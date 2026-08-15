/**
 * The product's entire pitch is that its numbers are honest, so the one thing
 * this build must not do is let simulated fixtures read as real football.
 * Remove this banner at the same time the real ingestion pipeline is wired up.
 */
export function DemoDataBanner() {
  return (
    <div className="border-b border-line-soft bg-surface">
      <p className="mx-auto max-w-6xl px-4 py-2 text-xs leading-relaxed text-fg-muted sm:px-6">
        <span className="mr-1.5 font-semibold text-warn">Demo data</span>
        Fixtures, results and accuracy figures on this site are generated from a
        simulated season — no live data provider is connected yet. The model and
        grading logic are real; the football is not.
      </p>
    </div>
  );
}
