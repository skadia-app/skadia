import type { Activity } from '@/api/types'
import { fmtDuration } from '@/lib/format'

interface Props { activity: Activity }

export function DiveStats({ activity }: Props) {
  const stats = [
    activity.max_depth_m != null ? { label: 'Max. Tiefe', value: `${activity.max_depth_m.toFixed(1)} m` } : null,
    activity.avg_depth_m != null ? { label: 'Ø Tiefe', value: `${activity.avg_depth_m.toFixed(1)} m` } : null,
    activity.dive_number != null ? { label: 'Tauchgang Nr.', value: `${activity.dive_number}` } : null,
    activity.start_cns_pct != null ? { label: 'CNS Start', value: `${activity.start_cns_pct.toFixed(1)} %` } : null,
    activity.end_cns_pct != null ? { label: 'CNS Ende', value: `${activity.end_cns_pct.toFixed(1)} %` } : null,
    activity.start_n2_pct != null ? { label: 'N₂ Start', value: `${activity.start_n2_pct.toFixed(1)} %` } : null,
    activity.end_n2_pct != null ? { label: 'N₂ Ende', value: `${activity.end_n2_pct.toFixed(1)} %` } : null,
    activity.surface_interval_s != null ? { label: 'Oberflächenintervall', value: fmtDuration(activity.surface_interval_s, 'short') } : null,
  ].filter(Boolean) as { label: string; value: string }[]

  if (stats.length === 0) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">Tauchgang</h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
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
