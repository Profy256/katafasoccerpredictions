import type { FormChar } from '@/api/types';

const STYLES: Record<FormChar, string> = {
  W: 'bg-good/20 text-good-text',
  D: 'bg-surface-hi text-fg-muted',
  L: 'bg-crit/20 text-crit-text',
};

const NAMES: Record<FormChar, string> = { W: 'Win', D: 'Draw', L: 'Loss' };

/**
 * Last few results, most recent first. The letter carries the meaning, so this
 * stays readable without colour.
 */
export function FormStrip({ form, teamName }: { form: FormChar[]; teamName: string }) {
  if (form.length === 0) {
    return <span className="text-xs text-fg-dim">No prior matches</span>;
  }
  return (
    <div
      className="flex items-center gap-1"
      role="img"
      aria-label={`${teamName} recent form, most recent first: ${form
        .map((f) => NAMES[f])
        .join(', ')}`}
    >
      {form.map((result, i) => (
        <span
          key={i}
          aria-hidden
          className={`grid h-5 w-5 place-items-center rounded text-[10px] font-semibold ${STYLES[result]}`}
        >
          {result}
        </span>
      ))}
    </div>
  );
}
