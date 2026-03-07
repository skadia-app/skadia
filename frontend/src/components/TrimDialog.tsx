import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import type { Activity } from '@/api/types'
import { useToast } from '@/lib/toast'
import { formatDuration } from '@/lib/utils'
import { useAccentColor } from '@/lib/accentColor'

interface TrimDialogProps {
  activity: Activity
  onClose: () => void
}

export default function TrimDialog({ activity, onClose }: TrimDialogProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { accent } = useAccentColor()

  // Duration in seconds
  const totalSeconds = activity.elapsed_time_s ?? 0

  const [trimStartMin, setTrimStartMin] = useState('')
  const [trimEndMin, setTrimEndMin] = useState('')

  const mutation = useMutation({
    mutationFn: async () => {
      const startedAt = new Date(activity.started_at)

      const body: Record<string, string> = {}

      if (trimStartMin !== '') {
        const offsetS = parseFloat(trimStartMin) * 60
        const newStart = new Date(startedAt.getTime() + offsetS * 1000)
        body.trim_start = newStart.toISOString()
      }
      if (trimEndMin !== '') {
        const offsetFromEndS = parseFloat(trimEndMin) * 60
        const effectiveEndedAt = activity.ended_at
          ? new Date(activity.ended_at)
          : new Date(startedAt.getTime() + totalSeconds * 1000)
        const newEnd = new Date(effectiveEndedAt.getTime() - offsetFromEndS * 1000)
        body.trim_end = newEnd.toISOString()
      }

      return apiClient<Activity>(`/activities/${activity.id}/trim`, {
        method: 'POST',
        body,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      addToast({ type: 'success', title: 'Aktivität getrimmt', message: 'Metriken wurden neu berechnet.' })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Trim fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

  // Calculate preview durations
  const startOffsetS = trimStartMin ? parseFloat(trimStartMin) * 60 : 0
  const endOffsetS = trimEndMin ? parseFloat(trimEndMin) * 60 : 0
  const newDuration = Math.max(0, totalSeconds - startOffsetS - endOffsetS)

  const isValid =
    (trimStartMin !== '' || trimEndMin !== '') &&
    newDuration > 0 &&
    startOffsetS + endOffsetS < totalSeconds

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" />
      <div
        className="relative bg-white dark:bg-neutral-950 rounded-lg shadow-lg p-6 max-w-sm w-[90vw]"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="trim-dialog-title"
      >
        <h2 id="trim-dialog-title" className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          Aktivität trimmen
        </h2>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Aktuelle Dauer: <strong>{formatDuration(totalSeconds)}</strong>
        </p>
        <p className="mt-1 text-xs text-neutral-400 dark:text-neutral-500">
          Originaldaten bleiben erhalten. Nur die Metriken werden aktualisiert.
        </p>

        <div className="mt-4 space-y-3">
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Vorne abschneiden (Minuten)
            </label>
            <input
              type="number"
              min="0"
              step="0.5"
              placeholder="0"
              value={trimStartMin}
              onChange={e => setTrimStartMin(e.target.value)}
              className="w-full px-3 py-2 border border-neutral-300 rounded-md dark:bg-neutral-900 dark:border-neutral-700 dark:text-neutral-50 focus:outline-none focus:ring-2 focus:ring-offset-0 text-sm"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Hinten abschneiden (Minuten)
            </label>
            <input
              type="number"
              min="0"
              step="0.5"
              placeholder="0"
              value={trimEndMin}
              onChange={e => setTrimEndMin(e.target.value)}
              className="w-full px-3 py-2 border border-neutral-300 rounded-md dark:bg-neutral-900 dark:border-neutral-700 dark:text-neutral-50 focus:outline-none focus:ring-2 focus:ring-offset-0 text-sm"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            />
          </div>
        </div>

        {(trimStartMin !== '' || trimEndMin !== '') && (
          <p className="mt-3 text-sm text-neutral-600 dark:text-neutral-400">
            Neue Dauer:{' '}
            <strong className={newDuration <= 0 ? 'text-red-500' : ''}>
              {newDuration > 0 ? formatDuration(Math.round(newDuration)) : 'Ungültig'}
            </strong>
          </p>
        )}

        <div className="mt-6 flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-md text-sm font-medium bg-neutral-200 hover:bg-neutral-300 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={!isValid || mutation.isPending}
            className="px-4 py-2 rounded-md text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed hover:opacity-90"
            style={{ backgroundColor: accent.shade500 }}
          >
            {mutation.isPending ? 'Wird verarbeitet…' : 'Trimmen'}
          </button>
        </div>
      </div>
    </div>
  )
}
