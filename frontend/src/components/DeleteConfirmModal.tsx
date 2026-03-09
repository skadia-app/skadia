import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteActivity, deleteActivities } from '@/api/activities'

interface Props {
  /** Single ID or array for bulk delete. */
  activityIds: string[]
  /** Activity title (for single delete) */
  activityTitle?: string | null
  onClose: () => void
  onDeleted: () => void
  /** Wenn gesetzt: Aktivität hat Kinder/Quellen. Zeigt Auswahl-Dialog. */
  hasChildren?: boolean
}

export default function DeleteConfirmModal({
  activityIds,
  activityTitle,
  onClose,
  onDeleted,
  hasChildren,
}: Props) {
  const [state, setState] = useState<'idle' | 'loading' | 'error'>('idle')
  const [sourcesMode, setSourcesMode] = useState<'restore' | 'delete' | null>(null)
  const queryClient = useQueryClient()

  const isSingle = activityIds.length === 1

  const mutation = useMutation({
    mutationFn: () =>
      isSingle
        ? deleteActivity(activityIds[0], sourcesMode ?? undefined)
        : deleteActivities(activityIds).then(() => undefined),
    onMutate: () => setState('loading'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      activityIds.forEach(id =>
        queryClient.removeQueries({ queryKey: ['activity', id] }),
      )
      onDeleted()
      onClose()
    },
    onError: () => {
      setState('error')
      setTimeout(() => setState('idle'), 3000)
    },
  })

  const label = isSingle
    ? (activityTitle ?? 'diese Aktivität')
    : `${activityIds.length} Aktivitäten`

  // Warning text varies based on sourcesMode
  const warningText = sourcesMode === 'restore'
    ? 'Das Fusion-Ergebnis wird gelöscht. Die Quell-Aktivitäten werden wiederhergestellt und wieder sichtbar.'
    : isSingle
      ? 'Diese Aktion kann nicht rückgängig gemacht werden. Die Originaldatei wird dauerhaft aus dem Speicher entfernt.'
      : 'Diese Aktion kann nicht rückgängig gemacht werden. Alle Originaldateien werden dauerhaft aus dem Speicher entfernt.'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white dark:bg-neutral-900 rounded-2xl shadow-2xl border border-neutral-200 dark:border-neutral-800 w-full max-w-sm p-6 space-y-4">
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          {isSingle ? 'Aktivität löschen?' : `${activityIds.length} Aktivitäten löschen?`}
        </h2>

        {isSingle && activityTitle && (
          <p className="text-sm text-neutral-600 dark:text-neutral-400 font-medium truncate">
            „{activityTitle}"
          </p>
        )}

        {hasChildren && sourcesMode === null ? (
          <div className="space-y-3">
            <p className="text-sm text-neutral-700 dark:text-neutral-300">
              Diese Aktivität enthält verknüpfte Original-Aktivitäten. Was soll mit ihnen passieren?
            </p>
            <div className="flex flex-col gap-2">
              <button onClick={() => setSourcesMode('restore')} className="w-full px-4 py-2.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-800 text-left cursor-pointer">
                <span className="font-medium block">Originale wiederherstellen</span>
                <span className="text-xs text-neutral-500">Quell-Aktivitäten werden wieder sichtbar, nur das Ergebnis wird gelöscht</span>
              </button>
              <button onClick={() => setSourcesMode('delete')} className="w-full px-4 py-2.5 text-sm rounded-lg border border-red-200 dark:border-red-900 text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 text-left cursor-pointer">
                <span className="font-medium block">Alle löschen</span>
                <span className="text-xs text-red-500">Das Ergebnis UND alle Quell-Aktivitäten werden dauerhaft gelöscht</span>
              </button>
            </div>
            <button onClick={onClose} className="w-full px-4 py-2 text-sm text-neutral-500 hover:text-neutral-700 cursor-pointer">Abbrechen</button>
          </div>
        ) : (
          <>
            <div className="flex items-start gap-2 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg p-3">
              <svg className="w-4 h-4 text-red-500 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
              </svg>
              <p className="text-xs text-red-700 dark:text-red-400 leading-relaxed">
                {warningText}
              </p>
            </div>

            {sourcesMode === 'delete' && (
              <p className="text-xs font-medium text-red-700 dark:text-red-400">
                Die zusammengeführten Quellaktivitäten werden ebenfalls dauerhaft gelöscht.
              </p>
            )}

            {state === 'error' && (
              <p className="text-sm text-red-500">Fehler beim Löschen. Bitte erneut versuchen.</p>
            )}

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
                disabled={state === 'loading'}
                className="px-4 py-2 text-sm rounded-lg bg-red-600 hover:bg-red-700 text-white font-semibold disabled:opacity-50 cursor-pointer flex items-center gap-2"
              >
                {state === 'loading' ? (
                  <>
                    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Löschen…
                  </>
                ) : (
                  <>
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                    {label} löschen
                  </>
                )}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
