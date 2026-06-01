import { useTranslation } from 'react-i18next'
import type { Activity, RecordingGroup } from '@/api/types'
import type { BestEffortRanked, NearMiss } from '@/api/bestEfforts'
import ActivityActionBar from '@/components/activity/ActivityActionBar'
import ActivityContextCard from '@/components/activity/ActivityContextCard'
import ActivitySourceTabs from '@/components/activity/ActivitySourceTabs'
import { StatsGrid } from '@/components/activity/StatsGrid'

interface ActivityOverviewProps {
  activity: Activity
  recordingGroup?: RecordingGroup | null
  activeActivityId?: string
  onActiveActivityChange?: (activityId: string) => void
  bestEfforts: BestEffortRanked[]
  nearMisses: NearMiss[]
  weatherDisplayFields?: string[]
}

export default function ActivityOverview({
  activity,
  recordingGroup,
  activeActivityId,
  onActiveActivityChange,
  bestEfforts,
  nearMisses,
  weatherDisplayFields,
}: ActivityOverviewProps) {
  const { t } = useTranslation()
  const sourceCount = recordingGroup?.members.length ?? 1
  const subtitle = sourceCount > 1
    ? t('activityDetail.overview.groupedSources', { count: sourceCount })
    : t('activityDetail.overview.singleActivity')

  return (
    <section className="grid gap-4 lg:grid-cols-[290px_minmax(0,1fr)]">
      <ActivityContextCard
        activity={activity}
        bestEfforts={bestEfforts}
        nearMisses={nearMisses}
        weatherDisplayFields={weatherDisplayFields}
      />
      <div className="rounded-lg border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-900">
        <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">{t('activityDetail.overview.metrics')}</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{subtitle}</p>
          </div>
          <ActivityActionBar activity={activity} />
        </div>

        <ActivitySourceTabs
          recordingGroup={recordingGroup}
          activeActivityId={activeActivityId ?? activity.id}
          onActiveActivityChange={onActiveActivityChange}
        />

        {activity.sport_type !== 'multisport' && (
          <div className="mt-3">
            <StatsGrid activity={activity} variant="overview" />
          </div>
        )}
      </div>
    </section>
  )
}
