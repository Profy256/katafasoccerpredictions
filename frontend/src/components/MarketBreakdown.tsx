import type { Prediction, PredictionResult } from '@/api/types';
import { MARKETS } from '@/lib/markets';
import { settledOutcomeLabel } from '@/lib/poisson';
import { OutcomeBadge } from './OutcomeBadge';

/**
 * One market's published pick.
 *
 * Deliberately shows only the selection — never the underlying probability
 * numbers. Those live inside the model; what readers act on is the pick, and
 * the accuracy record is where the model's quality is judged.
 */
export function MarketBreakdown({
  prediction,
  result,
}: {
  prediction: Prediction;
  result?: PredictionResult;
}) {
  const market = MARKETS[prediction.marketType];
  const pick = market.outcomes.find((o) => o.value === prediction.predictionValue)?.label;

  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{market.displayName}</h3>
        {result && <OutcomeBadge correct={result.wasCorrect} size="sm" />}
      </div>

      <p className="mt-4 text-lg font-semibold tracking-tight">{pick}</p>

      <p className="mt-3 border-t border-line-soft pt-3 text-xs leading-relaxed text-fg-muted">
        Published before kickoff.
        {result && (
          <>
            {' '}Settled:{' '}
            <span className="font-semibold text-fg">
              {settledOutcomeLabel(prediction.marketType, result.actualOutcome)}
            </span>
          </>
        )}
      </p>
    </div>
  );
}
