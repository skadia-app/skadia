import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchPendingReviewGear,
  confirmGear,
  rejectGear,
  type PendingReviewGear,
} from '@/api/documents'

export default function DocumentReviewPage() {
  const qc = useQueryClient()
  const { data: items = [], isLoading } = useQuery({
    queryKey: ['pending-review-gear'],
    queryFn: fetchPendingReviewGear,
  })

  const confirmMutation = useMutation({
    mutationFn: confirmGear,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-review-gear'] }),
  })

  const rejectMutation = useMutation({
    mutationFn: rejectGear,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-review-gear'] }),
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade...</div>
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Dokument-Überprüfung
        </h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Aus Rechnungen automatisch erkannte Ausrüstung überprüfen und bestätigen.
        </p>
      </div>

      {items.length === 0 ? (
        <div className="text-center py-16 text-neutral-400">
          <p className="text-lg">Keine ausstehenden Überprüfungen</p>
          <p className="text-sm mt-1">Hochgeladene Rechnungen werden hier nach der Verarbeitung angezeigt.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {items.map(item => (
            <PendingGearCard
              key={item.id}
              item={item}
              onConfirm={() => confirmMutation.mutate(item.id)}
              onReject={() => rejectMutation.mutate(item.id)}
              isConfirming={confirmMutation.isPending}
              isRejecting={rejectMutation.isPending}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function PendingGearCard({ item, onConfirm, onReject, isConfirming, isRejecting }: {
  item: PendingReviewGear
  onConfirm: () => void
  onReject: () => void
  isConfirming: boolean
  isRejecting: boolean
}) {
  return (
    <div className="border border-amber-200 dark:border-amber-800/50 rounded-xl p-4 bg-amber-50/50 dark:bg-amber-900/10">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 space-y-1">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                             bg-amber-100 text-amber-800 dark:bg-amber-800/30 dark:text-amber-300">
              KI-Vorschlag
            </span>
            <span className="text-sm text-neutral-500 dark:text-neutral-400">{item.type}</span>
          </div>
          <p className="font-medium text-neutral-900 dark:text-neutral-100">{item.name}</p>
          {item.brand && (
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{item.brand}</p>
          )}
        </div>
        <div className="flex gap-2 flex-shrink-0">
          <button
            onClick={onReject}
            disabled={isRejecting}
            className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                       disabled:opacity-50 transition-colors"
          >
            Ablehnen
          </button>
          <button
            onClick={onConfirm}
            disabled={isConfirming}
            className="px-3 py-1.5 text-sm rounded-lg text-white
                       hover:opacity-90 disabled:opacity-50 transition-colors"
            style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
          >
            Bestätigen
          </button>
        </div>
      </div>
    </div>
  )
}
