import type { Prediction, PredictionResult } from '@/api/types';
import { MARKETS } from '@/lib/markets';
import { settledOutcomeLabel } from '@/lib/poisson';
import { formatPct } from '@/lib/format';
import { OutcomeBadge } from './OutcomeBadge';/**
 * One market's full probability distribution.
 *
 * Uses the emphasis pattern rather than a categorical palette: the published
 * pick carries the accent hue and every other outcome recedes to gray, so the
 * chart says "this is the pick, here is what it beat" at a glance.
 *
 * Note the Double Chance rows sum to 200% by construction — its outcomes
 * overlap. They are drawn as independent bars for exactly that reason.
 */
export function MarketBreakdown({
  prediction,
  result,
}: {
  prediction: Prediction;
  result?: PredictionResult;
}) {
  const market = MARKETS[prediction.marketType];

  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{market.displayName}</h3>
        {result && <OutcomeBadge correct={result.wasCorrect} size="sm" />}
      </div>

      <div className="mt-3 space-y-2">
        {market.outcomes.map((outcome) => {
          const probability =
            prediction.distribution.find((d) => d.value === outcome.value)?.probability ?? 0;
          const isPick = outcome.value === prediction.predictionValue;

          return (
            <div key={outcome.value} className="grid grid-cols-[7.5rem_1fr_3rem] items-center gap-3">
              <span
                className={`truncate text-xs ${isPick ? 'font-semibold text-fg' : 'text-fg-muted'}`}
              >
                {outcome.label}
              </span>
              <span className="h-2 overflow-hidden rounded-full bg-line-soft">
                <span
                  className={`block h-full rounded-r-[4px] ${
                    isPick ? 'bg-brand' : 'bg-fg-dim/45'
                  }`}
                  style={{ width: `${Math.max(probability * 100, 0.5)}%` }}
                />
              </span>
              <span
                className={`text-right text-xs tabular-nums ${
                  isPick ? 'font-semibold text-fg' : 'text-fg-muted'
                }`}
              >
                {formatPct(probability * 100, 1)}
              </span>
            </div>
          );
        })}
      </div>

      <p className="mt-3 border-t border-line-soft pt-3 text-xs leading-relaxed text-fg-muted">
        Published pick:{' '}
        <span className="font-semibold text-fg">
          {market.outcomes.find((o) => o.value === prediction.predictionValue)?.label}
        </span>
        {result && (
          <>
            {' · '}Settled:{' '}
            <span className="font-semibold text-fg">
              {settledOutcomeLabel(prediction.marketType, result.actualOutcome)}
            </span>
          </>
        )}
      </p>
    </div>
  );
}
