import type { Activity } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { ActivityStatCard } from './ActivityStatCard'

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
    activity.o2_toxicity_otu != null ? { label: 'O₂-Toxizität (OTU)', value: `${Math.round(activity.o2_toxicity_otu)}` } : null,
    activity.surface_interval_s != null ? { label: 'Oberflächenintervall', value: fmtDuration(activity.surface_interval_s, 'short') } : null,
  ].filter(Boolean) as { label: string; value: string }[]

  if (stats.length === 0) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">Tauchgang</h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
        {stats.map(s => (
          <ActivityStatCard key={s.label} label={s.label} value={s.value} />
        ))}
      </div>
    </div>
  )
}
