import type { Metadata } from 'next';
import Link from 'next/link';
import { DayPredictions, DayPredictionsHeader } from '@/components/DayPredictions';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: "Today's Football Predictions",
  description:
    "Every football match priced for today with a published prediction — 1X2, Double Chance, BTTS and Over/Under picks, backed by a public accuracy record.",
  alternates: { canonical: '/predictions/today' },
};

export default function TodayPredictionsPage() {
  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <nav className="text-sm text-fg-muted" aria-label="Day">
        <span className="text-fg" aria-current="page">Today</span>
        <span aria-hidden className="mx-2 text-fg-dim">/</span>
        <Link href="/predictions/tomorrow" className="hover:text-fg">
          Tomorrow
        </Link>
      </nav>
      <div className="mt-2">
        <DayPredictionsHeader offset={0} />
      </div>
      <DayPredictions offset={0} />
    </div>
  );
}
