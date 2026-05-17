import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { useQuery } from '@tanstack/react-query'
import {
  Folder,
  Upload,
  Activity,
  Map,
  Loader2,
  CheckCircle2,
  XCircle,
  SkipForward,
  Clock,
  ChevronDown,
  ChevronUp,
  ArrowDownToLine,
  ExternalLink,
  Inbox,
} from 'lucide-react'
import { useImportQueue, useImportHistory } from '@/hooks/useImportLog'
import type { ImportLogEntry } from '@/api/imports'
import { fetchImportQueueCount } from '@/api/importQueue'
import { ImportUploadPanel } from '@/components/ImportHub'
import ImportQueueTab from '@/components/ImportQueueTab'
import { activeImportStepFromSteps } from '@/lib/importSteps'
import { ImportSourcePanel, type ImportSource } from '@/pages/ImportPage'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type ViewId = 'import' | 'review' | 'queue' | 'history'

const VIEWS: Array<{ id: ViewId; label: string }> = [
  { id: 'import', label: 'Importieren' },
  { id: 'review', label: 'Zu prüfen' },
  { id: 'queue', label: 'Warteschlange' },
  { id: 'history', label: 'Verlauf' },
]

const IMPORT_SOURCES: Array<{ id: ImportSource; label: string; description: string }> = [
  {
    id: 'garmin',
    label: 'Garmin',
    description: 'Export-ZIP oder freigegebenes Export-Verzeichnis importieren.',
  },
  {
    id: 'strava',
    label: 'Strava',
    description: 'Strava-Archiv hochladen oder aus einem lokalen Export lesen.',
  },
  {
    id: 'komoot',
    label: 'Komoot',
    description: 'Aufgezeichnete und geplante Touren über den bestehenden Wizard importieren.',
  },
]

function parseView(value: string | null): ViewId | null {
  return value === 'import' || value === 'review' || value === 'queue' || value === 'history'
    ? value
    : null
}

function parseSource(value: string | null): ImportSource | null {
  return value === 'garmin' || value === 'strava' || value === 'komoot' ? value : null
}

const IMPORT_ERROR_TRANSLATIONS: Record<string, string> = {
  'processing timeout — server may have restarted':
    'Verarbeitungs-Timeout — Server wurde möglicherweise neu gestartet',
}

function translateImportError(msg: string): string {
  return IMPORT_ERROR_TRANSLATIONS[msg] ?? msg
}

function elapsedLabel(createdAt: string): string {
  const secs = Math.floor((Date.now() - new Date(createdAt).getTime()) / 1000)
  if (secs < 60) return `vor ${secs}s`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `vor ${mins}m`
  return `vor ${Math.floor(mins / 60)}h`
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

// ---------------------------------------------------------------------------
// Platform badge config
// ---------------------------------------------------------------------------

interface PlatformConfig {
  icon: React.ReactNode
  label: string
  badgeClass: string
}

function getPlatformConfig(platform: string): PlatformConfig {
  const iconSize = 'w-3.5 h-3.5'
  switch (platform) {
    case 'hotfolder':
      return {
        icon: <Folder className={iconSize} />,
        label: 'Hotfolder',
        badgeClass:
          'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
      }
    case 'upload':
      return {
        icon: <Upload className={iconSize} />,
        label: 'Upload',
        badgeClass:
          'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
      }
    case 'garmin':
      return {
        icon: <Activity className={iconSize} />,
        label: 'Garmin',
        badgeClass:
          'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
      }
    case 'strava':
      return {
        icon: <Activity className={iconSize} />,
        label: 'Strava',
        badgeClass:
          'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
      }
    case 'komoot':
      return {
        icon: <Map className={iconSize} />,
        label: 'Komoot',
        badgeClass:
          'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
      }
    default:
      return {
        icon: <ArrowDownToLine className={iconSize} />,
        label: platform,
        badgeClass:
          'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400',
      }
  }
}

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

function StatusBadge({ status, linkQueued = false }: { status: string; linkQueued?: boolean }) {
  switch (status) {
    case 'pending':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400">
          <Clock className="w-3 h-3" />
          Wartend
        </span>
      )
    case 'processing':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
          <Loader2 className="w-3 h-3 animate-spin" />
          Verarbeitung
        </span>
      )
    case 'completed':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
          <CheckCircle2 className="w-3 h-3" />
          Fertig
        </span>
      )
    case 'skipped':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
          <SkipForward className="w-3 h-3" />
          Duplikat
        </span>
      )
    case 'error':
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
          <XCircle className="w-3 h-3" />
          Fehler
        </span>
      )
    case 'queued':
      const badge = (
        <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
          <Inbox className="h-3 w-3" />
          Zu prüfen
        </span>
      )
      return linkQueued ? (
        <Link to="/imports?view=review" className="inline-flex hover:opacity-80">
          {badge}
        </Link>
      ) : badge
    default:
      return (
        <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400">
          {status}
        </span>
      )
  }
}

