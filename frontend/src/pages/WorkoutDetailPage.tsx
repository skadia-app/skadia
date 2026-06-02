import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useParams, Link } from 'react-router-dom'
import { fetchWorkout } from '@/api/workouts'
import { WorkoutOverview } from '@/components/activity/WorkoutOverview'
import type { WorkoutStep } from '@/api/types'
import { fmtDuration, fmtDateCompact } from '@/lib/format'
import type { WorkoutActivityRef } from '@/api/types'
import { sportLabel } from '@/lib/sportTaxonomy'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

const INTENSITY_LABEL_KEYS: Record<number, string> = {
  0: 'workoutDetail.intensities.interval',
  1: 'workoutDetail.intensities.warmup',
  2: 'workoutDetail.intensities.cooldown',
  3: 'workoutDetail.intensities.rest',
}

const INTENSITY_BADGE: Record<number, string> = {
  0: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  1: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  2: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  3: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
}

function formatStepDuration(
  step: WorkoutStep,
  t: ReturnType<typeof useTranslation>['t'],
  formatDistance: (meters: number | null | undefined) => string,
): string {
  const dt = step.duration_type
  const dv = step.duration_value

  if (dt === 0 || dt == null) {
    // Time-based: value in ms
    if (dv != null) return fmtDuration(dv / 1000, 'short')
    return '—'
  }
  if (dt === 1) {
    // Distance-based: value in m
    if (dv != null) return formatDistance(dv)
    return '—'
  }
  if (dt >= 14 && dt <= 19) {
    // Repeat step
    if (dv != null) return t('workoutDetail.repeatTimes', { count: dv })
    return t('workoutDetail.repeat')
  }
  return '—'
}

export default function WorkoutDetailPage() {
  const { t } = useTranslation()
  const { formatDistance, formatDistanceCompact } = useUnitFormatters()
  const { id } = useParams<{ id: string }>()
  const { data: workout, isLoading, isError } = useQuery({
    queryKey: ['workout', id],
    queryFn: () => fetchWorkout(id ?? ''),
    enabled: !!id,
  })

  if (isLoading) {
    return <div className="p-6 text-neutral-500">{t('workoutDetail.loading')}</div>
  }

  if (isError || !workout) {
    return (
      <div className="p-6">
        <p className="text-red-500">{t('workoutDetail.loadFailed')}</p>
        <Link to="/workouts" className="text-sm text-neutral-500 hover:underline mt-2 inline-block">
          {t('workoutDetail.backToOverview')}
        </Link>
      </div>
    )
  }

  return (
    <div className="max-w-4xl mx-auto p-6 space-y-6">
      {/* Header */}
      <div>
        <Link
          to="/workouts"
          className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 mb-3 inline-flex items-center gap-1"
        >
          <span aria-hidden="true">←</span> {t('workoutDetail.allWorkouts')}
        </Link>
        <h1 className="text-2xl font-bold">
          {workout.name || t('workoutDetail.unnamed')}
        </h1>
        {workout.description && (
          <p className="text-neutral-500 mt-1">{workout.description}</p>
        )}
        <div className="flex gap-4 mt-2 text-sm text-neutral-400">
          <span>{t('workoutDetail.stepCount', { count: workout.num_valid_steps ?? 0 })}</span>
          {workout.pool_length_m != null && (
            <span>{t('activityStats.poolLength')}: {formatDistanceCompact(workout.pool_length_m)}</span>
          )}
        </div>
      </div>

      {/* Visual overview */}
      {workout.steps.length > 0 && <WorkoutOverview workout={workout} />}

      {/* Steps table */}
      {workout.steps.length > 0 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
          <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
            <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              {t('workoutDetail.steps')}
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-800 text-neutral-500 text-xs">
                  <th className="px-4 py-2 text-left w-10">#</th>
                  <th className="px-4 py-2 text-left">{t('workoutDetail.table.name')}</th>
                  <th className="px-4 py-2 text-left">{t('workoutDetail.table.durationDistance')}</th>
                  <th className="px-4 py-2 text-left">{t('workoutDetail.table.intensity')}</th>
                </tr>
              </thead>
              <tbody>
                {workout.steps.map((step, i) => {
                  const intensityLabel = step.intensity != null
                    ? (INTENSITY_LABEL_KEYS[step.intensity] ? t(INTENSITY_LABEL_KEYS[step.intensity]) : String(step.intensity))
                    : '—'
                  const badgeClass = step.intensity != null
                    ? (INTENSITY_BADGE[step.intensity] ?? 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400')
                    : ''

                  return (
                    <tr
                      key={step.message_index}
                      className="border-b border-neutral-100 dark:border-neutral-800/60 last:border-0 hover:bg-neutral-50 dark:hover:bg-neutral-900/40"
                    >
                      <td className="px-4 py-2 text-neutral-400">{i + 1}</td>
                      <td className="px-4 py-2 font-medium">
                        {step.step_name || '—'}
                        {step.notes && (
                          <div className="text-xs text-neutral-400 font-normal">{step.notes}</div>
                        )}
                      </td>
                      <td className="px-4 py-2 tabular-nums text-neutral-600 dark:text-neutral-400">
                        {formatStepDuration(step, t, formatDistance)}
                      </td>
                      <td className="px-4 py-2">
                        {step.intensity != null ? (
                          <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${badgeClass}`}>
                            {intensityLabel}
                          </span>
                        ) : (
                          <span className="text-neutral-400">—</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {workout.steps.length === 0 && (
        <p className="text-neutral-400 text-sm">{t('workoutDetail.noSteps')}</p>
      )}

      {/* Completed activities */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-2">
          <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
            {t('workoutDetail.completed')}
          </h2>
          {(workout.activities?.length ?? 0) > 0 && (
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400">
              {workout.activities!.length}×
            </span>
          )}
        </div>
        {(workout.activities?.length ?? 0) === 0 ? (
          <p className="px-4 py-3 text-sm text-neutral-400">{t('workoutDetail.notCompletedYet')}</p>
        ) : (
          <div className="divide-y divide-neutral-100 dark:divide-neutral-800/60">
            {workout.activities!.map((a: WorkoutActivityRef) => (
              <Link
                key={a.id}
                to={`/activities/${a.id}`}
                className="flex items-center gap-3 px-4 py-3 hover:bg-neutral-50 dark:hover:bg-neutral-900/40 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <div className="font-medium truncate text-sm">
                    {a.title || sportLabel(a.sport_type)}
                  </div>
                  <div className="text-xs text-neutral-400 mt-0.5">
                    {fmtDateCompact(a.started_at)}
                  </div>
                </div>
                <div className="flex items-center gap-3 text-xs text-neutral-400 shrink-0 tabular-nums">
                  {a.distance_m != null && (
                    <span>{formatDistance(a.distance_m)}</span>
                  )}
                  {a.elapsed_time_s != null && (
                    <span>{fmtDuration(a.elapsed_time_s, 'short')}</span>
                  )}
                  <span aria-hidden="true" className="text-neutral-300 dark:text-neutral-600">→</span>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
