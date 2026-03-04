import type { HealthOverview } from '@/api/health'

interface Props {
  data: HealthOverview
}

function formatSleepDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}:${m.toString().padStart(2, '0')}`
}

function totalSleepSeconds(sleep: { deep_seconds: number | null; light_seconds: number | null; rem_seconds: number | null; awake_seconds: number | null }): number {
  return (sleep.deep_seconds ?? 0) + (sleep.light_seconds ?? 0) + (sleep.rem_seconds ?? 0)
}

function readinessColor(score: number): string {
  if (score < 40) return 'text-rose-500'
  if (score < 70) return 'text-orange-500'
  return 'text-emerald-500'
}

function readinessBg(score: number): string {
  if (score < 40) return 'bg-rose-50 dark:bg-rose-950/20 border-rose-200 dark:border-rose-800/40'
  if (score < 70) return 'bg-orange-50 dark:bg-orange-950/20 border-orange-200 dark:border-orange-800/40'
  return 'bg-emerald-50 dark:bg-emerald-950/20 border-emerald-200 dark:border-emerald-800/40'
}

function readinessLabel(level: string | null): string {
  if (!level) return ''
  const labels: Record<string, string> = {
    LOW: 'Niedrig',
    MODERATE: 'Moderat',
    HIGH: 'Hoch',
  }
  return labels[level] ?? level
}

export default function HealthOverviewCards({ data }: Props) {
  const { daily_summary, sleep, hrv, weight, readiness } = data

  // Steps
  const steps = daily_summary?.total_steps
  const stepGoal = daily_summary?.step_goal
  const stepProgress = steps != null && stepGoal != null && stepGoal > 0
    ? Math.min(100, (steps / stepGoal) * 100)
    : null

  // Sleep
  const sleepTotal = sleep ? totalSleepSeconds(sleep) : null
  const sleepScore = sleep?.score_overall

  // HRV
  const rmssd = hrv?.rmssd_ms

  // Weight
  const weightKg = weight ? weight.weight_g / 1000 : null
  const bmi = weight?.bmi

  // Readiness
  const readinessScore = readiness?.score
  const readinessLevel = readiness?.level

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
      {/* Steps */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Schritte heute</div>
        <div className="text-xl font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
          {steps != null ? steps.toLocaleString('de-DE') : '—'}
        </div>
        {stepGoal != null && (
          <div className="text-xs text-neutral-400 mt-0.5">Ziel: {stepGoal.toLocaleString('de-DE')}</div>
        )}
        {stepProgress != null && (
          <div className="mt-2 h-1.5 w-full rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${stepProgress >= 100 ? 'bg-emerald-500' : 'bg-orange-400'}`}
              style={{ width: `${stepProgress}%` }}
            />
          </div>
        )}
      </div>

      {/* Sleep */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Schlaf letzte Nacht</div>
        <div className="text-xl font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
          {sleepTotal != null ? formatSleepDuration(sleepTotal) : '—'}
        </div>
        {sleepScore != null && (
          <div className="text-xs text-neutral-400 mt-0.5">Score: {sleepScore}</div>
        )}
      </div>

      {/* HRV */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">HRV</div>
        <div className="text-xl font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
          {rmssd != null ? `${Math.round(rmssd)} ms` : '—'}
        </div>
        {hrv?.time_of_day && (
          <div className="text-xs text-neutral-400 mt-0.5">RMSSD · {hrv.time_of_day}</div>
        )}
        {rmssd == null && (
          <div className="text-xs text-neutral-400 mt-0.5">Keine Messung</div>
        )}
      </div>

      {/* Weight */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Gewicht</div>
        <div className="text-xl font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
          {weightKg != null ? `${weightKg.toFixed(1)} kg` : '—'}
        </div>
        {bmi != null && (
          <div className="text-xs text-neutral-400 mt-0.5">BMI: {bmi.toFixed(1)}</div>
        )}
      </div>

      {/* Readiness */}
      <div className={`rounded-lg border p-3 ${readinessScore != null ? readinessBg(readinessScore) : 'border-neutral-200 dark:border-neutral-800'}`}>
        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Bereitschaft</div>
        <div className={`text-xl font-semibold tabular-nums ${readinessScore != null ? readinessColor(readinessScore) : 'text-neutral-900 dark:text-neutral-100'}`}>
          {readinessScore != null ? readinessScore : '—'}
        </div>
        {readinessLevel != null && (
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {readinessLabel(readinessLevel)}
          </div>
        )}
      </div>
    </div>
  )
}
