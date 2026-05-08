import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ModalShell } from '@/components/ui/ModalShell'
import { apiFetch } from '@/lib/auth'
import type { Activity, LiftCandidate } from '@/api/types'

interface LiftReviewModalProps {
  open: boolean
  onClose: () => void
  activity: Pick<Activity, 'id' | 'lifts_acknowledged_at'>
  candidates: LiftCandidate[]
}

type ModalMode = 'compact' | 'expanded'

export default function LiftReviewModal({ open, onClose, activity, candidates }: LiftReviewModalProps) {
  const activityId = activity.id
  const qc = useQueryClient()

  // Track whether a bulk action (confirm/reject) already handled the backend state.
  // If true, closing via X skips the acknowledge call.
  const [bulkActionDone, setBulkActionDone] = useState(false)

  const [mode, setMode] = useState<ModalMode>('compact')

  const unreviewedCandidates = candidates.filter(
    c => c.source === 'heuristic' && !c.confirmed && !c.rejected_by_user,
  )
  const n = unreviewedCandidates.length
  const totalAscent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.ascent_m ?? 0), 0))
  const totalDescent = Math.round(unreviewedCandidates.reduce((s, c) => s + (c.descent_m ?? 0), 0))

  const invalidateAfterBulk = () => {
    qc.invalidateQueries({ queryKey: ['activity', activityId] })
    qc.invalidateQueries({ queryKey: ['lift-candidates', activityId] })
  }

  const confirmAllMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/confirm-all`, { method: 'POST' }),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const rejectAllMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/reject-all`, { method: 'POST' }),
    onSuccess: () => {
      invalidateAfterBulk()
      setBulkActionDone(true)
      onClose()
    },
  })

  const acknowledgeMutation = useMutation({
    mutationFn: () =>
      apiFetch(`/api/v1/activities/${activityId}/lifts/acknowledge`, { method: 'POST' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['activity', activityId] })
    },
  })

  // Called when user explicitly closes via X or backdrop click.
  // Fires acknowledge only if no bulk action was already taken.
  const handleClose = () => {
    if (!bulkActionDone) {
      acknowledgeMutation.mutate()
    }
    onClose()
  }

  if (!open) return null

  return (
    <ModalShell onClose={handleClose} maxWidth="max-w-md">
      {mode === 'compact' ? (
        <div className="space-y-4">
          <div className="flex items-start justify-between gap-4">
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
              Lifte erkannt
            </h2>
            <button
              onClick={handleClose}
              aria-label="Schließen"
              className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors"
            >
              ✕
            </button>
          </div>

          <p className="text-sm text-neutral-600 dark:text-neutral-400">
            {n === 1
              ? '1 Liftfahrt wurde erkannt — ist sie korrekt?'
              : `${n} Liftfahrten wurden erkannt — sind sie korrekt?`}
          </p>

          <ul className="text-sm text-neutral-700 dark:text-neutral-300">
            <li>
              {n === 1 ? '1 Liftfahrt' : `${n} Liftfahrten`}
              {' · '}
              {`+${totalAscent} m / −${totalDescent} m`}
            </li>
          </ul>

          <div className="flex flex-col gap-2 sm:flex-row sm:gap-2 pt-1">
            <button
              onClick={() => confirmAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg bg-accent-DEFAULT px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              Ja, alle bestätigen
            </button>
            <button
              onClick={() => rejectAllMutation.mutate()}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              Nein, alle ablehnen
            </button>
            <button
              onClick={() => setMode('expanded')}
              disabled={confirmAllMutation.isPending || rejectAllMutation.isPending}
              className="cursor-pointer flex-1 rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              Prüfen
            </button>
          </div>
        </div>
      ) : (
        // PASS-B: Inspector-Map + Liste (Phase 6.3.4) wird hier ausgebaut.
        <div className="space-y-4">
          <div className="flex items-start justify-between gap-4">
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
              Lifte prüfen
            </h2>
            <button
              onClick={handleClose}
              aria-label="Schließen"
              className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors"
            >
              ✕
            </button>
          </div>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 italic">
            Inspector-Ansicht (Karte + Lift-Liste) folgt in einem zweiten Schritt.
          </p>
          <button
            onClick={() => setMode('compact')}
            className="cursor-pointer rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
          >
            Zurück zur Übersicht
          </button>
        </div>
      )}
    </ModalShell>
  )
}
