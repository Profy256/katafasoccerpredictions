'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { AccuracyPoint } from '@/api/types';
import { formatCount, formatDate, formatRate } from '@/lib/format';

/**
 * Cumulative hit rate over time.
 *
 * Single series, so there is no legend — the heading names what is plotted, and
 * the endpoint is direct-labelled. A crosshair snaps to the nearest settlement
 * day; the same readout is reachable from the keyboard with the arrow keys, and
 * every value also exists in the table below the chart.
 */

const HEIGHT = 280;
const PAD = { top: 16, right: 60, bottom: 28, left: 44 };
const NOMINAL_WIDTH = 820;

export function AccuracyTimeline({ points }: { points: AccuracyPoint[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(NOMINAL_WIDTH);
  const [activeIndex, setActiveIndex] = useState<number | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setWidth(Math.max(320, entry.contentRect.width));
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const innerW = Math.max(1, width - PAD.left - PAD.right);
  const innerH = HEIGHT - PAD.top - PAD.bottom;

  const { yMin, yMax, ticks } = useMemo(() => {
    const values = points.map((p) => p.cumulativeHitRate);
    const lo = Math.min(...values, 1);
    const hi = Math.max(...values, 0);
    // Pad, then snap outward to 5% steps so the axis reads in round numbers.
    const min = Math.max(0, Math.floor((lo - 0.04) * 20) / 20);
    const max = Math.min(1, Math.ceil((hi + 0.04) * 20) / 20);
    const span = max - min || 0.05;
    const step = span / 4;
    return {
      yMin: min,
      yMax: max,
      ticks: Array.from({ length: 5 }, (_, i) => min + step * i),
    };
  }, [points]);

  const x = (i: number) =>
    PAD.left + (points.length <= 1 ? innerW / 2 : (i / (points.length - 1)) * innerW);
  const y = (rate: number) =>
    PAD.top + innerH - ((rate - yMin) / (yMax - yMin || 1)) * innerH;

  const linePath = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${x(i).toFixed(2)} ${y(p.cumulativeHitRate).toFixed(2)}`)
    .join(' ');

  const last = points[points.length - 1];
  const active = activeIndex !== null ? points[activeIndex] : null;

  // Roughly five x labels, always including the first and last day.
  const labelEvery = Math.max(1, Math.ceil(points.length / 5));

  const pointerToIndex = (clientX: number) => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect || points.length === 0) return null;
    const rel = clientX - rect.left - PAD.left;
    const ratio = rel / innerW;
    return Math.min(points.length - 1, Math.max(0, Math.round(ratio * (points.length - 1))));
  };

  if (points.length === 0) {
    return (
      <p className="py-10 text-center text-sm text-fg-muted">
        No graded predictions yet.
      </p>
    );
  }

  return (
    <div ref={containerRef} className="relative">
      <svg
        width={width}
        height={HEIGHT}
        role="img"
        aria-label={`Cumulative hit rate over time, ending at ${formatRate(
          last.cumulativeHitRate,
        )} across ${points.length} settlement days.`}
        tabIndex={0}
        className="touch-pan-y"
        onPointerMove={(e) => setActiveIndex(pointerToIndex(e.clientX))}
        onPointerLeave={() => setActiveIndex(null)}
        onFocus={() => setActiveIndex((i) => i ?? points.length - 1)}
        onBlur={() => setActiveIndex(null)}
        onKeyDown={(e) => {
          if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
          e.preventDefault();
          setActiveIndex((i) => {
            const current = i ?? points.length - 1;
            const next = current + (e.key === 'ArrowRight' ? 1 : -1);
            return Math.min(points.length - 1, Math.max(0, next));
          });
        }}
      >
        {/* Gridlines and y labels */}
        {ticks.map((tick) => (
          <g key={tick}>
            <line
              x1={PAD.left}
              x2={PAD.left + innerW}
              y1={y(tick)}
              y2={y(tick)}
              stroke="var(--color-line)"
              strokeWidth={1}
            />
            <text
              x={PAD.left - 8}
              y={y(tick)}
              textAnchor="end"
              dominantBaseline="middle"
              fill="var(--color-fg-dim)"
              fontSize={11}
            >
              {formatRate(tick, 0)}
            </text>
          </g>
        ))}

        {/* X labels. The last day is always labelled, and a periodic label is
            dropped if it would collide with it. */}
        {points.map((p, i) =>
          i === points.length - 1 ||
          (i % labelEvery === 0 && x(points.length - 1) - x(i) > 56) ? (
            <text
              key={p.date}
              x={x(i)}
              y={HEIGHT - 8}
              textAnchor={i === 0 ? 'start' : i === points.length - 1 ? 'end' : 'middle'}
              fill="var(--color-fg-dim)"
              fontSize={11}
            >
              {formatDate(p.date).replace(/ \d{4}$/, '')}
            </text>
          ) : null,
        )}

        {/* Line only, no area wash: the y-axis is zoomed to the data range, and
            filling to a non-zero baseline would overstate the movement. */}
        <path
          d={linePath}
          fill="none"
          stroke="var(--color-brand)"
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {/* Crosshair */}
        {active && activeIndex !== null && (
          <g>
            <line
              x1={x(activeIndex)}
              x2={x(activeIndex)}
              y1={PAD.top}
              y2={PAD.top + innerH}
              stroke="var(--color-fg-dim)"
              strokeWidth={1}
            />
            <circle
              cx={x(activeIndex)}
              cy={y(active.cumulativeHitRate)}
              r={5}
              fill="var(--color-brand)"
              stroke="var(--color-surface)"
              strokeWidth={2}
            />
          </g>
        )}

        {/* Endpoint marker and direct label */}
        <circle
          cx={x(points.length - 1)}
          cy={y(last.cumulativeHitRate)}
          r={4}
          fill="var(--color-brand)"
          stroke="var(--color-surface)"
          strokeWidth={2}
        />
        <text
          x={x(points.length - 1) + 10}
          y={y(last.cumulativeHitRate)}
          dominantBaseline="middle"
          fill="var(--color-fg)"
          fontSize={12}
          fontWeight={600}
        >
          {formatRate(last.cumulativeHitRate)}
        </text>
      </svg>

      {/* Tooltip: value leads, label follows */}
      {active && activeIndex !== null && (
        <div
          role="status"
          className="pointer-events-none absolute top-2 z-10 min-w-40 rounded-lg border border-line bg-surface-hi px-3 py-2 shadow-lg"
          style={{
            left: Math.min(Math.max(x(activeIndex) - 80, 0), Math.max(0, width - 176)),
          }}
        >
          <p className="text-xs text-fg-muted">{formatDate(active.date)}</p>
          <p className="mt-1 flex items-center gap-2">
            <span aria-hidden className="h-0.5 w-3 rounded bg-brand" />
            <span className="text-sm font-semibold tabular-nums">
              {formatRate(active.cumulativeHitRate)}
            </span>
            <span className="text-xs text-fg-muted">cumulative</span>
          </p>
          <p className="mt-0.5 text-xs text-fg-muted tabular-nums">
            {formatRate(active.dailyHitRate)} that day · {formatCount(active.settled)} settled
          </p>
        </div>
      )}
    </div>
  );
}