function activeImportStep(entry: ImportLogEntry): { current: number; total: number; label: string } | null {
  return activeImportStepFromSteps(entry.metadata?.steps)
}

// ---------------------------------------------------------------------------
// Platform badge (compact, icon + label)
// ---------------------------------------------------------------------------

function PlatformBadge({ platform }: { platform: string }) {
  const cfg = getPlatformConfig(platform)
  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${cfg.badgeClass}`}
    >
      {cfg.icon}
      {cfg.label}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Activity cell
// ---------------------------------------------------------------------------

function ActivityCell({ entry }: { entry: ImportLogEntry }) {
  const isMonitoring =
    entry.metadata && entry.metadata['data_type'] === 'monitoring'

  if (isMonitoring) {
    return (
      <span className="text-xs text-neutral-500 dark:text-neutral-400 italic">
        Monitoring-Daten
      </span>
    )
  }

  if (entry.activity_id) {
    return (
      <Link
        to={`/activities/${entry.activity_id}`}
        className="inline-flex items-center gap-1 text-xs text-accent-DEFAULT hover:text-accent-600 transition-colors"
      >
        <ExternalLink className="w-3 h-3" />
        Öffnen
      </Link>
    )
  }

  return <span className="text-xs text-neutral-400">—</span>
}

// ---------------------------------------------------------------------------
// Queue section
// ---------------------------------------------------------------------------

function QueueSection() {
  const { data, isLoading } = useImportQueue()
  const entries = data?.entries ?? []

  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Import-Warteschlange
        </h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
          Aktuell verarbeitete und wartende Imports — aktualisiert alle 5 Sekunden.
        </p>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-16 rounded-xl bg-neutral-100 dark:bg-neutral-800 animate-pulse"
            />
          ))}
        </div>
      ) : entries.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-neutral-400 dark:text-neutral-500">
          <Loader2 className="w-8 h-8 mb-3 opacity-30" />
          <p className="text-sm">Keine aktiven Imports</p>
        </div>
      ) : (
        <div className="space-y-3">
          {entries.map((entry) => (
            <QueueCard key={entry.id} entry={entry} />
          ))}
        </div>
      )}
    </section>
  )
}

function QueueCard({ entry }: { entry: ImportLogEntry }) {
  const cfg = getPlatformConfig(entry.source_platform)
  const step = activeImportStep(entry)

  return (
    <div className="flex items-center gap-4 p-4 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      {/* Platform icon */}
      <div
        className={`flex-shrink-0 w-9 h-9 rounded-lg flex items-center justify-center ${cfg.badgeClass}`}
      >
        {cfg.icon}
      </div>

      {/* Info */}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-mono text-neutral-800 dark:text-neutral-200 truncate">
          {entry.filename}
        </p>
        <div className="mt-1">
          <StatusBadge status={entry.status} />
        </div>
        {step && (
          <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
            Schritt {step.current}/{step.total}: {step.label}
          </p>
        )}
      </div>

      {/* Elapsed */}
      <div className="flex-shrink-0 text-xs text-neutral-400 dark:text-neutral-500">
        {elapsedLabel(entry.created_at)}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// History section
// ---------------------------------------------------------------------------

const PAGE_SIZE = 50

function HistorySection() {
  const [sourcePlatformFilter, setSourcePlatformFilter] = useState('')
  const [importTypeFilter, setImportTypeFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [offset, setOffset] = useState(0)
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())

  const params = {
    source_platform: sourcePlatformFilter || undefined,
    import_type: importTypeFilter || undefined,
    status: statusFilter || undefined,
    limit: PAGE_SIZE,
    offset,
  }

  const { data, isLoading } = useImportHistory(params)
  const entries = data?.entries ?? []
  const total = data?.total ?? 0

  const handleSourceChange = (v: string) => {
    setSourcePlatformFilter(v)
    setOffset(0)
  }

  const handleStatusChange = (v: string) => {
    setStatusFilter(v)
    setOffset(0)
  }

  const handleTypeChange = (v: string) => {
    setImportTypeFilter(v)
    setOffset(0)
  }

  const toggleError = (id: string) => {
    setExpandedErrors((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const pageStart = total === 0 ? 0 : offset + 1
  const pageEnd = Math.min(offset + PAGE_SIZE, total)

  const selectClass =
    'px-3 py-1.5 text-sm rounded-lg border border-neutral-300 dark:border-neutral-700 ' +
    'bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 ' +
    'focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40'

  return (
    <section className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
          Import-Verlauf
        </h2>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-3 items-center">
        <select
          value={sourcePlatformFilter}
          onChange={(e) => handleSourceChange(e.target.value)}
          className={selectClass}
          aria-label="Quelle filtern"
        >
          <option value="">Alle Quellen</option>
          <option value="hotfolder">Hotfolder</option>
          <option value="upload">Upload</option>
          <option value="url">URL</option>
          <option value="garmin">Garmin</option>
          <option value="strava">Strava</option>
          <option value="komoot">Komoot</option>
          <option value="gdpr_import">DSGVO-Import</option>
        </select>

        <select
          value={importTypeFilter}
          onChange={(e) => handleTypeChange(e.target.value)}
          className={selectClass}
          aria-label="Typ filtern"
        >
          <option value="">Alle Typen</option>
          <option value="activity">Aktivität</option>
          <option value="route">Route</option>
          <option value="workout">Workout</option>
          <option value="monitoring">Monitoring</option>
          <option value="photo">Foto</option>
          <option value="video">Video</option>
          <option value="document">Dokument</option>
          <option value="gear">Gear</option>
          <option value="segment">Segment</option>
          <option value="archive">Archiv</option>
          <option value="unknown">Unbekannt</option>
        </select>

        <select
          value={statusFilter}
          onChange={(e) => handleStatusChange(e.target.value)}
          className={selectClass}
          aria-label="Status filtern"
        >
          <option value="">Alle Status</option>
          <option value="pending">Wartend</option>
          <option value="processing">Verarbeitung</option>
          <option value="completed">Fertig</option>
          <option value="skipped">Duplikat</option>
          <option value="queued">Zu prüfen</option>
          <option value="error">Fehler</option>
        </select>
      </div>

      {/* Table */}
      {isLoading && entries.length === 0 ? (
        <div className="space-y-2">
          {[0, 1, 2, 3, 4].map((i) => (
            <div
              key={i}
              className="h-10 rounded-lg bg-neutral-100 dark:bg-neutral-800 animate-pulse"
            />
          ))}
        </div>
      ) : entries.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-neutral-400 dark:text-neutral-500">
          <ArrowDownToLine className="w-10 h-10 mb-3 opacity-30" />
          <p className="text-base font-medium">Noch keine Imports</p>
          <p className="text-sm mt-1 opacity-75">
            Importierte Dateien werden hier angezeigt.
          </p>
        </div>
      ) : (
        <>
          <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium whitespace-nowrap">
                    Quelle
                  </th>
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium">
                    Datei
                  </th>
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium whitespace-nowrap">
                    Status
                  </th>
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium whitespace-nowrap">
                    Dauer
                  </th>
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium whitespace-nowrap">
                    Aktivität
                  </th>
                  <th className="text-left px-3 py-2.5 text-neutral-500 dark:text-neutral-400 font-medium whitespace-nowrap">
                    Datum
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
                {entries.map((entry) => (
                  <HistoryRow
                    key={entry.id}
                    entry={entry}
                    isExpanded={expandedErrors.has(entry.id)}
                    onToggleExpand={() => toggleError(entry.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between text-sm text-neutral-500 dark:text-neutral-400">
            <span>
              {pageStart}–{pageEnd} von {total} Einträgen
            </span>
            <div className="flex gap-2">
              <button
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700
                           hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-40 disabled:cursor-not-allowed
                           disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                Zurück
              </button>
              <button
                disabled={offset + PAGE_SIZE >= total}
                onClick={() => setOffset(offset + PAGE_SIZE)}
                className="px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700
                           hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-40 disabled:cursor-not-allowed
                           disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                Weiter
              </button>
            </div>
          </div>
        </>
      )}
    </section>
  )
}

// ---------------------------------------------------------------------------
// History row
// ---------------------------------------------------------------------------

interface HistoryRowProps {
  entry: ImportLogEntry
  isExpanded: boolean
  onToggleExpand: () => void
}

function HistoryRow({ entry, isExpanded, onToggleExpand }: HistoryRowProps) {
  const hasError = entry.status === 'error' && !!entry.error_msg

  return (
    <>
      <tr className="even:bg-neutral-50/50 dark:even:bg-neutral-800/20 hover:bg-neutral-50 dark:hover:bg-neutral-800/40 transition-colors">
        {/* Source */}
        <td className="px-3 py-2.5">
          <PlatformBadge platform={entry.source_platform} />
        </td>

        {/* Filename */}
        <td className="px-3 py-2.5 max-w-[200px]">
          <span className="font-mono text-xs text-neutral-800 dark:text-neutral-200 truncate block">
            {entry.filename}
          </span>
        </td>

        {/* Status */}
        <td className="px-3 py-2.5">
          <div className="flex items-center gap-1">
            <StatusBadge status={entry.status} linkQueued />
            {hasError && (
              <button
                onClick={onToggleExpand}
                aria-label={isExpanded ? 'Fehlermeldung ausblenden' : 'Fehlermeldung anzeigen'}
                className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors"
              >
                {isExpanded ? (
                  <ChevronUp className="w-3.5 h-3.5" />
                ) : (
                  <ChevronDown className="w-3.5 h-3.5" />
                )}
              </button>
            )}
          </div>
        </td>

        {/* Duration */}
        <td className="px-3 py-2.5 text-xs text-neutral-500 dark:text-neutral-400 whitespace-nowrap">
          {entry.duration_ms != null ? formatDuration(entry.duration_ms) : '—'}
        </td>

        {/* Activity */}
        <td className="px-3 py-2.5">
          <ActivityCell entry={entry} />
        </td>

        {/* Date */}
        <td className="px-3 py-2.5 text-xs text-neutral-500 dark:text-neutral-400 whitespace-nowrap">
          {format(new Date(entry.created_at), 'dd. MMM yyyy HH:mm', {
            locale: de,
          })}
        </td>
      </tr>

      {/* Expanded error row */}
      {hasError && isExpanded && (
        <tr>
          <td
            colSpan={6}
            className="px-4 py-3 bg-red-50 dark:bg-red-950/20 border-t border-red-100 dark:border-red-900/30"
          >
            <p className="text-xs text-red-700 dark:text-red-400 font-mono break-all">
              {translateImportError(entry.error_msg!)}
            </p>
          </td>
        </tr>
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ImportsPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const viewParam = parseView(searchParams.get('view'))
  const sourceParam = parseSource(searchParams.get('source'))
  const hasExplicitImportParam = searchParams.has('view') || searchParams.has('source')

  const { data: reviewCount = 0 } = useQuery({
    queryKey: ['import-queue-count'],
    queryFn: fetchImportQueueCount,
    staleTime: 30_000,
    retry: false,
  })

  const activeView: ViewId =
    viewParam ?? (sourceParam ? 'import' : !hasExplicitImportParam && reviewCount > 0 ? 'review' : 'import')
  const activeSource = activeView === 'import' ? sourceParam : null

  const setView = (view: ViewId) => {
    const next = new URLSearchParams(searchParams)
    next.set('view', view)
    next.delete('source')
    setSearchParams(next, { replace: false })
  }

  const setSource = (source: ImportSource) => {
    const next = new URLSearchParams(searchParams)
    next.set('view', 'import')
    next.set('source', source)
    setSearchParams(next, { replace: false })
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Imports
        </h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Importieren, offene Prüfungen, aktive Warteschlange und Verlauf an einem Ort.
        </p>
      </div>

      <div
        role="tablist"
        aria-label="Import-Ansichten"
        className="flex flex-wrap gap-1 border-b border-neutral-200 dark:border-neutral-800"
      >
        {VIEWS.map(view => (
          <button
            key={view.id}
            type="button"
            role="tab"
            aria-selected={activeView === view.id}
            onClick={() => setView(view.id)}
            className={`cursor-pointer relative px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px ${
              activeView === view.id
                ? 'border-accent-DEFAULT text-accent-DEFAULT'
                : 'border-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100'
            }`}
          >
            <span className="inline-flex items-center gap-1.5">
              {view.label}
              {view.id === 'review' && reviewCount > 0 && (
                <span className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-orange-500 px-1 text-[10px] font-bold leading-none text-white">
                  {reviewCount > 9 ? '9+' : reviewCount}
                </span>
              )}
            </span>
          </button>
        ))}
      </div>

      {activeView === 'import' && (
        <ImportStartSection
          activeSource={activeSource}
          onSelectSource={setSource}
          onGoToReview={() => setView('review')}
          onNavigate={navigate}
        />
      )}
      {activeView === 'review' && (
        <section className="space-y-6">
          <div>
            <h2 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
              Zu prüfen
            </h2>
            <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
              Importierte Dateien mit manueller Nacharbeit prüfen, zuordnen oder verwerfen.
            </p>
          </div>
          <ImportQueueTab />
        </section>
      )}
      {activeView === 'queue' && <QueueSection />}
      {activeView === 'history' && <HistorySection />}
    </div>
  )
}

function ImportStartSection({
  activeSource,
  onSelectSource,
  onGoToReview,
  onNavigate,
}: {
  activeSource: ImportSource | null
  onSelectSource: (source: ImportSource) => void
  onGoToReview: () => void
  onNavigate: (path: string) => void
}) {
  return (
    <div className="space-y-6">
      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Dateien importieren
          </h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            FIT, GPX, TCX, Bilder und PDFs direkt hochladen.
          </p>
        </div>
        <ImportUploadPanel
          onGoToQueue={onGoToReview}
          onNavigate={onNavigate}
        />
      </section>

      <section className="space-y-3">
        <div>
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Quellen
          </h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            Bestehende Import-Wizards direkt in dieser Oberfläche starten.
          </p>
        </div>

        <div className="grid gap-3 md:grid-cols-3">
          {IMPORT_SOURCES.map(source => (
            <button
              key={source.id}
              type="button"
              onClick={() => onSelectSource(source.id)}
              className={`cursor-pointer rounded-lg border p-4 text-left transition-colors ${
                activeSource === source.id
                  ? 'border-accent-DEFAULT bg-accent-DEFAULT/5'
                  : 'border-neutral-200 bg-white hover:border-neutral-300 dark:border-neutral-800 dark:bg-neutral-900 dark:hover:border-neutral-700'
              }`}
            >
              <span className="block text-sm font-semibold text-neutral-900 dark:text-neutral-100">
                {source.label}
              </span>
              <span className="mt-1 block text-xs leading-5 text-neutral-500 dark:text-neutral-400">
                {source.description}
              </span>
            </button>
          ))}
        </div>
      </section>

      {activeSource && (
        <section className="space-y-3">
          <div>
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
              {IMPORT_SOURCES.find(source => source.id === activeSource)?.label}-Import
            </h2>
          </div>
          {IMPORT_SOURCES.map(source => (
            <div
              key={source.id}
              className={activeSource === source.id ? undefined : 'hidden'}
            >
              <ImportSourcePanel source={source.id} />
            </div>
          ))}
        </section>
      )}
    </div>
  )
}
