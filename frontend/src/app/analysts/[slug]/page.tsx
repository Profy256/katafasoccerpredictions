import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getAnalystRecord, getOwnedSlipIds, getPackages } from '@/api/client';
import { HitRateBars } from '@/components/charts/HitRateBars';
import { SlipCard } from '@/components/SlipCard';
import {
  formatCount,
  formatDate,
  formatOdds,
  formatRate,
  formatSignedPct,
} from '@/lib/format';

export const dynamic = 'force-dynamic';

const THIN_SAMPLE = 60;

export async function generateMetadata({
  params,
}: PageProps<'/analysts/[slug]'>): Promise<Metadata> {
  const { slug } = await params;
  const record = await getAnalystRecord(slug);
  if (!record) return { title: 'Analyst not found' };
  return {
    title: `${record.analyst.name} — Football Tips Record`,
    description: `Public, auto-graded betting tips record for ${record.analyst.name} on Katafa — every slip settled and counted, hit rate and ROI included.`,
    alternates: { canonical: `/analysts/${slug}` },
  };
}

export default async function AnalystPage({ params }: PageProps<'/analysts/[slug]'>) {
  const { slug } = await params;
  const [record, packages, owned] = await Promise.all([
    getAnalystRecord(slug),
    getPackages(),
    getOwnedSlipIds(),
  ]);
  if (!record) notFound();

  const { analyst } = record;
  const packageName = (code: string) =>
    packages.find((p) => p.code === code)?.name ?? code;
  const thin = record.overall.total < THIN_SAMPLE;

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      <Link href="/analysts" className="text-sm text-fg-muted hover:text-fg">
        ← All analysts
      </Link>

      <header className="mt-4 flex flex-wrap items-start gap-4">
        <span
          aria-hidden
          className="grid h-14 w-14 shrink-0 place-items-center rounded-full bg-surface-hi text-lg font-semibold"
        >
          {analyst.initials}
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">{analyst.name}</h1>
          <p className="mt-0.5 text-sm text-fg-dim">
            {analyst.handle} · publishing since {formatDate(analyst.joinedAt)}
          </p>
          <p className="mt-3 max-w-xl text-[15px] leading-relaxed text-fg-muted">
            {analyst.bio}
          </p>
          <p className="mt-3 flex flex-wrap gap-1.5">
            {analyst.packages.map((code) => (
              <Link
                key={code}
                href={`/pro#${code}`}
                className="rounded-lg border border-line px-2 py-1 text-xs text-fg-muted hover:border-brand/40 hover:text-brand"
              >
                {packageName(code)}
              </Link>
            ))}
          </p>
        </div>
      </header>

      {/* Record */}
      <section className="mt-8 rounded-xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-fg-dim">
              Hit rate, all tips
            </p>
            <p className="mt-1 text-5xl font-semibold leading-none tabular-nums">
              {formatRate(record.overall.hitRate)}
            </p>
            <p className="mt-2 text-sm text-fg-muted">
              {formatCount(record.overall.correct)} from{' '}
              {formatCount(record.overall.total)} settled tips
            </p>
          </div>
          <dl className="grid grid-cols-3 gap-x-6 gap-y-2 text-sm">
            <div>
              <dt className="text-[10px] uppercase tracking-wider text-fg-dim">Last 30 days</dt>
              <dd className="mt-0.5 font-semibold tabular-nums">
                {record.last30Days.total > 0 ? formatRate(record.last30Days.hitRate) : '—'}
              </dd>
              <dd className="text-[11px] text-fg-dim tabular-nums">
                n={formatCount(record.last30Days.total)}
              </dd>
            </div>
            <div>
              <dt className="text-[10px] uppercase tracking-wider text-fg-dim">Avg odds</dt>
              <dd className="mt-0.5 font-semibold tabular-nums">
                {formatOdds(record.averageOdds)}
              </dd>
            </div>
            <div>
              <dt className="text-[10px] uppercase tracking-wider text-fg-dim">ROI</dt>
              <dd
                className={`mt-0.5 font-semibold tabular-nums ${
                  record.roi >= 0 ? 'text-good-text' : 'text-crit-text'
                }`}
              >
                {formatSignedPct(record.roi)}
              </dd>
              <dd className="text-[11px] text-fg-dim tabular-nums">
                {record.profitUnits >= 0 ? '+' : ''}
                {record.profitUnits.toFixed(1)}u
              </dd>
            </div>
          </dl>
        </div>

        <p className="mt-5 border-t border-line-soft pt-4 text-xs leading-relaxed text-fg-muted">
          {thin
            ? 'This record is built on a small number of settled tips. Over samples this size a hit rate moves several points on luck alone — treat it as provisional.'
            : 'ROI assumes a flat one-unit stake on every settled tip at the odds published with it. Hit rate and ROI can move in opposite directions: short-priced winners raise one and barely touch the other.'}
        </p>
      </section>

      {record.byPackage.length > 0 && (
        <section className="mt-6 rounded-xl border border-line bg-surface p-5">
          <h2 className="text-sm font-semibold">By package</h2>
          <p className="mt-1 text-xs leading-relaxed text-fg-muted">
            Hit rate on a fixed 0–100% scale. Packages differ in how selective
            they are, so these are not directly comparable to each other.
          </p>
          <div className="mt-4">
            <HitRateBars buckets={record.byPackage} />
          </div>
        </section>
      )}

      {record.recentSlips.length > 0 && (
        <section className="mt-6">
          <h2 className="text-lg font-semibold tracking-tight">Recent slips</h2>
          <p className="mt-1 text-sm text-fg-muted">
            Settled slips are published in full so the record above can be checked.
          </p>
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            {record.recentSlips.map((slip) => (
              <SlipCard
                key={slip.id}
                slip={slip}
                packageName={packageName(slip.packageCode)}
                owned={owned.has(slip.id)}
                analysts={[analyst]}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
