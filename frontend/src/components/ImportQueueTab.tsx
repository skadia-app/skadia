import { useState, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  fetchImportQueue,
  assignQueueItem,
  patchQueueItem,
  deleteQueueItem,
  type ImportQueueItem,
} from '@/api/importQueue'
import { fetchActivities } from '@/api/activities'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes?: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function itemTypeLabel(type: ImportQueueItem['item_type']): string {
  const labels: Record<ImportQueueItem['item_type'], string> = {
    image_no_exif: 'Bild ohne GPS-Datum',
    image_ambiguous: 'Bild – mehrere Aktivitäten',
    pdf_untyped: 'PDF ohne Kategorie',
    duplicate: 'Mögliches Duplikat',
    import_error: 'Importfehler',
  }
  return labels[type] ?? type
}

function itemTypeBadgeColor(type: ImportQueueItem['item_type']): string {
  const colors: Record<ImportQueueItem['item_type'], string> = {
    image_no_exif: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    image_ambiguous: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    pdf_untyped: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300',
    duplicate: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    import_error: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  }
  return colors[type] ?? 'bg-neutral-100 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300'
}

// ---------------------------------------------------------------------------
// Activity search dropdown (shared)
// ---------------------------------------------------------------------------

interface ActivitySearchDropdownProps {
  value: string
  onChange: (activityId: string, activityTitle: string) => void
  placeholder?: string
}

