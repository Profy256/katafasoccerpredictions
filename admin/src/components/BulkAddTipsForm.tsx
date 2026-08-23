'use client';

import { useMemo, useState } from 'react';
import { bulkAddTipsAction } from '@/app/actions';
import { parseTipsBlock, type ParsedTip } from '@/lib/parseTips';
import type { Analyst } from '@/api/types';

/**
 * Pastes a block of arbitrary text and turns it into an editable table of
 * tips before anything is written. parseTipsBlock's guesses are just a
 * starting point — there is no format strict enough to trust blindly here,
 * so every row stays editable and nothing submits until fixture, market,
 * selection, odds and kickoff are all filled in.
 *
 * One analyst applies to the whole batch: a pasted list is normally one
 * person's picks for the slip, not a mix. Kickoff stays per row, which is
 * what lets a single paste cover a week's worth of Akatambula fixtures
 * spread across different days.
 */
export function BulkAddTipsForm({
  slipId,
  analysts,
  nextPosition,
}: {
  slipId: string;
  analysts: Analyst[];
  nextPosition: number;
}) {
  const [text, setText] = useState('');
  const [rows, setRows] = useState<ParsedTip[]>([]);
  const [analystId, setAnalystId] = useState('');

  function onParse() {
    setRows(parseTipsBlock(text));
  }

  function updateRow(index: number, patch: Partial<ParsedTip>) {
    setRows((prev) => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  }

  function removeRow(index: number) {
    setRows((prev) => prev.filter((_, i) => i !== index));
  }

  const isComplete = (r: ParsedTip) =>
    Boolean(r.fixtureLabel && r.marketLabel && r.selectionLabel && r.odds && r.kickoffAt);

  const completeRows = useMemo(() => rows.filter(isComplete), [rows]);
  const incompleteCount = rows.length - completeRows.length;
  const tipsJson = useMemo(() => JSON.stringify(completeRows), [completeRows]);

  return (
    <div className="mt-4 space-y-4 rounded-lg border border-line p-4">
      <div>
        <label htmlFor="bulkText">Paste here</label>
        <textarea
          id="bulkText"
          rows={6}
          placeholder={
            'One match per line, however you already have it, e.g.\n' +
            'Arsenal vs Chelsea | 1X2 | Home win | 1.85 | 2026-08-22 15:00\n' +
            'Man City vs Liverpool - BTTS Yes @1.60 - 23 Aug 17:30'
          }
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        <div className="mt-2 flex items-center gap-3">
          <button
            type="button"
            onClick={onParse}
            disabled={!text.trim()}
            className="rounded-lg border border-line px-3 py-1.5 text-xs hover:border-brand disabled:cursor-not-allowed disabled:opacity-40"
          >
            Parse into rows
          </button>
          {rows.length > 0 && (
            <span className="text-xs text-fg-dim">
              {rows.length} row{rows.length === 1 ? '' : 's'} found
            </span>
          )}
        </div>
      </div>

      {rows.length > 0 && (
        <>
          <div className="overflow-x-auto rounded-lg border border-line">
            <table className="w-full min-w-[860px] text-left text-sm">
              <thead>
                <tr className="border-b border-line text-xs text-fg-muted">
                  <th className="px-3 py-2 font-medium">Fixture</th>
                  <th className="px-3 py-2 font-medium">Market</th>
                  <th className="px-3 py-2 font-medium">Selection</th>
                  <th className="px-3 py-2 font-medium">Odds</th>
                  <th className="px-3 py-2 font-medium">Kickoff</th>
                  <th className="px-3 py-2 font-medium" />
                </tr>
              </thead>
              <tbody>
                {rows.map((row, i) => (
                  <tr
                    key={i}
                    className={
                      'border-b border-line-soft last:border-0' +
                      (isComplete(row) ? '' : ' bg-warn/5')
                    }
                  >
                    <td className="px-3 py-2">
                      <input
                        type="text"
                        value={row.fixtureLabel}
                        onChange={(e) => updateRow(i, { fixtureLabel: e.target.value })}
                        className="w-full min-w-[180px]"
                      />
                    </td>
                    <td className="px-3 py-2">
                      <input
                        type="text"
                        value={row.marketLabel}
                        onChange={(e) => updateRow(i, { marketLabel: e.target.value })}
                        className="w-full min-w-[110px]"
                      />
                    </td>
                    <td className="px-3 py-2">
                      <input
                        type="text"
                        value={row.selectionLabel}
                        onChange={(e) => updateRow(i, { selectionLabel: e.target.value })}
                        className="w-full min-w-[140px]"
                      />
                    </td>
                    <td className="px-3 py-2">
                      <input
                        type="text"
                        inputMode="decimal"
                        value={row.odds}
                        onChange={(e) => updateRow(i, { odds: e.target.value })}
                        className="w-full min-w-[70px]"
                      />
                    </td>
                    <td className="px-3 py-2">
                      <input
                        type="datetime-local"
                        value={row.kickoffAt}
                        onChange={(e) => updateRow(i, { kickoffAt: e.target.value })}
                        className="w-full min-w-[170px]"
                      />
                    </td>
                    <td className="px-3 py-2">
                      <button
                        type="button"
                        onClick={() => removeRow(i)}
                        className="text-xs text-crit-text hover:underline"
                      >
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {incompleteCount > 0 && (
            <p className="text-xs text-warn">
              {incompleteCount} row{incompleteCount === 1 ? '' : 's'} still missing a field
              (highlighted) — those won&apos;t be added until filled in.
            </p>
          )}

          <div>
            <label htmlFor="bulkAnalystId">Analyst (applies to every row above)</label>
            <select
              id="bulkAnalystId"
              value={analystId}
              onChange={(e) => setAnalystId(e.target.value)}
              required
            >
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

          <form action={bulkAddTipsAction}>
            <input type="hidden" name="slipId" value={slipId} />
            <input type="hidden" name="analystId" value={analystId} />
            <input type="hidden" name="tipsJson" value={tipsJson} />
            <button
              type="submit"
              disabled={completeRows.length === 0 || !analystId}
              className="rounded-lg bg-brand px-3 py-2 text-sm font-medium text-canvas hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Add {completeRows.length || ''} tip{completeRows.length === 1 ? '' : 's'} starting at
              position {nextPosition}
            </button>
          </form>
        </>
      )}
    </div>
  );
}
