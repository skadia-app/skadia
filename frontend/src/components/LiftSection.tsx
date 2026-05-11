import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { Activity, LiftCandidate } from '@/api/types'
import { apiFetch } from '@/lib/auth'
import { getNetActivityTotals } from '@/lib/netActivityTotals'

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
    queryFn: () => apiFetch(`/api/v1/activities/${activityId}/lifts/candidates`).then(r => r.json()),
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

  // Banner: show when the activity has unreviewed heuristic candidates.
  const unreviewedHeuristicCount = candidates.filter(
    c => c.source === 'heuristic' && !c.confirmed && !c.rejected_by_user,
  ).length
  const showReviewBanner =
    (activity.lifts_acknowledged_at == null) && unreviewedHeuristicCount > 0

  if (liftCount === 0 && unconfirmed.length === 0) return null

  return (
    <section className="space-y-3">
      <h3 className="font-semibold text-sm">Liftfahrten</h3>

      {showReviewBanner && (
        <div className="rounded-md border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950 px-3 py-2 flex items-center justify-between text-sm">
          <span className="text-amber-800 dark:text-amber-200">
            {unreviewedHeuristicCount === 1
              ? '1 Liftfahrt unbestätigt'
              : `${unreviewedHeuristicCount} Liftfahrten unbestätigt`}
          </span>
          {onOpenReview && (
            <button
              onClick={onOpenReview}
              className="cursor-pointer ml-3 rounded bg-amber-600 hover:bg-amber-700 px-3 py-1 text-xs font-medium text-white transition-colors"
            >
              Prüfen
            </button>
          )}
        </div>
      )}

      {liftCount > 0 && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-2 text-sm">
          {/* Headline row: count + combined elevation */}
          <div className="text-sm font-medium">
            {liftCount === 1 ? '1 Liftfahrt' : `${liftCount} Liftfahrten`}
            {' · '}
            {`+${Math.round(liftAscentM ?? 0)} / −${Math.round(liftDescentM ?? 0)} m`}
          </div>

          <div className="flex justify-between">
            <span className="text-neutral-500 dark:text-neutral-400">{liftCount} Fahrten</span>
            <span>{((liftDistanceM ?? 0) / 1000).toFixed(1)} km · {Math.round(liftDurationS ? liftDurationS / 60 : 0)} min</span>
          </div>
          <div className="flex justify-between">
            <span className="text-neutral-500 dark:text-neutral-400">Lift ↑ / ↓</span>
            <span>{Math.round(liftAscentM ?? 0)} / {Math.round(liftDescentM ?? 0)} m</span>
          </div>
          <div className="border-t border-neutral-200 dark:border-neutral-800 pt-2 flex justify-between font-medium">
            <span>Netto</span>
            <span>{(net.distanceM / 1000).toFixed(1)} km · {Math.round(net.elevationGainM)} m↑</span>
          </div>
          <label className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400 cursor-pointer">
            <input
              type="checkbox"
              checked={excludeLiftSubtraction}
              onChange={e => onToggleExclude(e.target.checked)}
              className="rounded"
            />
            Lifte herausrechnen deaktivieren
          </label>

          {/* Heuristic candidates — confirm/unconfirm list */}
          {heuristicCandidates.length > 0 && (
            <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
              {heuristicCandidates.map(c => {
                const start = new Date(c.started_at)
                const hhmm = `${start.getHours().toString().padStart(2, '0')}:${start.getMinutes().toString().padStart(2, '0')}`
                const ascentDesc = c.descent_m && c.descent_m > 0
                  ? `+${Math.round(c.ascent_m)} / −${Math.round(c.descent_m)} m`
                  : `+${Math.round(c.ascent_m)} m`

                return (
                  <div key={c.id} className="flex items-center gap-2 rounded border border-neutral-200 dark:border-neutral-800 p-2 text-sm">
                    <span className="flex-1">
                      {hhmm} · {ascentDesc} · {Math.round(c.duration_s / 60)} min
                      {c.confidence === 'high' && <span className="ml-1 text-xs text-green-600 dark:text-green-400">(sicher)</span>}
                      {' '}
                      <span className={`text-xs font-medium ${c.confirmed ? 'text-green-700 dark:text-green-400' : 'text-neutral-400'}`}>
                        {c.confirmed ? 'bestätigt' : 'offen'}
                      </span>
                    </span>
                    {!c.confirmed ? (
                      <>
                        <button
                          onClick={() => confirm.mutate(c.id)}
                          disabled={confirm.isPending}
                          className="text-xs px-2 py-1 rounded bg-accent-DEFAULT text-white cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          ✓ Bestätigen
                        </button>
                        <button
                          onClick={() => reject.mutate(c.id)}
                          disabled={reject.isPending}
                          className="text-xs px-2 py-1 rounded bg-neutral-100 dark:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          ✗ Verwerfen
                        </button>
                      </>
                    ) : (
                      <button
                        onClick={() => unconfirm.mutate(c.id)}
                        disabled={unconfirm.isPending}
                        title="Bestätigte Liftfahrten zählen in den Aktivitäts- und Gesamtstatistiken. Rückgängig macht den Confirm zurück — der Kandidat bleibt erhalten und kann erneut bestätigt werden."
                        className="text-xs px-2 py-1 rounded bg-neutral-100 dark:bg-neutral-800 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        ↶ Rückgängig
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
            {unconfirmed.length} erkannte Fahrten — bitte bestätigen:
          </p>
          {unconfirmed.map(c => {
            const start = new Date(c.started_at)
            const hhmm = `${start.getHours().toString().padStart(2, '0')}:${start.getMinutes().toString().padStart(2, '0')}`
            const ascentDesc = c.descent_m && c.descent_m > 0
              ? `+${Math.round(c.ascent_m)} / −${Math.round(c.descent_m)} m`
              : `+${Math.round(c.ascent_m)} m`
            return (
              <div key={c.id} className="flex items-center gap-2 rounded border border-neutral-200 dark:border-neutral-800 p-2 text-sm">
                <span className="flex-1">
                  {hhmm} · {ascentDesc} · {Math.round(c.duration_s / 60)} min
                  {c.confidence === 'high' && <span className="ml-1 text-xs text-green-600 dark:text-green-400">(sicher)</span>}
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
