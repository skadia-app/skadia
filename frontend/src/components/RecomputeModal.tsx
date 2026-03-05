import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { reprocessAllActivities } from '@/api/activities'

export const REPROCESS_SCOPES = [
  ['streams',    'Streams & Track'],
  ['sensors',    'Sensoren'],
  ['metrics',    'Metriken'],
  ['details',    'Runden/Splits'],
  ['segments',   'Segmente & Runden'],
  ['sport_type', 'Sportart'],
  ['tags',       'Tags'],
] as const

type RecomputeScope = (typeof REPROCESS_SCOPES)[number][0]

const DEFAULT_SCOPES: RecomputeScope[] = ['streams', 'sensors', 'metrics', 'details', 'segments']

interface Props {
  /** IDs to recompute. Pass a single-element array for detail page use. */
  activityIds: string[]
  onClose: () => void
  /** Called after successful recompute (e.g. to invalidate queries). */
  onSuccess?: () => void
}

type Mode = 'all' | 'scoped'

export default function RecomputeModal({ activityIds, onClose, onSuccess }: Props) {
  const [mode, setMode] = useState<Mode>('all')
  const [selected, setSelected] = useState<Set<RecomputeScope>>(new Set(DEFAULT_SCOPES))
  const [state, setState] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () =>
      reprocessAllActivities({
        ids: activityIds,
        scope: mode === 'all' ? undefined : [...selected],
      }),
    onMutate: () => setState('loading'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      activityIds.forEach(id => {
        queryClient.invalidateQueries({ queryKey: ['activity', id] })
        queryClient.invalidateQueries({ queryKey: ['activity-streams', id] })
        queryClient.invalidateQueries({ queryKey: ['activity-sensors', id] })
      })
      setState('success')
      onSuccess?.()
      setTimeout(() => { setState('idle'); onClose() }, 1500)
    },
    onError: () => {
      setState('error')
      setTimeout(() => setState('idle'), 3000)
    },
  })

  function toggleScope(scope: RecomputeScope) {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(scope)) next.delete(scope)
      else next.add(scope)
      return next
    })
  }

  const count = activityIds.length
  const label = count === 1 ? '1 Aktivität' : `${count} Aktivitäten`

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white dark:bg-neutral-900 rounded-2xl shadow-2xl border border-neutral-200 dark:border-neutral-800 w-full max-w-sm p-6 space-y-5">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Neu berechnen — {label}
        </h2>

        {/* Mode selector */}
        <div className="space-y-2">
          <label className="flex items-center gap-3 cursor-pointer group">
            <input
              type="radio"
              checked={mode === 'all'}
              onChange={() => setMode('all')}
              className="accent-orange-500"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">
              Alles neu berechnen
            </span>
          </label>
          <label className="flex items-center gap-3 cursor-pointer group">
            <input
              type="radio"
              checked={mode === 'scoped'}
              onChange={() => setMode('scoped')}
              className="accent-orange-500"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">
              Details wählen
            </span>
          </label>
        </div>

        {/* Scope checkboxes */}
        {mode === 'scoped' && (
          <div className="ml-7 space-y-2 border-l-2 border-neutral-100 dark:border-neutral-800 pl-4">
            {REPROCESS_SCOPES.map(([key, scopeLabel]) => (
              <label key={key} className="flex items-center gap-3 cursor-pointer group">
                <input
                  type="checkbox"
                  checked={selected.has(key)}
                  onChange={() => toggleScope(key)}
                  className="accent-orange-500"
                />
                <span className="text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                  {scopeLabel}
                </span>
              </label>
            ))}
          </div>
        )}

        {/* State feedback */}
        {state === 'error' && (
          <p className="text-sm text-red-500">Fehler beim Neuberechnen. Bitte erneut versuchen.</p>
        )}
        {state === 'success' && (
          <p className="text-sm text-green-600 dark:text-green-400">Gestartet ✓</p>
        )}

        {/* Buttons */}
        <div className="flex justify-end gap-3 pt-1">
          <button
            onClick={onClose}
            disabled={state === 'loading'}
            className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 cursor-pointer"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={state === 'loading' || (mode === 'scoped' && selected.size === 0)}
            className="px-4 py-2 text-sm rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white font-medium disabled:opacity-50 cursor-pointer"
          >
            {state === 'loading' ? 'Läuft…' : 'Berechnen'}
          </button>
        </div>
      </div>
    </div>
  )
}
