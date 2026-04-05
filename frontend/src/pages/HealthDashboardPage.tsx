import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  fetchHealthOverview,
  fetchHealthDailySummary,
  fetchHealthSleep,
  fetchHealthHRV,
  fetchHealthWeight,
  fetchHealthTrainingLoad,
  fetchHealthReadiness,
} from '@/api/health'
import { toLocalISO } from '@/lib/dateUtils'
import HealthOverviewCards from '@/components/health/HealthOverviewCards'
import StepsChart from '@/components/health/StepsChart'
import SleepChart from '@/components/health/SleepChart'
import HRVChart from '@/components/health/HRVChart'
import WeightChart from '@/components/health/WeightChart'
import ReadinessCard from '@/components/health/ReadinessCard'
import TrainingLoadChart from '@/components/health/TrainingLoadChart'
import BloodPressureEntryDialog from '@/components/health/BloodPressureEntryDialog'
import WeightEntryDialog from '@/components/health/WeightEntryDialog'

// ── Types ─────────────────────────────────────────────────────────────────────

type Period = '7d' | '30d' | '90d' | '1y'

const periodLabels: Record<Period, string> = {
  '7d': '7T',
  '30d': '30T',
  '90d': '90T',
  '1y': '1J',
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function getDateRange(period: Period): { from: string; to: string } {
  const to = toLocalISO(new Date())
  const from = new Date()
  if (period === '7d') from.setDate(from.getDate() - 7)
  else if (period === '30d') from.setDate(from.getDate() - 30)
  else if (period === '90d') from.setDate(from.getDate() - 90)
  else if (period === '1y') from.setFullYear(from.getFullYear() - 1)
  return { from: toLocalISO(from), to }
}

// ── Chart Section ─────────────────────────────────────────────────────────────

function ChartCard({ title, children, action }: { title: string; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="px-4 pt-4 pb-2 flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          {title}
        </h3>
        {action}
      </div>
      <div className="px-2 pb-3">{children}</div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function HealthDashboardPage() {
  const [period, setPeriod] = useState<Period>('30d')
  const [showBPDialog, setShowBPDialog] = useState(false)
  const [showWeightDialog, setShowWeightDialog] = useState(false)
  const { from, to } = getDateRange(period)

  // Overview (always latest, no date filter)
  const { data: overview = {} } = useQuery({
    queryKey: ['health-overview'],
    queryFn: fetchHealthOverview,
    retry: 1,
  })

  // Daily summary
  const { data: dailySummary = [] } = useQuery({
    queryKey: ['health-daily-summary', from, to],
    queryFn: () => fetchHealthDailySummary(from, to),
    retry: 1,
  })

  // Sleep
  const { data: sleep = [] } = useQuery({
    queryKey: ['health-sleep', from, to],
    queryFn: () => fetchHealthSleep(from, to),
    retry: 1,
  })

  // HRV
  const { data: hrv = [] } = useQuery({
    queryKey: ['health-hrv', from, to],
    queryFn: () => fetchHealthHRV(from, to),
    retry: 1,
  })

  // Weight
  const { data: weight = [] } = useQuery({
    queryKey: ['health-weight', from, to],
    queryFn: () => fetchHealthWeight(from, to),
    retry: 1,
  })

  // Training load
  const { data: trainingLoad = [] } = useQuery({
    queryKey: ['health-training-load', from, to],
    queryFn: () => fetchHealthTrainingLoad(from, to),
    retry: 1,
  })

  // Readiness
  const { data: readiness = [] } = useQuery({
    queryKey: ['health-readiness', from, to],
    queryFn: () => fetchHealthReadiness(from, to),
    retry: 1,
  })

  const latestReadiness = readiness.length > 0
    ? readiness.sort((a, b) => b.calendar_date.localeCompare(a.calendar_date))[0]
    : null

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Gesundheit</h1>
        <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
          {(Object.keys(periodLabels) as Period[]).map(p => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={
                p === period
                  ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium cursor-pointer'
                  : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors cursor-pointer'
              }
            >
              {periodLabels[p]}
            </button>
          ))}
        </div>
      </div>

      {/* Overview cards (always latest data) */}
      <HealthOverviewCards data={overview} />

      {/* Readiness card (full width, if data available) */}
      {latestReadiness != null && (
        <section>
          <ReadinessCard data={latestReadiness} />
        </section>
      )}

      {/* 2-column chart grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <ChartCard title="Schritte">
          <StepsChart data={dailySummary} />
        </ChartCard>

        <ChartCard title="Schlaf">
          <SleepChart data={sleep} />
        </ChartCard>

        <ChartCard title="HRV — Herzratenvariabilität">
          <HRVChart data={hrv} />
        </ChartCard>

        <ChartCard
          title="Gewicht"
          action={
            <button
              onClick={() => setShowWeightDialog(true)}
              className="cursor-pointer flex items-center gap-1 text-xs font-medium px-2.5 py-1 rounded-md bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 text-neutral-600 dark:text-neutral-300 transition-colors"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              Messung
            </button>
          }
        >
          <WeightChart data={weight} />
        </ChartCard>
      </div>

      {/* Blood pressure (full width) */}
      <ChartCard
        title="Blutdruck"
        action={
          <button
            onClick={() => setShowBPDialog(true)}
            className="cursor-pointer flex items-center gap-1 text-xs font-medium px-2.5 py-1 rounded-md bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 text-neutral-600 dark:text-neutral-300 transition-colors"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            Messung
          </button>
        }
      >
        <div className="px-2 text-sm text-neutral-400 dark:text-neutral-500 py-4 text-center">
          Blutdruckdaten werden im Analytics-Bereich angezeigt.
        </div>
      </ChartCard>

      {/* Training load (full width) */}
      <ChartCard title="Trainingsbelastung">
        <TrainingLoadChart data={trainingLoad} />
      </ChartCard>

      {/* Dialogs */}
      {showBPDialog && (
        <BloodPressureEntryDialog onClose={() => setShowBPDialog(false)} />
      )}
      {showWeightDialog && (
        <WeightEntryDialog onClose={() => setShowWeightDialog(false)} />
      )}
    </div>
  )
}
