import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import '@/i18n/i18n'
import type { Activity, LiftCandidate } from '@/api/types'
import { apiFetch } from '@/lib/auth'
import { getNetActivityTotals } from '@/lib/netActivityTotals'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

type ActivityLiftProps = Pick<
  Activity,
  | 'id'
  | 'distance_m'
  | 'elevation_gain_m'
  | 'elevation_loss_m'
  | 'lift_count'
  | 'lift_distance_m'
  | 'lift_ascent_m'
  | 'lift_descent_m'
  | 'lift_duration_s'
  | 'exclude_lift_subtraction'
  | 'lift_detection_osm_override'
  | 'lift_detection_heuristic_override'
  | 'lifts_acknowledged_at'
>

interface LiftSectionProps {
  activity: ActivityLiftProps
  onToggleExclude: (v: boolean) => Promise<void>
  // Optional: opens the bulk-review modal from the parent.
  // When absent the banner renders without a button (read-only notice).
  onOpenReview?: () => void
}

async function throwOnError(res: Response): Promise<Response> {
  if (res.ok) return res
  const body = await res.json().catch(() => ({})) as Record<string, unknown>
  const msg = typeof body.message === 'string' ? body.message : `HTTP ${res.status}`
  throw new Error(msg)
}

export default function LiftSection({ activity, onToggleExclude, onOpenReview }: LiftSectionProps) {
  const { t } = useTranslation()
  const { formatDistance, formatElevation } = useUnitFormatters()
  const activityId = activity.id
  const liftCount = activity.lift_count ?? 0
  const liftDistanceM = activity.lift_distance_m
  const liftAscentM = activity.lift_ascent_m
  const liftDescentM = activity.lift_descent_m
  const liftDurationS = activity.lift_duration_s
  const excludeLiftSubtraction = activity.exclude_lift_subtraction ?? false

  const qc = useQueryClient()

  const { data: candidates = [] } = useQuery<LiftCandidate[]>({
    queryKey: ['lift-candidates', activityId],
    queryFn: async () => {
      const data = await apiFetch(`/api/v1/activities/${activityId}/lifts/candidates`).then(r => r.json())
      return Array.isArray(data) ? data : []
    },
  })

  const confirm = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${id}/confirm`, { method: 'POST' }).then(throwOnError),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const reject = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${id}`, { method: 'DELETE' }).then(throwOnError),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const unconfirm = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${id}/unconfirm`, { method: 'PATCH' }).then(throwOnError),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  const net = getNetActivityTotals(activity as Activity)

  // Heuristic candidates not rejected by user — shown for confirmation/unconfirmation
  const heuristicCandidates = candidates.filter(
    c => c.source === 'heuristic' && !c.rejected_by_user,
  )
  const unconfirmed = heuristicCandidates.filter(c => !c.confirmed)

  const unreviewedCandidateCount = candidates.filter(
    c => !c.confirmed && !c.rejected_by_user,
  ).length
  const detectionDisabledForActivity =
    activity.lift_detection_osm_override === false &&
    activity.lift_detection_heuristic_override === false
  const showReviewBanner =
    detectionDisabledForActivity ||
    unreviewedCandidateCount > 0

  if (liftCount === 0 && unconfirmed.length === 0 && !showReviewBanner) return null

  return (
    <section className="space-y-3">
      <h3 className="font-semibold text-sm">{t('liftSection.title')}</h3>

      {showReviewBanner && (
        <div className="rounded-md border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950 px-3 py-2 flex items-center justify-between text-sm">
          <span className="text-amber-800 dark:text-amber-200">
            {detectionDisabledForActivity
              ? t('liftSection.detectionDisabled')
              : unreviewedCandidateCount === 1
                ? t('liftSection.unconfirmedCount', { count: unreviewedCandidateCount })
                : t('liftSection.unconfirmedCount', { count: unreviewedCandidateCount })}
          </span>
          {onOpenReview && (
            <button
              onClick={onOpenReview}
              className="cursor-pointer ml-3 rounded bg-amber-600 hover:bg-amber-700 px-3 py-1 text-xs font-medium text-white transition-colors"
            >
              {t('liftSection.review')}
            </button>
          )}
        </div>
      )}

      {liftCount > 0 && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-2 text-sm">
          {/* Headline row: count + combined elevation */}
          <div className="text-sm font-medium">
            {t('liftSection.rideCount', { count: liftCount })}
            {' · '}
            {`+${formatElevation(liftAscentM ?? 0)} / −${formatElevation(liftDescentM ?? 0)}`}
          </div>

          <div className="flex justify-between">
            <span className="text-neutral-500 dark:text-neutral-400">{t('liftSection.tripCount', { count: liftCount })}</span>
            <span>{formatDistance(liftDistanceM ?? 0)} · {Math.round(liftDurationS ? liftDurationS / 60 : 0)} min</span>
          </div>
          <div className="flex justify-between">
            <span className="text-neutral-500 dark:text-neutral-400">{t('liftSection.liftElevation')}</span>
            <span>{formatElevation(liftAscentM ?? 0)} / {formatElevation(liftDescentM ?? 0)}</span>
          </div>
          <div className="border-t border-neutral-200 dark:border-neutral-800 pt-2 flex justify-between font-medium">
            <span>{t('liftSection.net')}</span>
            <span>{formatDistance(net.distanceM)} · {formatElevation(net.elevationGainM)}↑</span>
          </div>
          <label className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400 cursor-pointer">
            <input
              type="checkbox"
              checked={excludeLiftSubtraction}
              onChange={e => onToggleExclude(e.target.checked)}
              className="rounded"
            />
            {t('liftSection.disableSubtraction')}
          </label>

          {/* Heuristic candidates — confirm/unconfirm list */}
          {heuristicCandidates.length > 0 && (
            <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
              {heuristicCandidates.map(c => {
                const start = new Date(c.started_at)
                const hhmm = `${start.getHours().toString().padStart(2, '0')}:${start.getMinutes().toString().padStart(2, '0')}`
                const ascentDesc = c.descent_m && c.descent_m > 0
                  ? `+${formatElevation(c.ascent_m)} / −${formatElevation(c.descent_m)}`
                  : `+${formatElevation(c.ascent_m)}`

                return (
                  <div key={c.id} className="flex items-center gap-2 rounded border border-neutral-200 dark:border-neutral-800 p-2 text-sm">
                    <span className="flex-1">
                      {hhmm} · {ascentDesc} · {Math.round(c.duration_s / 60)} min
                      {c.confidence === 'high' && <span className="ml-1 text-xs text-green-600 dark:text-green-400">{t('liftSection.highConfidence')}</span>}
                      {' '}
                      <span className={`text-xs font-medium ${c.confirmed ? 'text-green-700 dark:text-green-400' : 'text-neutral-400'}`}>
                        {c.confirmed ? t('liftSection.confirmed') : t('liftSection.open')}
                      </span>
                    </span>
                    {!c.confirmed ? (
                      <>
                        <button
                          onClick={() => confirm.mutate(c.id)}
                          disabled={confirm.isPending}
                          className="text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          ✓ {t('liftSection.confirm')}
                        </button>
                        <button
                          onClick={() => reject.mutate(c.id)}
                          disabled={reject.isPending}
                          className="text-xs px-2 py-1 rounded bg-neutral-100 dark:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          ✗ {t('liftSection.reject')}
                        </button>
                      </>
                    ) : (
                      <button
                        onClick={() => unconfirm.mutate(c.id)}
                        disabled={unconfirm.isPending}
                        title={t('liftSection.undoTitle')}
                        className="text-xs px-2 py-1 rounded bg-neutral-100 dark:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {t('liftSection.undo')}
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      )}

      {unconfirmed.length > 0 && liftCount === 0 && (
        <div className="space-y-2">
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t('liftSection.detectedPrompt', { count: unconfirmed.length })}
          </p>
          {unconfirmed.map(c => {
            const start = new Date(c.started_at)
            const hhmm = `${start.getHours().toString().padStart(2, '0')}:${start.getMinutes().toString().padStart(2, '0')}`
            const ascentDesc = c.descent_m && c.descent_m > 0
              ? `+${formatElevation(c.ascent_m)} / −${formatElevation(c.descent_m)}`
              : `+${formatElevation(c.ascent_m)}`
            return (
              <div key={c.id} className="flex items-center gap-2 rounded border border-neutral-200 dark:border-neutral-800 p-2 text-sm">
                <span className="flex-1">
                  {hhmm} · {ascentDesc} · {Math.round(c.duration_s / 60)} min
                  {c.confidence === 'high' && <span className="ml-1 text-xs text-green-600 dark:text-green-400">{t('liftSection.highConfidence')}</span>}
                </span>
                <button
                  onClick={() => confirm.mutate(c.id)}
                  disabled={confirm.isPending}
                  className="text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  ✓
                </button>
                <button
                  onClick={() => reject.mutate(c.id)}
                  disabled={reject.isPending}
                  className="text-xs px-2 py-1 rounded bg-neutral-100 dark:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  ✗
                </button>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
