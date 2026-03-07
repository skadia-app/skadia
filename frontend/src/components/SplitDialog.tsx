import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { apiClient } from '@/api/client'
import type { Activity } from '@/api/types'
import { useToast } from '@/lib/toast'
import { formatDuration } from '@/lib/utils'
import { useAccentColor } from '@/lib/accentColor'

interface SplitDialogProps {
  activity: Activity
  onClose: () => void
}

interface SplitResponse {
  activity_id: string
  new_activity_id: string
}

export default function SplitDialog({ activity, onClose }: SplitDialogProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { accent } = useAccentColor()
  const navigate = useNavigate()

  const totalSeconds = activity.elapsed_time_s ?? 0

  // Offset from start in minutes
  const [splitOffsetMin, setSplitOffsetMin] = useState('')

  const splitOffsetS = splitOffsetMin ? parseFloat(splitOffsetMin) * 60 : 0
  const part1S = Math.round(splitOffsetS)
  const part2S = Math.round(Math.max(0, totalSeconds - splitOffsetS))

  const isValid =
    splitOffsetMin !== '' &&
    splitOffsetS > 30 &&
    part2S > 30 &&
    splitOffsetS < totalSeconds

  const mutation = useMutation({
    mutationFn: async () => {
      const startedAt = new Date(activity.started_at)
      const splitAt = new Date(startedAt.getTime() + splitOffsetS * 1000)
      return apiClient<SplitResponse>(`/activities/${activity.id}/split`, {
        method: 'POST',
        body: { split_at: splitAt.toISOString() },
      })
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      addToast({
        type: 'success',
        title: 'Aktivität aufgeteilt',
        message: 'Zwei Aktivitäten wurden erstellt.',
      })
      onClose()
      // Navigate to the new (second) activity.
      navigate(`/activities/${result.new_activity_id}`)
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Aufteilung fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

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
        aria-labelledby="split-dialog-title"
      >
        <h2 id="split-dialog-title" className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          Aktivitat aufteilen
        </h2>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Gesamtdauer: <strong>{formatDuration(totalSeconds)}</strong>
        </p>
        <p className="mt-1 text-xs text-neutral-400 dark:text-neutral-500">
          Es wird eine zweite Aktivitat ab dem Teilungspunkt erstellt.
        </p>

        <div className="mt-4">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Teilungspunkt (Minuten ab Start)
          </label>
          <input
            type="number"
            min="1"
            step="0.5"
            placeholder="z.B. 30"
            value={splitOffsetMin}
            onChange={e => setSplitOffsetMin(e.target.value)}
            className="w-full px-3 py-2 border border-neutral-300 rounded-md dark:bg-neutral-900 dark:border-neutral-700 dark:text-neutral-50 focus:outline-none focus:ring-2 focus:ring-offset-0 text-sm"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          />
        </div>

        {splitOffsetMin !== '' && (
          <div className="mt-3 grid grid-cols-2 gap-2">
            <div className="rounded-md bg-neutral-100 dark:bg-neutral-800 px-3 py-2 text-center">
              <p className="text-xs text-neutral-500 dark:text-neutral-400">Teil 1</p>
              <p className="text-sm font-semibold text-neutral-900 dark:text-neutral-50 mt-0.5">
                {part1S > 0 ? formatDuration(part1S) : '—'}
              </p>
            </div>
            <div className="rounded-md bg-neutral-100 dark:bg-neutral-800 px-3 py-2 text-center">
              <p className="text-xs text-neutral-500 dark:text-neutral-400">Teil 2</p>
              <p className={`text-sm font-semibold mt-0.5 ${part2S <= 0 ? 'text-red-500' : 'text-neutral-900 dark:text-neutral-50'}`}>
                {part2S > 0 ? formatDuration(part2S) : 'Ungultig'}
              </p>
            </div>
          </div>
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
            {mutation.isPending ? 'Wird aufgeteilt…' : 'Aufteilen'}
          </button>
        </div>
      </div>
    </div>
  )
}
