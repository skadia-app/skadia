import { Suspense } from 'react'
import PMCChartExtended from '@/components/analytics/training/PMCChartExtended'
import YearProgressionChart from '@/components/analytics/training/YearProgressionChart'
import TrainingVolumeChart from '@/components/analytics/training/TrainingVolumeChart'
import ZoneDistributionChart from '@/components/analytics/training/ZoneDistributionChart'
import PowerDurationCurve from '@/components/analytics/training/PowerDurationCurve'
import PersonalRecordsTable from '@/components/analytics/training/PersonalRecordsTable'
import RunningFormTrend from '@/components/analytics/RunningFormTrend'
import ErrorBoundary from '@/components/ErrorBoundary'

function CardSkeleton({ height = 'h-[300px]' }: { height?: string }) {
  return (
    <div className={`rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 ${height} animate-pulse`} />
  )
}

export default function TrainingTab() {
  return (
    <div className="space-y-6">
      {/* PMC Extended — full width */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <ErrorBoundary>
          <Suspense fallback={<CardSkeleton height="h-[400px]" />}>
            <PMCChartExtended />
          </Suspense>
        </ErrorBoundary>
      </section>

      {/* Year progression — full width */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <ErrorBoundary>
          <Suspense fallback={<CardSkeleton height="h-[320px]" />}>
            <YearProgressionChart />
          </Suspense>
        </ErrorBoundary>
      </section>

      {/* Volume + Zones side by side */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <ErrorBoundary>
            <Suspense fallback={<CardSkeleton height="h-[340px]" />}>
              <TrainingVolumeChart />
            </Suspense>
          </ErrorBoundary>
        </section>
        <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <ErrorBoundary>
            <Suspense fallback={<CardSkeleton height="h-[340px]" />}>
              <ZoneDistributionChart />
            </Suspense>
          </ErrorBoundary>
        </section>
      </div>

      {/* Power Duration Curve — full width */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <ErrorBoundary>
          <Suspense fallback={<CardSkeleton height="h-[320px]" />}>
            <PowerDurationCurve />
          </Suspense>
        </ErrorBoundary>
      </section>

      {/* Personal records — full width */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <ErrorBoundary>
          <Suspense fallback={<CardSkeleton height="h-[240px]" />}>
            <PersonalRecordsTable />
          </Suspense>
        </ErrorBoundary>
      </section>

      {/* Running Form Trend — full width */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
        <ErrorBoundary>
          <Suspense fallback={<CardSkeleton height="h-[320px]" />}>
            <RunningFormTrend />
          </Suspense>
        </ErrorBoundary>
      </section>
    </div>
  )
}