function ActivitySearchDropdown({ value, onChange, placeholder }: ActivitySearchDropdownProps) {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)

  const { data } = useQuery({
    queryKey: ['activities-search', query],
    queryFn: () => fetchActivities(undefined, 10, 'started_at', 'desc', query ? { search: query } : undefined),
    staleTime: 30_000,
  })

  const activities = data?.activities ?? []

  return (
    <div className="relative">
      <input
        type="text"
        value={query || value}
        onChange={e => { setQuery(e.target.value); setOpen(true) }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        placeholder={placeholder ?? 'Aktivität suchen…'}
        className="w-full rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
      />
      {open && activities.length > 0 && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-lg overflow-hidden">
          {activities.map(a => (
            <button
              key={a.id}
              type="button"
              onMouseDown={() => {
                onChange(a.id, a.title ?? a.id)
                setQuery('')
                setOpen(false)
              }}
              className="w-full px-3 py-2 text-left text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800 text-neutral-800 dark:text-neutral-200 truncate"
            >
              {a.title ?? 'Unbenannte Aktivität'}
              <span className="ml-2 text-xs text-neutral-400">
                {a.started_at ? new Date(a.started_at).toLocaleDateString('de-DE') : ''}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Individual item renderers
// ---------------------------------------------------------------------------

interface ItemProps {
  item: ImportQueueItem
  onRefresh: () => void
}

function ImageNoExifItem({ item, onRefresh }: ItemProps) {
  const [selectedActivityId, setSelectedActivityId] = useState('')
  const [selectedActivityTitle, setSelectedActivityTitle] = useState('')

  const assignMutation = useMutation({
    mutationFn: () => assignQueueItem(item.id, selectedActivityId),
    onSuccess: onRefresh,
  })

  const dismissMutation = useMutation({
    mutationFn: () => deleteQueueItem(item.id),
    onSuccess: onRefresh,
  })

  return (
    <div className="space-y-3">
      <div className="flex items-start gap-3">
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">{item.filename}</p>
          {item.file_size && (
            <p className="text-xs text-neutral-400">{formatBytes(item.file_size)}</p>
          )}
        </div>
      </div>
      <ActivitySearchDropdown
        value={selectedActivityTitle}
        onChange={(id, title) => { setSelectedActivityId(id); setSelectedActivityTitle(title) }}
        placeholder="Aktivität auswählen…"
      />
      <div className="flex gap-2">
        <button
          onClick={() => assignMutation.mutate()}
          disabled={!selectedActivityId || assignMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {assignMutation.isPending ? 'Wird zugeordnet…' : 'Zuordnen'}
        </button>
        <button
          onClick={() => dismissMutation.mutate()}
          disabled={dismissMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        >
          Verwerfen
        </button>
      </div>
      {assignMutation.isError && (
        <p className="text-xs text-red-600 dark:text-red-400">
          {assignMutation.error instanceof Error ? assignMutation.error.message : 'Fehler beim Zuordnen'}
        </p>
      )}
    </div>
  )
}

function ImageAmbiguousItem({ item, onRefresh }: ItemProps) {
  const [selectedActivityId, setSelectedActivityId] = useState('')
  const [selectedActivityTitle, setSelectedActivityTitle] = useState('')

  const candidates = item.metadata?.ambiguous_candidates ?? []

  const assignMutation = useMutation({
    mutationFn: () => assignQueueItem(item.id, selectedActivityId),
    onSuccess: onRefresh,
  })

  const dismissMutation = useMutation({
    mutationFn: () => deleteQueueItem(item.id),
    onSuccess: onRefresh,
  })

  return (
    <div className="space-y-3">
      <div>
        <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">{item.filename}</p>
        {item.file_size && (
          <p className="text-xs text-neutral-400">{formatBytes(item.file_size)}</p>
        )}
      </div>
      {candidates.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs text-neutral-500 dark:text-neutral-400">Mögliche Aktivitäten:</p>
          {candidates.map(c => (
            <button
              key={c.activity_id}
              type="button"
              onClick={() => { setSelectedActivityId(c.activity_id); setSelectedActivityTitle(c.activity_title) }}
              className={`w-full text-left px-3 py-1.5 rounded-lg text-xs transition-colors ${
                selectedActivityId === c.activity_id
                  ? 'bg-accent-DEFAULT/10 border border-accent-DEFAULT text-accent-DEFAULT'
                  : 'border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 text-neutral-700 dark:text-neutral-300'
              }`}
            >
              {c.activity_title}
              <span className="ml-2 text-neutral-400">
                {new Date(c.started_at).toLocaleDateString('de-DE')}
              </span>
            </button>
          ))}
        </div>
      )}
      {candidates.length === 0 && (
        <ActivitySearchDropdown
          value={selectedActivityTitle}
          onChange={(id, title) => { setSelectedActivityId(id); setSelectedActivityTitle(title) }}
        />
      )}
      <div className="flex gap-2">
        <button
          onClick={() => assignMutation.mutate()}
          disabled={!selectedActivityId || assignMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {assignMutation.isPending ? 'Wird zugeordnet…' : 'Zuordnen'}
        </button>
        <button
          onClick={() => dismissMutation.mutate()}
          disabled={dismissMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        >
          Verwerfen
        </button>
      </div>
    </div>
  )
}

const PDF_TYPES = [
  { value: 'gear_document', label: 'Gear-Dokument' },
  { value: 'invoice', label: 'Rechnung' },
  { value: 'other', label: 'Sonstiges' },
] as const

type PdfType = typeof PDF_TYPES[number]['value']

function PdfUntypedItem({ item, onRefresh }: ItemProps) {
  const [pdfType, setPdfType] = useState<PdfType>('other')
  const [gearId, setGearId] = useState('')

  const categorizeMutation = useMutation({
    mutationFn: () => patchQueueItem(item.id, {
      status: 'resolved',
      pdf_type: pdfType,
      gear_id: pdfType === 'gear_document' ? gearId : undefined,
    }),
    onSuccess: onRefresh,
  })

  const dismissMutation = useMutation({
    mutationFn: () => deleteQueueItem(item.id),
    onSuccess: onRefresh,
  })

  return (
    <div className="space-y-3">
      <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">{item.filename}</p>
      <div className="flex gap-2 flex-wrap">
        {PDF_TYPES.map(t => (
          <button
            key={t.value}
            type="button"
            onClick={() => setPdfType(t.value)}
            className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${
              pdfType === t.value
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      {pdfType === 'gear_document' && (
        <input
          type="text"
          value={gearId}
          onChange={e => setGearId(e.target.value)}
          placeholder="Ausrüstungs-ID (optional)"
          className="w-full rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-3 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
        />
      )}
      <div className="flex gap-2">
        <button
          onClick={() => categorizeMutation.mutate()}
          disabled={categorizeMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {categorizeMutation.isPending ? 'Wird gespeichert…' : 'Kategorisieren'}
        </button>
        <button
          onClick={() => dismissMutation.mutate()}
          disabled={dismissMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        >
          Verwerfen
        </button>
      </div>
    </div>
  )
}

function DuplicateItem({ item, onRefresh }: ItemProps) {
  const dismissMutation = useMutation({
    mutationFn: () => patchQueueItem(item.id, { status: 'dismissed' }),
    onSuccess: onRefresh,
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteQueueItem(item.id),
    onSuccess: onRefresh,
  })

  return (
    <div className="space-y-3">
      <div>
        <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">{item.filename}</p>
        {item.metadata?.duplicate_of_title && (
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Mögliches Duplikat von{' '}
            {item.metadata.duplicate_of ? (
              <Link
                to={`/activities/${item.metadata.duplicate_of}`}
                className="text-accent-DEFAULT hover:underline"
              >
                {item.metadata.duplicate_of_title}
              </Link>
            ) : (
              <span>{item.metadata.duplicate_of_title}</span>
            )}
          </p>
        )}
      </div>
      <div className="flex gap-2 flex-wrap">
        <button
          onClick={() => dismissMutation.mutate()}
          disabled={dismissMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 transition-colors"
        >
          Behalten
        </button>
        <button
          onClick={() => deleteMutation.mutate()}
          disabled={deleteMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 border border-red-200 dark:border-red-800 transition-colors"
        >
          {deleteMutation.isPending ? 'Wird gelöscht…' : 'Löschen'}
        </button>
      </div>
    </div>
  )
}

function ImportErrorItem({ item, onRefresh }: ItemProps) {
  const retryMutation = useMutation({
    mutationFn: () => patchQueueItem(item.id, { status: 'pending', retry: true }),
    onSuccess: onRefresh,
  })

  const dismissMutation = useMutation({
    mutationFn: () => deleteQueueItem(item.id),
    onSuccess: onRefresh,
  })

  return (
    <div className="space-y-3">
      <div>
        <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">{item.filename}</p>
        {item.metadata?.error && (
          <p className="text-xs text-red-600 dark:text-red-400 mt-0.5 break-words">{item.metadata.error}</p>
        )}
      </div>
      <div className="flex gap-2">
        <button
          onClick={() => retryMutation.mutate()}
          disabled={retryMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {retryMutation.isPending ? 'Wird gestartet…' : 'Erneut versuchen'}
        </button>
        <button
          onClick={() => dismissMutation.mutate()}
          disabled={dismissMutation.isPending}
          className="px-3 py-1.5 rounded-lg text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        >
          Verwerfen
        </button>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Queue item card
// ---------------------------------------------------------------------------

function QueueItemCard({ item, onRefresh }: ItemProps) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4 space-y-3">
      <div className="flex items-center gap-2">
        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-medium ${itemTypeBadgeColor(item.item_type)}`}>
          {itemTypeLabel(item.item_type)}
        </span>
        <span className="text-xs text-neutral-400 ml-auto">
          {new Date(item.created_at).toLocaleDateString('de-DE')}
        </span>
      </div>

      {item.item_type === 'image_no_exif' && (
        <ImageNoExifItem item={item} onRefresh={onRefresh} />
      )}
      {item.item_type === 'image_ambiguous' && (
        <ImageAmbiguousItem item={item} onRefresh={onRefresh} />
      )}
      {item.item_type === 'pdf_untyped' && (
        <PdfUntypedItem item={item} onRefresh={onRefresh} />
      )}
      {item.item_type === 'duplicate' && (
        <DuplicateItem item={item} onRefresh={onRefresh} />
      )}
      {item.item_type === 'import_error' && (
        <ImportErrorItem item={item} onRefresh={onRefresh} />
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main export
// ---------------------------------------------------------------------------

export default function ImportQueueTab() {
  const queryClient = useQueryClient()

  const { data: items = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['import-queue'],
    queryFn: () => fetchImportQueue('pending'),
    staleTime: 30_000,
  })

  const onRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['import-queue'] })
    queryClient.invalidateQueries({ queryKey: ['import-queue-count'] })
  }, [queryClient])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12 text-neutral-400 text-sm">
        Lade Warteschlange…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center justify-center py-12 text-red-500 text-sm">
        Fehler beim Laden der Warteschlange.
        <button onClick={() => refetch()} className="ml-2 underline">Erneut versuchen</button>
      </div>
    )
  }

  if (items.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-neutral-400 space-y-2">
        <svg className="w-10 h-10" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="1.5">
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p className="text-sm font-medium">Keine offenen Einträge</p>
        <p className="text-xs">Alle Dateien wurden verarbeitet.</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-neutral-500 dark:text-neutral-400">
        {items.length} {items.length === 1 ? 'Eintrag' : 'Einträge'} warten auf Bearbeitung.
      </p>
      {items.map(item => (
        <QueueItemCard key={item.id} item={item} onRefresh={onRefresh} />
      ))}
    </div>
  )
}
