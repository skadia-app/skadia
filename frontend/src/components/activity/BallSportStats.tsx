import { useTranslation } from 'react-i18next'
import type { Activity } from '@/api/types'
import { formatSpeedKmh } from '@/lib/sportFieldLabels'
import { ActivityStatCard } from './ActivityStatCard'

interface Props { activity: Activity }

export function BallSportStats({ activity }: Props) {
  const { t } = useTranslation()
  const hasScore = activity.player_score != null || activity.opponent_score != null
  const hasBall = activity.max_ball_speed_ms != null || activity.avg_ball_speed_ms != null

  if (!hasScore && !hasBall) return null

  return (
    <div>
      <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-2">{t('activityTables.ballSports.title')}</h3>
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {hasScore && (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{t('activityTables.ballSports.result')}</div>
            <div className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
              {activity.player_score ?? 0} : {activity.opponent_score ?? 0}
            </div>
            {activity.opponent_name && (
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">vs. {activity.opponent_name}</div>
            )}
          </div>
        )}
        {activity.max_ball_speed_ms != null && (
          <ActivityStatCard label={t('activityTables.ballSports.maxBallSpeed')} value={formatSpeedKmh(activity.max_ball_speed_ms)} />
        )}
        {activity.avg_ball_speed_ms != null && (
          <ActivityStatCard label={t('activityTables.ballSports.avgBallSpeed')} value={formatSpeedKmh(activity.avg_ball_speed_ms)} />
        )}
      </div>
    </div>
  )
}
