import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { previewRename, executeRename } from '@/api/settings'
import type { RenameResult } from '@/api/settings'

interface RenamePreviewProps {
  open: boolean
  onClose: () => void
  schema: string
}

export default function RenamePreview({ open, onClose, schema }: RenamePreviewProps) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [result, setResult] = useState<RenameResult | null>(null)

  const { data: items, isLoading, isError, error } = useQuery({
    queryKey: ['rename-preview', schema],
    queryFn: () => previewRename(schema),
    enabled: open && schema.length > 0,
  })

  const renameMutation = useMutation({
    mutationFn: executeRename,
    onSuccess: (data) => {
      setResult(data)
      setConfirmOpen(false)
    },
  })

  function handleClose() {
    setConfirmOpen(false)
    setResult(null)
    renameMutation.reset()
    onClose()
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" onClick={handleClose} />

      {/* Dialog */}
      <div className="relative bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800
                      shadow-xl w-full max-w-2xl max-h-[80vh] flex flex-col mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-neutral-800">
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Dateien reorganisieren
          </h2>
          <button
            onClick={handleClose}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {isLoading && (
            <div className="flex items-center justify-center py-8 text-neutral-400">
              Lade Vorschau...
            </div>
          )}

          {isError && (
            <div className="rounded-lg border border-rose-200 dark:border-rose-900/50 bg-rose-50 dark:bg-rose-900/20 p-4 text-sm text-rose-600 dark:text-rose-400">
              {error instanceof Error ? error.message : 'Vorschau konnte nicht geladen werden.'}
            </div>
          )}

          {result && (
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 text-center space-y-2">
              <p className="text-lg font-medium text-neutral-900 dark:text-neutral-100">
                Reorganisierung abgeschlossen
              </p>
              <p className="text-sm text-neutral-600 dark:text-neutral-400">
                <span className="text-emerald-600 dark:text-emerald-400 font-medium">{result.succeeded}</span> erfolgreich
                {result.failed > 0 && (
                  <>, <span className="text-rose-500 font-medium">{result.failed}</span> fehlgeschlagen</>
                )}
              </p>
            </div>
          )}

          {!isLoading && !result && items && (
            <>
              <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-4">
                {items.length === 0
                  ? 'Alle Dateien entsprechen bereits dem aktuellen Schema.'
                  : `${items.length} ${items.length === 1 ? 'Datei wird' : 'Dateien werden'} verschoben`}
              </p>

              {items.length > 0 && (
                <div className="space-y-1.5">
                  {items.map((item) => (
                    <div
                      key={item.old_path}
                      className="flex items-center gap-2 text-xs font-mono rounded-lg p-2
                                 bg-neutral-50 dark:bg-neutral-800/50"
                    >
                      <span className="text-neutral-500 dark:text-neutral-400 truncate flex-1 text-right">
                        {item.old_path}
                      </span>
                      <svg className="w-4 h-4 text-accent-DEFAULT flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" />
                      </svg>
                      <span className="text-neutral-800 dark:text-neutral-200 truncate flex-1">
                        {item.new_path}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}

          {renameMutation.isPending && (
            <div className="mt-4">
              <div className="h-2 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
                <div className="h-full bg-accent-DEFAULT rounded-full animate-pulse w-full" />
              </div>
              <p className="text-xs text-neutral-400 mt-2 text-center">Dateien werden verschoben...</p>
            </div>
          )}

          {renameMutation.isError && (
            <p className="mt-4 text-sm text-rose-500">
              Fehler: {renameMutation.error instanceof Error ? renameMutation.error.message : 'Unbekannter Fehler'}
            </p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-neutral-200 dark:border-neutral-800">
          {result ? (
            <button
              onClick={handleClose}
              className="px-4 py-2 rounded-lg text-sm font-medium
                         bg-accent-DEFAULT hover:bg-accent-600 text-white transition-colors"
            >
              Schließen
            </button>
          ) : confirmOpen ? (
            <>
              <span className="text-sm text-neutral-600 dark:text-neutral-400 mr-auto">
                Wirklich alle Dateien verschieben?
              </span>
              <button
                onClick={() => setConfirmOpen(false)}
                disabled={renameMutation.isPending}
                className="px-4 py-2 rounded-lg text-sm font-medium
                           border border-neutral-200 dark:border-neutral-700
                           text-neutral-600 dark:text-neutral-300
                           hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                Abbrechen
              </button>
              <button
                onClick={() => renameMutation.mutate(items ?? [])}
                disabled={renameMutation.isPending}
                className="px-4 py-2 rounded-lg text-sm font-medium text-white
                           bg-rose-500 hover:bg-rose-600
                           disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                {renameMutation.isPending ? 'Wird ausgeführt...' : 'Ja, verschieben'}
              </button>
            </>
          ) : (
            <>
              <button
                onClick={handleClose}
                className="px-4 py-2 rounded-lg text-sm font-medium
                           border border-neutral-200 dark:border-neutral-700
                           text-neutral-600 dark:text-neutral-300
                           hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                Abbrechen
              </button>
              <button
                onClick={() => setConfirmOpen(true)}
                disabled={!items || items.length === 0}
                className="px-4 py-2 rounded-lg text-sm font-medium text-white
                           bg-accent-DEFAULT hover:bg-accent-600
                           disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                Reorganisieren
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
