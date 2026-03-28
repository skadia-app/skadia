import type { ReactNode } from 'react'
import type { Activity } from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain, formatElevationLoss } from '@/lib/utils'

interface StatsGridProps {
  activity: Activity
}

export function StatsGrid({ activity }: StatsGridProps) {
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)
  const isDEM = activity.elevation_source === 'dem'

  const stats: { label: string; value: string | null; badge?: ReactNode }[] = [
    { label: 'Distanz', value: formatDistance(activity.distance_m) },
    { label: 'Dauer', value: formatDuration(activity.elapsed_time_s ?? 0) },
    { label: 'Bewegungszeit', value: activity.moving_time_s ? formatDuration(activity.moving_time_s) : null },
    {
      label: 'Anstieg',
      value: gain,
      badge: isDEM ? (
        <span
          className="text-xs text-emerald-600 dark:text-emerald-400 ml-1 font-normal"
          title="Höhe via DEM (Open-Meteo) korrigiert"
        >
          DEM
        </span>
      ) : undefined,
    },
    { label: 'Abstieg', value: loss },
    { label: 'Avg HR', value: activity.avg_heart_rate != null ? `${Math.round(activity.avg_heart_rate)} bpm` : null },
    { label: 'Max HR', value: activity.max_heart_rate != null ? `${activity.max_heart_rate} bpm` : null },
    { label: 'Avg Power', value: activity.avg_power_w != null ? `${Math.round(activity.avg_power_w)} W` : null },
    { label: 'NP', value: activity.normalized_power_w != null ? `${Math.round(activity.normalized_power_w)} W` : null },
    { label: 'Avg Cadence', value: activity.avg_cadence != null ? `${Math.round(activity.avg_cadence)} rpm` : null },
    { label: 'TSS', value: activity.tss != null ? `${Math.round(activity.tss)}` : null },
    { label: 'Avg Temp', value: activity.avg_temperature_c != null ? `${Math.round(activity.avg_temperature_c)} °C` : null },
  ]

  const visible = stats.filter(s => s.value && s.value !== '—')

  if (visible.length === 0) return null

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
      {visible.map(({ label, value, badge }) => (
        <div key={label} className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
          <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
          <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
            {value}{badge}
          </div>
        </div>
      ))}
    </div>
  )
}
