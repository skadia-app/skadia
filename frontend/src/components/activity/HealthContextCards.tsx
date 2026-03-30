import { useQuery } from '@tanstack/react-query'
import { fetchHealthContext } from '@/api/activities'
import type { HealthContext } from '@/api/types'

interface HealthContextCardsProps {
  activityId: string
}

function formatDurationMin(totalMin: number): string {
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  if (h > 0) return `${h}h ${m}min`
  return `${m}min`
}

function formatHrvTime(ts: string): string {
  // ts is a full timestamp string — extract HH:MM
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return ts
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ts
  }
}

function scoreColor(score: number): string {
  if (score >= 80) return 'text-green-600 dark:text-green-400'
  if (score >= 60) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function scoreBg(score: number): string {
  if (score >= 80) return 'bg-green-500'
  if (score >= 60) return 'bg-amber-400'
  return 'bg-red-500'
}

interface SleepCardProps {
  hc: HealthContext
}

function SleepCard({ hc }: SleepCardProps) {
  const totalMin = hc.sleep_duration_min ?? 0
  const deepMin = hc.deep_sleep_min ?? 0
  const remMin = hc.rem_sleep_min ?? 0
  const lightMin = totalMin > 0 ? Math.max(0, totalMin - deepMin - remMin) : 0

  const deepPct = totalMin > 0 ? (deepMin / totalMin) * 100 : 0
  const remPct = totalMin > 0 ? (remMin / totalMin) * 100 : 0
  const lightPct = totalMin > 0 ? (lightMin / totalMin) * 100 : 0

  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">Schlaf</span>
        {hc.sleep_score != null && (
          <span className={`text-xs font-semibold tabular-nums ${scoreColor(hc.sleep_score)}`}>
            {hc.sleep_score}/100
          </span>
        )}
      </div>

      {hc.sleep_duration_min != null && (
        <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {formatDurationMin(hc.sleep_duration_min)}
        </div>
      )}

      {/* Sleep stage breakdown bar */}
      {totalMin > 0 && (deepMin > 0 || remMin > 0) && (
        <div className="space-y-1">
          <div className="flex h-1.5 rounded-full overflow-hidden gap-px">
            {deepPct > 0 && (
              <div
                className="bg-indigo-600 dark:bg-indigo-500"
                style={{ width: `${deepPct}%` }}
                title={`Tiefschlaf: ${deepMin}min`}
              />
            )}
            {remPct > 0 && (
              <div
                className="bg-violet-500 dark:bg-violet-400"
                style={{ width: `${remPct}%` }}
                title={`REM: ${remMin}min`}
              />
            )}
            {lightPct > 0 && (
              <div
                className="bg-sky-300 dark:bg-sky-600"
                style={{ width: `${lightPct}%` }}
                title={`Leichtschlaf: ${lightMin}min`}
              />
            )}
          </div>
          <div className="flex gap-3 text-[10px] text-neutral-500 dark:text-neutral-400">
            {deepMin > 0 && <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-indigo-600 dark:bg-indigo-500" />Tief {deepMin}min</span>}
            {remMin > 0 && <span className="flex items-center gap-1"><span className="inline-block w-2 h-2 rounded-sm bg-violet-500 dark:bg-violet-400" />REM {remMin}min</span>}
          </div>
        </div>
      )}

      {/* Score bar */}
      {hc.sleep_score != null && (
        <div className="h-1 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${scoreBg(hc.sleep_score)}`}
            style={{ width: `${hc.sleep_score}%` }}
          />
        </div>
      )}
    </div>
  )
}

interface HrvCardProps {
  hc: HealthContext
}

function HrvCard({ hc }: HrvCardProps) {
  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1">
      <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">HRV</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {Math.round(hc.hrv_rmssd!)} <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">ms</span>
      </div>
      {hc.hrv_measured_at && (
        <div className="text-[11px] text-neutral-400 dark:text-neutral-500">
          {formatHrvTime(hc.hrv_measured_at)}
        </div>
      )}
    </div>
  )
}

interface RestingHrCardProps {
  hc: HealthContext
}

function RestingHrCard({ hc }: RestingHrCardProps) {
  return (
    <div className="flex-1 min-w-0 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-1">
      <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">Ruhepuls</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
        {hc.resting_hr} <span className="text-xs font-normal text-neutral-500 dark:text-neutral-400">bpm</span>
      </div>
    </div>
  )
}

export function HealthContextCards({ activityId }: HealthContextCardsProps) {
  const { data: hc } = useQuery({
    queryKey: ['healthContext', activityId],
    queryFn: () => fetchHealthContext(activityId),
    staleTime: 5 * 60 * 1000,
  })

  if (!hc) return null

  const hasSleep = hc.sleep_duration_min != null || hc.sleep_score != null
  const hasHrv = hc.hrv_rmssd != null
  const hasRhr = hc.resting_hr != null

  if (!hasSleep && !hasHrv && !hasRhr) return null

  return (
    <div className="flex flex-wrap gap-3">
      {hasSleep && <SleepCard hc={hc} />}
      {hasHrv && <HrvCard hc={hc} />}
      {hasRhr && <RestingHrCard hc={hc} />}
    </div>
  )
}
