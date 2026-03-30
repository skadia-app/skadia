import { useQuery } from '@tanstack/react-query'
import { useParams, Link } from 'react-router-dom'
import { fetchWorkout } from '@/api/workouts'
import { WorkoutOverview } from '@/components/activity/WorkoutOverview'
import type { WorkoutStep } from '@/api/types'
import { fmtDuration } from '@/lib/format'

const INTENSITY_LABELS: Record<number, string> = {
  0: 'Intervall',
  1: 'Aufwärmen',
  2: 'Abkühlen',
  3: 'Pause',
}

const INTENSITY_BADGE: Record<number, string> = {
  0: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  1: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  2: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  3: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
}

function formatDuration(step: WorkoutStep): string {
  const dt = step.duration_type
  const dv = step.duration_value

  if (dt === 0 || dt == null) {
    // Time-based: value in ms
    if (dv != null) return fmtDuration(dv / 1000, 'short')
    return '—'
  }
  if (dt === 1) {
    // Distance-based: value in m
    if (dv != null) return `${(dv / 1000).toFixed(2)} km`
    return '—'
  }
  if (dt >= 14 && dt <= 19) {
    // Repeat step
    if (dv != null) return `${dv}× wiederholen`
    return 'Wiederholung'
  }
  return '—'
}

export default function WorkoutDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: workout, isLoading, isError } = useQuery({
    queryKey: ['workout', id],
    queryFn: () => fetchWorkout(id ?? ''),
    enabled: !!id,
  })

  if (isLoading) {
    return <div className="p-6 text-neutral-500">Lade Workout...</div>
  }

  if (isError || !workout) {
    return (
      <div className="p-6">
        <p className="text-red-500">Workout konnte nicht geladen werden.</p>
        <Link to="/workouts" className="text-sm text-neutral-500 hover:underline mt-2 inline-block">
          Zurück zur Übersicht
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
          <span aria-hidden="true">←</span> Alle Workouts
        </Link>
        <h1 className="text-2xl font-bold">
          {workout.name || 'Unbenanntes Workout'}
        </h1>
        {workout.description && (
          <p className="text-neutral-500 mt-1">{workout.description}</p>
        )}
        <div className="flex gap-4 mt-2 text-sm text-neutral-400">
          <span>{workout.num_valid_steps ?? 0} Schritte</span>
          {workout.pool_length_m != null && (
            <span>Bahn: {workout.pool_length_m} m</span>
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
              Schritte
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-800 text-neutral-500 text-xs">
                  <th className="px-4 py-2 text-left w-10">#</th>
                  <th className="px-4 py-2 text-left">Name</th>
                  <th className="px-4 py-2 text-left">Dauer / Distanz</th>
                  <th className="px-4 py-2 text-left">Intensität</th>
                </tr>
              </thead>
              <tbody>
                {workout.steps.map((step, i) => {
                  const intensityLabel = step.intensity != null
                    ? (INTENSITY_LABELS[step.intensity] ?? String(step.intensity))
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
                        {formatDuration(step)}
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
        <p className="text-neutral-400 text-sm">Keine Schritte vorhanden.</p>
      )}
    </div>
  )
}
