// frontend/src/components/activity/RunningDynamicsStats.tsx
import type { Activity } from '@/api/types'

interface Props {
  activity: Activity
}

export function RunningDynamicsStats({ activity }: Props) {
  const stats = [
    activity.avg_vertical_oscillation_mm != null
      ? { label: 'Ø Vertikale Bewegung', value: `${(activity.avg_vertical_oscillation_mm / 10).toFixed(1)} cm` }
      : null,
    activity.avg_stance_time_ms != null
      ? { label: 'Ø Bodenkontaktzeit', value: `${Math.round(activity.avg_stance_time_ms)} ms` }
      : null,
    activity.avg_step_length_mm != null
      ? { label: 'Ø Schrittlänge', value: `${(activity.avg_step_length_mm / 1000).toFixed(2)} m` }
      : null,
    activity.avg_vertical_ratio_pct != null
      ? { label: 'Vertikales Verhältnis', value: `${activity.avg_vertical_ratio_pct.toFixed(1)} %` }
      : null,
    activity.avg_stance_time_balance_pct != null
      ? { label: 'GCT Balance', value: `${activity.avg_stance_time_balance_pct.toFixed(1)} / ${(100 - activity.avg_stance_time_balance_pct).toFixed(1)} %` }
      : null,
  ].filter(Boolean) as { label: string; value: string }[]

  if (stats.length === 0) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">Laufdynamik</h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
        {stats.map(s => (
          <div key={s.label} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{s.label}</div>
            <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">{s.value}</div>
          </div>
        ))}
      </div>
    </div>
  )
}
