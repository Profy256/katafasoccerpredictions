'use client';

import { useState } from 'react';
import { addTipAction } from '@/app/actions';
import type { Analyst, MarketType } from '@/api/types';

/**
 * A tip is either fully structured (matchId + market + selection, auto-graded
 * off the result feed) or free text an admin grades by hand later via
 * /tips/[id]/settle — never a partial mix, which the API rejects. This form
 * mirrors that: the structured fields are one block, toggled together.
 *
 * Client-side only for the market → valid-selection cascade; the write itself
 * still goes through the server action, which is all the validation that
 * matters.
 */
export function AddTipForm({
  slipId,
  analysts,
  markets,
  nextPosition,
}: {
  slipId: string;
  analysts: Analyst[];
  markets: MarketType[];
  nextPosition: number;
}) {
  const [structured, setStructured] = useState(false);
  const [marketCode, setMarketCode] = useState<string>(markets[0]?.code ?? '');
  const outcomes = markets.find((m) => m.code === marketCode)?.outcomes ?? [];

  return (
    <form action={addTipAction} className="mt-4 space-y-4 rounded-lg border border-line p-4">
      <input type="hidden" name="slipId" value={slipId} />

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label htmlFor="analystId">Analyst</label>
          <select id="analystId" name="analystId" required defaultValue="">
            <option value="" disabled>
              Select…
            </option>
            {analysts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="position">Position</label>
          <input
            id="position"
            name="position"
            type="number"
            min={1}
            step={1}
            defaultValue={nextPosition}
            required
          />
        </div>
      </div>

      <div>
        <label htmlFor="fixtureLabel">Fixture</label>
        <input id="fixtureLabel" name="fixtureLabel" type="text" required placeholder="Arsenal vs Chelsea" />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label htmlFor="marketLabel">Market label</label>
          <input id="marketLabel" name="marketLabel" type="text" required placeholder="1X2" />
        </div>
        <div>
          <label htmlFor="selectionLabel">Selection label</label>
          <input
            id="selectionLabel"
            name="selectionLabel"
            type="text"
            required
            placeholder="Arsenal to win"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label htmlFor="odds">Odds</label>
          <input id="odds" name="odds" type="text" inputMode="decimal" required placeholder="1.850" />
        </div>
        <div>
          <label htmlFor="kickoffAt">Kickoff (local time)</label>
          <input id="kickoffAt" name="kickoffAt" type="datetime-local" required />
        </div>
      </div>

      <div>
        <label htmlFor="note">Note (optional)</label>
        <input id="note" name="note" type="text" />
      </div>

      <label className="flex items-center gap-2 text-xs font-normal text-fg-muted">
        <input
          type="checkbox"
          className="w-auto"
          checked={structured}
          onChange={(e) => setStructured(e.target.checked)}
        />
        Structured — auto-grades off a tracked match and market instead of being settled by hand
      </label>

      {structured && (
        <div className="grid grid-cols-3 gap-4 border-t border-line-soft pt-4">
          <div>
            <label htmlFor="matchId">Match ID</label>
            <input id="matchId" name="matchId" type="text" placeholder="uuid" />
          </div>
          <div>
            <label htmlFor="marketType">Market</label>
            <select
              id="marketType"
              name="marketType"
              value={marketCode}
              onChange={(e) => setMarketCode(e.target.value)}
            >
              {markets.map((m) => (
                <option key={m.code} value={m.code}>
                  {m.displayName}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="selectionValue">Selection value</label>
            <select id="selectionValue" name="selectionValue" defaultValue="">
              <option value="" disabled>
                Select…
              </option>
              {outcomes.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label} ({o.value})
                </option>
              ))}
            </select>
          </div>
        </div>
      )}

      <button
        type="submit"
        className="rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90"
      >
        Add tip
      </button>
    </form>
  );
}
