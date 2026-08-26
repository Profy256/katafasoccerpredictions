import type { Metadata } from 'next';
import Link from 'next/link';
import { DayPredictions, DayPredictionsHeader } from '@/components/DayPredictions';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: "Tomorrow's Football Predictions",
  description:
    "Tomorrow's football predictions and fixtures with a published pick in every market — plan ahead with selections priced days before kickoff.",
  alternates: { canonical: '/predictions/tomorrow' },
};

export default function TomorrowPredictionsPage() {
  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <nav className="text-sm text-fg-muted" aria-label="Day">
        <Link href="/predictions/today" className="hover:text-fg">
          Today
        </Link>
        <span aria-hidden className="mx-2 text-fg-dim">/</span>
        <span className="text-fg" aria-current="page">Tomorrow</span>
      </nav>
      <div className="mt-2">
        <DayPredictionsHeader offset={1} />
      </div>
      <DayPredictions offset={1} />
    </div>
  );
}
