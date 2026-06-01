// frontend/src/components/activity/RunningDynamicsStats.tsx
import { useTranslation } from 'react-i18next'
import type { Activity } from '@/api/types'
import { formatShortLength } from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { ActivityStatCard } from './ActivityStatCard'

interface Props {
  activity: Activity
}

export function RunningDynamicsStats({ activity }: Props) {
  const { t } = useTranslation()
  const { options: unitOptions } = useUnitFormatters()
  const stats = [
    activity.avg_vertical_oscillation_mm != null
      ? { label: t('activityDetail.runningDynamics.avgVerticalOscillation'), value: `${(activity.avg_vertical_oscillation_mm / 10).toFixed(1)} cm` }
      : null,
    activity.avg_stance_time_ms != null
      ? { label: t('activityDetail.runningDynamics.avgGroundContactTime'), value: `${Math.round(activity.avg_stance_time_ms)} ms` }
      : null,
    activity.avg_step_length_mm != null
      ? { label: t('activityDetail.runningDynamics.avgStepLength'), value: formatShortLength(activity.avg_step_length_mm / 1000, unitOptions) }
      : null,
    activity.avg_vertical_ratio_pct != null
      ? { label: t('activityDetail.runningDynamics.verticalRatio'), value: `${activity.avg_vertical_ratio_pct.toFixed(1)} %` }
      : null,
    activity.avg_stance_time_balance_pct != null
      ? { label: 'GCT Balance', value: `${activity.avg_stance_time_balance_pct.toFixed(1)} / ${(100 - activity.avg_stance_time_balance_pct).toFixed(1)} %` }
      : null,
  ].filter(Boolean) as { label: string; value: string }[]

  if (stats.length === 0) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">{t('activityDetail.runningDynamics.title')}</h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
        {stats.map(s => (
          <ActivityStatCard key={s.label} label={s.label} value={s.value} />
        ))}
      </div>
    </div>
  )
}
