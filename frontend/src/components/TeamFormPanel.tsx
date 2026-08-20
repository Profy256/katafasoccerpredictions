import type { FormSummary, Team } from '@/api/types';
import { formatStrength } from '@/lib/format';
import { FormStrip } from './FormStrip';

/**
 * The model inputs for one side, shown as the model actually used them:
 * venue-split and expressed relative to the league average.
 */
export function TeamFormPanel({ team, form }: { team: Team; form: FormSummary }) {
  const venueLabel = form.venue === 'home' ? 'at home' : 'away';

  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold">{team.name}</p>
          <p className="mt-0.5 text-xs text-fg-dim">
            Form {venueLabel} · last {form.played} {form.played === 1 ? 'match' : 'matches'}
          </p>
        </div>
        <FormStrip form={form.recent} teamName={team.name} />
      </div>

      <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3">
        <Stat
          label="Attack strength"
          value={formatStrength(form.attackStrength)}
          hint={`Scores ${describeMultiplier(form.attackStrength)} an average side ${venueLabel}`}
        />
        <Stat
          label="Defence strength"
          value={formatStrength(form.defenseStrength)}
          hint={`Concedes ${describeMultiplier(form.defenseStrength)} an average side ${venueLabel}`}
        />
        <Stat label={`Record ${venueLabel}`} value={`${form.wins}W ${form.draws}D ${form.losses}L`} />
        <Stat label="Goals for / against" value={`${form.goalsFor} / ${form.goalsAgainst}`} />
      </dl>
    </div>
  );
}

/** Turns 1.24 into "24% more than" and 0.82 into "18% less than". */
function describeMultiplier(value: number): string {
  const delta = Math.round(Math.abs(value - 1) * 100);
  if (delta < 3) return 'about the same as';
  return `${delta}% ${value > 1 ? 'more than' : 'less than'}`;
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div>
      <dt className="text-[11px] uppercase tracking-wider text-fg-dim">{label}</dt>
      <dd className="mt-0.5 text-sm font-semibold tabular-nums">{value}</dd>
      {hint && <p className="mt-0.5 text-[11px] leading-snug text-fg-muted">{hint}</p>}
    </div>
  );
}
