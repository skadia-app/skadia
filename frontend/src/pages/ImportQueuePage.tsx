import { useState, useCallback, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import AuthenticatedImage from '@/components/AuthenticatedImage'
import { useToast } from '@/lib/toast'
import {
  fetchImportQueue,
  assignQueueItem,
  patchQueueItem,
  type ImportQueueItem,
} from '@/api/importQueue'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'

// ---------------------------------------------------------------------------
// Activity search modal
// ---------------------------------------------------------------------------

interface ActivitySearchModalProps {
  onSelect: (activity: Activity) => void
  onClose: () => void
}

function ActivitySearchModal({ onSelect, onClose }: ActivitySearchModalProps) {
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const handleQueryChange = useCallback((value: string) => {
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setDebouncedQuery(value), 300)
  }, [])

  const { data, isLoading } = useQuery({
    queryKey: ['activity-search-modal', debouncedQuery],
    queryFn: () =>
      fetchActivities(undefined, 10, undefined, undefined, {
        search: debouncedQuery || undefined,
      }),
    enabled: debouncedQuery.length >= 2,
  })

  const activities = data?.activities ?? []

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="bg-white dark:bg-neutral-900 rounded-2xl shadow-2xl w-full max-w-lg mx-4 flex flex-col max-h-[80vh]">
        <div className="p-4 border-b border-neutral-200 dark:border-neutral-800">
          <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100 mb-3">
            Aktivität auswählen
          </h2>
          <input
            type="text"
            autoFocus
            placeholder="Aktivität suchen…"
            value={query}
            onChange={(e) => handleQueryChange(e.target.value)}
            className="w-full px-3 py-2 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600
                       bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/50 focus:border-accent-DEFAULT"
          />
        </div>

        <div className="overflow-y-auto flex-1">
          {debouncedQuery.length < 2 ? (
            <p className="px-4 py-8 text-center text-sm text-neutral-400">
              Mindestens 2 Zeichen eingeben
            </p>
          ) : isLoading ? (
            <p className="px-4 py-8 text-center text-sm text-neutral-400">Suche…</p>
          ) : activities.length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-neutral-400">
              Keine Aktivitäten gefunden
            </p>
          ) : (
            <ul className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {activities.map((activity) => (
                <li key={activity.id}>
                  <button
                    className="w-full text-left px-4 py-3 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
                    onClick={() => onSelect(activity)}
                  >
                    <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                      {activity.title ?? '(Ohne Titel)'}
                    </p>
                    <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                      {activity.sport_type}
                      {activity.started_at
                        ? ` · ${new Date(activity.started_at).toLocaleDateString('de-DE')}`
                        : ''}
                    </p>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="p-3 border-t border-neutral-200 dark:border-neutral-800 flex justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                       cursor-pointer transition-colors"
          >
            Abbrechen
          </button>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Queue item card
// ---------------------------------------------------------------------------

interface QueueCardProps {
  item: ImportQueueItem
  onAssign: (activityId: string) => void
  onDismiss: () => void
  isAssigning: boolean
  isDismissing: boolean
}

function QueueCard({ item, onAssign, onDismiss, isAssigning, isDismissing }: QueueCardProps) {
  const [showSearch, setShowSearch] = useState(false)

  const handleSelect = (activity: Activity) => {
    setShowSearch(false)
    onAssign(activity.id)
  }

  const dateStr = item.created_at
    ? new Date(item.created_at).toLocaleDateString('de-DE', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
    : null

  const typeLabel: Record<ImportQueueItem['item_type'], string> = {
    image_no_exif: 'Kein EXIF-Datum',
    image_ambiguous: 'Mehrdeutig',
    image_no_match: 'Foto ohne Zuordnung',
    pdf_untyped: 'PDF',
    duplicate: 'Duplikat',
    import_error: 'Fehler',
  }

  return (
    <>
      {showSearch && (
        <ActivitySearchModal
          onSelect={handleSelect}
          onClose={() => setShowSearch(false)}
        />
      )}

      <div className="flex items-start gap-4 p-4 rounded-xl border border-neutral-200 dark:border-neutral-800
                      bg-white dark:bg-neutral-900">
        {/* Thumbnail */}
        <div className="flex-shrink-0 w-[60px] h-[45px] rounded-lg overflow-hidden bg-neutral-100 dark:bg-neutral-800
                        flex items-center justify-center">
          {item.item_type !== 'pdf_untyped' && item.item_type !== 'import_error' ? (
            <AuthenticatedImage
              src={`/api/v1/import-queue/${item.id}/thumb`}
              alt={item.filename}
              className="w-full h-full object-cover"
            />
          ) : (
            <PhotoPlaceholderIcon />
          )}
        </div>

        {/* Info */}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
            {item.filename}
          </p>
          <div className="flex items-center gap-2 mt-0.5 flex-wrap">
            <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium
                             bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
              {typeLabel[item.item_type] ?? item.item_type}
            </span>
            {dateStr && (
              <span className="text-xs text-neutral-400">{dateStr}</span>
            )}
            {item.file_size != null && (
              <span className="text-xs text-neutral-400">{formatBytes(item.file_size)}</span>
            )}
          </div>
          {item.metadata?.error && (
            <p className="mt-1 text-xs text-red-500 dark:text-red-400 line-clamp-2">
              {item.metadata.error}
            </p>
          )}
          {item.metadata?.duplicate_of_title && (
            <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
              Mögliches Duplikat von: {item.metadata.duplicate_of_title}
            </p>
          )}
        </div>

        {/* Actions */}
        <div className="flex-shrink-0 flex items-center gap-2">
          <button
            onClick={onDismiss}
            disabled={isDismissing || isAssigning}
            className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800
                       disabled:opacity-50 cursor-pointer transition-colors"
          >
            Verwerfen
          </button>
          <button
            onClick={() => setShowSearch(true)}
            disabled={isAssigning || isDismissing}
            className="px-3 py-1.5 text-sm rounded-lg text-white hover:opacity-90
                       disabled:opacity-50 transition-colors"
            style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
          >
            {isAssigning ? 'Zuordnen…' : 'Aktivität zuordnen'}
          </button>
        </div>
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function ImportQueuePage() {
  const qc = useQueryClient()
  const { addToast } = useToast()
  const [assigningId, setAssigningId] = useState<string | null>(null)
  const [dismissingId, setDismissingId] = useState<string | null>(null)

  const { data: items = [], isLoading } = useQuery({
    queryKey: ['import-queue', 'pending'],
    queryFn: () => fetchImportQueue('pending'),
  })

  const assignMutation = useMutation({
    mutationFn: ({ id, activityId }: { id: string; activityId: string }) =>
      assignQueueItem(id, activityId),
    onMutate: ({ id }) => setAssigningId(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['import-queue'] })
      addToast({ type: 'success', title: 'Foto zugeordnet' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Zuordnung fehlgeschlagen', duration: 0 })
    },
    onSettled: () => setAssigningId(null),
  })

  const dismissMutation = useMutation({
    mutationFn: (id: string) => patchQueueItem(id, { status: 'dismissed' }),
    onMutate: (id) => setDismissingId(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['import-queue'] })
      addToast({ type: 'info', title: 'Eintrag verworfen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Verwerfen fehlgeschlagen', duration: 0 })
    },
    onSettled: () => setDismissingId(null),
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Nicht zugeordnete Fotos
        </h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Importierte Fotos ohne Aktivitätszuordnung manuell einer Aktivität zuweisen oder verwerfen.
        </p>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-16 text-neutral-400 text-sm">Lade…</div>
      ) : items.length === 0 ? (
        <div className="text-center py-16 text-neutral-400">
          <div className="flex justify-center mb-3">
            <PhotoPlaceholderIcon size={40} />
          </div>
          <p className="text-lg font-medium">Keine wartenden Fotos</p>
          <p className="text-sm mt-1">Alle importierten Fotos wurden bereits zugeordnet.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {items.map((item) => (
            <QueueCard
              key={item.id}
              item={item}
              onAssign={(activityId) => assignMutation.mutate({ id: item.id, activityId })}
              onDismiss={() => dismissMutation.mutate(item.id)}
              isAssigning={assigningId === item.id}
              isDismissing={dismissingId === item.id}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function PhotoPlaceholderIcon({ size = 20 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="text-neutral-400"
    >
      <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
      <circle cx="8.5" cy="8.5" r="1.5" />
      <polyline points="21 15 16 10 5 21" />
    </svg>
  )
}
