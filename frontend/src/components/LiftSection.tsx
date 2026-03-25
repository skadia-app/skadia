import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/auth'

interface LiftCandidate {
  id: string
  started_at: string
  ended_at: string
  duration_s: number
  ascent_m: number
  confidence: 'high' | 'medium'
  confirmed: boolean
}

interface LiftSectionProps {
  activityId: string
  liftCount: number
  liftDistanceM?: number | null
  liftAscentM?: number | null
  liftDescentM?: number | null
  liftDurationS?: number | null
  distanceM: number
  totalAscentM: number
  excludeLiftSubtraction: boolean
  onToggleExclude: (v: boolean) => Promise<void>
}

export default function LiftSection({
  activityId, liftCount, liftDistanceM, liftAscentM, liftDescentM,
  liftDurationS, distanceM, totalAscentM,
  excludeLiftSubtraction, onToggleExclude,
}: LiftSectionProps) {
  const qc = useQueryClient()

  const { data: candidates = [] } = useQuery<LiftCandidate[]>({
    queryKey: ['lift-candidates', activityId],
    queryFn: () => apiFetch(`/api/v1/activities/${activityId}/lift-candidates`).then(r => r.json()),
  })

  const confirm = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${id}/confirm`, { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] }),
  })

  const reject = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/activities/${activityId}/lift-candidates/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] }),
  })

  const unconfirmed = candidates.filter(c => !c.confirmed)
  const netDist = distanceM - (excludeLiftSubtraction ? 0 : (liftDistanceM ?? 0))
  const netAsc = totalAscentM - (excludeLiftSubtraction ? 0 : (liftAscentM ?? 0))

  if (liftCount === 0 && unconfirmed.length === 0) return null

  return (
    <section className="space-y-3">
      <h3 className="font-semibold text-sm">Liftfahrten</h3>

      {liftCount > 0 && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 space-y-2 text-sm">
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
            <span>{(netDist / 1000).toFixed(1)} km · {Math.round(netAsc)} m↑</span>
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
        </div>
      )}

      {unconfirmed.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {unconfirmed.length} erkannte Fahrten — bitte bestätigen:
          </p>
          {unconfirmed.map(c => {
            const start = new Date(c.started_at)
            const hhmm = `${start.getHours().toString().padStart(2, '0')}:${start.getMinutes().toString().padStart(2, '0')}`
            return (
              <div key={c.id} className="flex items-center gap-2 rounded border border-neutral-200 dark:border-neutral-800 p-2 text-sm">
                <span className="flex-1">
                  {hhmm} · +{Math.round(c.ascent_m)} m · {Math.round(c.duration_s / 60)} min
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
