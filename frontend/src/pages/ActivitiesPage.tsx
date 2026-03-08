import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchActivities, downloadActivitiesZip } from '@/api/activities'
import type { Activity } from '@/api/types'
import type { ExportFormat } from '@/api/activities'
import { apiDownload } from '@/api/client'
import { apiFetch } from '@/lib/auth'
import {
  formatDuration, formatDistance,
  formatElevationGain, formatElevationLoss, sportLabel, showIndoorBadge,
} from '@/lib/utils'
import { SportIcon } from '@/components/SportIcon'
import ActivityFilters, { paramsToFilters, filtersToParams, filtersToApiParams } from '@/components/ActivityFilters'
import type { ActivityFilterState } from '@/components/ActivityFilters'
import RecomputeModal from '@/components/RecomputeModal'
import DeleteConfirmModal from '@/components/DeleteConfirmModal'

const PAGE_SIZES = [20, 50, 100] as const

// 0 = Infinite-Scroll-Modus
type PageSize = 20 | 50 | 100 | 0

type SortColumn = 'started_at' | 'distance_m' | 'elevation_gain_m' | 'elapsed_time_s'
type SortDir = 'asc' | 'desc'

// ── sessionStorage-Schlüssel für Pagination-Persistenz ──────────────────────
const SS_CURSORS = 'skadia-activities-cursors'
const SS_LAST_ACTIVITY = 'skadia-last-activity-id'

function saveCursors(cursors: (string | undefined)[]) {
  try {
    sessionStorage.setItem(SS_CURSORS, JSON.stringify(cursors))
  } catch (_) { /* ignore */ }
}

function loadCursors(): (string | undefined)[] | null {
  try {
    const raw = sessionStorage.getItem(SS_CURSORS)
    if (!raw) return null
    return JSON.parse(raw) as (string | undefined)[]
  } catch (_) { return null }
}

function saveLastActivity(id: string) {
  try {
    sessionStorage.setItem(SS_LAST_ACTIVITY, id)
  } catch (_) { /* ignore */ }
}

function loadLastActivity(): string | null {
  try {
    return sessionStorage.getItem(SS_LAST_ACTIVITY)
  } catch (_) { return null }
}

function clearLastActivity() {
  try {
    sessionStorage.removeItem(SS_LAST_ACTIVITY)
  } catch (_) { /* ignore */ }
}

/**
 * Erzeugt einen Cursor für ein bestimmtes Datum.
 * Format: base64url(RFC3339Nano_timestamp + "|" + uuid)
 */
function buildCursorForDate(dateStr: string, direction: SortDir): string {
  const MAX_UUID = 'ffffffff-ffff-ffff-ffff-ffffffffffff'
  const MIN_UUID = '00000000-0000-0000-0000-000000000000'

  let raw: string
  if (direction === 'desc') {
    raw = dateStr + 'T23:59:59.999999999Z|' + MAX_UUID
  } else {
    raw = dateStr + 'T00:00:00Z|' + MIN_UUID
  }
  return btoa(raw)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
}

// ── Export-Untermenü (single activity) ──────────────────────────────────────
const EXPORT_FORMATS: [ExportFormat, string][] = [
  ['original', 'Original'],
  ['gpx', 'GPX'],
  ['tcx', 'TCX'],
  ['fit', 'FIT'],
]

function ExportSubMenu({ activityId, onClose }: { activityId: string; onClose: () => void }) {
  const [open, setOpen] = useState(false)

  const handleExport = async (format: ExportFormat) => {
    try {
      const { blob, filename } = await apiDownload(`/activities/${activityId}/export?format=${format}`)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename || `activity.${format}`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      onClose()
    } catch (error) {
      console.error('Export failed:', error)
    }
  }

  return (
    <div className="relative" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button className="w-full text-left px-3 py-2 text-sm flex items-center justify-between gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer">
        <span className="flex items-center gap-2">
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          Exportieren
        </span>
        <svg className="w-3 h-3 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
      {open && (
        <div className="absolute left-full top-0 ml-1 bg-white dark:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg min-w-[120px] z-50 py-1">
          {EXPORT_FORMATS.map(([fmt, label]) => (
            <button
              key={fmt}
              onClick={() => handleExport(fmt)}
              className="w-full text-left block px-3 py-2 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
            >
              {label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── ZIP-Untermenü (multi-select) ─────────────────────────────────────────────
function ZipSubMenu({
  ids,
  onZip,
  onClose,
  zipLoading,
}: {
  ids: string[]
  onZip: (ids: string[], format: ExportFormat) => void
  onClose: () => void
  zipLoading: boolean
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="relative" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button
        disabled={zipLoading}
        className="w-full text-left px-3 py-2 text-sm flex items-center justify-between gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer disabled:opacity-50"
      >
        <span className="flex items-center gap-2">
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          {zipLoading ? 'Lädt…' : 'Als ZIP herunterladen'}
        </span>
        <svg className="w-3 h-3 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
      {open && !zipLoading && (
        <div className="absolute left-full top-0 ml-1 bg-white dark:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg min-w-[120px] z-50 py-1">
          {EXPORT_FORMATS.map(([fmt, label]) => (
            <button
              key={fmt}
              onClick={() => { onZip(ids, fmt); onClose() }}
              className="w-full text-left block px-3 py-2 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
            >
              {label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── ZIP-Dropdown für Bulk-Aktionsleiste ──────────────────────────────────────
function ZipDropdown({
  ids,
  loading,
  onDownload,
}: {
  ids: string[]
  loading: boolean
  onDownload: (ids: string[], format: ExportFormat) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(prev => !prev)}
        disabled={loading}
        className="text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer flex items-center gap-1 disabled:opacity-50"
      >
        {loading ? 'Lädt…' : 'ZIP'}
        <svg className="w-3 h-3 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 left-0 bg-white dark:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg min-w-[120px] z-30 py-1">
          {EXPORT_FORMATS.map(([fmt, label]) => (
            <button
              key={fmt}
              onClick={() => { onDownload(ids, fmt); setOpen(false) }}
              className="w-full text-left px-3 py-2 text-xs text-neutral-700 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
            >
              {label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Dialog: "Alle in neuen Tabs öffnen" ──────────────────────────────────────
// Browsers block window.open() in loops after the first call (popup blocker).
// We show a dialog with real <a target="_blank"> links instead so the user
// can open each tab with a single deliberate click.
interface OpenTabsItem { id: string; title: string | null }

function OpenTabsDialog({ items, onClose }: { items: OpenTabsItem[]; onClose: () => void }) {
  useEffect(() => {
    function handleKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onMouseDown={onClose}
    >
      <div
        className="bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-xl shadow-2xl w-full max-w-sm mx-4 overflow-hidden"
        onMouseDown={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-100 dark:border-neutral-800">
          <span className="text-sm font-semibold text-neutral-800 dark:text-neutral-100">
            {items.length} Aktivitäten öffnen
          </span>
          <button
            onClick={onClose}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <p className="px-4 pt-3 pb-1 text-xs text-neutral-500 dark:text-neutral-400">
          Browser-Popup-Blocker verhindern das automatische Öffnen mehrerer Tabs. Klicke jeden Link einzeln an.
        </p>
        <ul className="overflow-y-auto max-h-72 py-2 divide-y divide-neutral-100 dark:divide-neutral-800">
          {items.map(({ id, title }) => (
            <li key={id}>
              <a
                href={`/activities/${id}`}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-3 px-4 py-2.5 text-sm text-neutral-700 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-800 no-underline transition-colors"
              >
                <svg className="w-3.5 h-3.5 flex-shrink-0 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                </svg>
                <span className="truncate">
                  {title ?? <span className="text-neutral-400 font-mono text-xs">{id.slice(0, 8)}&hellip;</span>}
                </span>
              </a>
            </li>
          ))}
        </ul>
        <div className="px-4 py-3 border-t border-neutral-100 dark:border-neutral-800 flex justify-end">
          <button
            onClick={onClose}
            className="text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
          >
            Schließen
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Kontext-Menü-Komponente ──────────────────────────────────────────────────
interface ContextMenuState {
  x: number
  y: number
  activityId: string
  activityTitle?: string | null
  sportType?: string
}

interface ContextMenuProps {
  state: ContextMenuState
  selectedIds: Set<string>
  zipLoading: boolean
  onClose: () => void
  onRecompute: (ids: string[]) => void
  onDelete: (ids: string[], title?: string | null) => void
  onZip: (ids: string[], format: ExportFormat) => void
  onOpenTabs: (items: OpenTabsItem[]) => void
  onGroup: (ids: string[]) => void
  onUngroup: (id: string) => void
}

function ActivityContextMenu({
  state,
  selectedIds,
  zipLoading,
  onClose,
  onRecompute,
  onDelete,
  onZip,
  onOpenTabs,
  onGroup,
  onUngroup,
}: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClick() { onClose() }
    function handleKeyDown(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [onClose])

  const style: React.CSSProperties = {
    position: 'fixed',
    top: state.y,
    left: state.x,
    zIndex: 9999,
  }

  const isMulti = selectedIds.size > 1 && selectedIds.has(state.activityId)
  const ids = isMulti ? [...selectedIds] : [state.activityId]
  const count = ids.length

  const menuClass = 'bg-white dark:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg py-1 min-w-[200px] text-sm'

  if (isMulti) {
    return (
      <div
        ref={menuRef}
        style={style}
        onMouseDown={e => e.stopPropagation()}
        className={menuClass}
      >
        <div className="px-3 py-2 text-xs text-neutral-400 dark:text-neutral-500 border-b border-neutral-100 dark:border-neutral-700 font-medium">
          {count} ausgewählt
        </div>
        <button
          onClick={() => { onOpenTabs(ids.map(id => ({ id, title: null }))); onClose() }}
          className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
          </svg>
          Alle in neuen Tabs öffnen
        </button>
        <ZipSubMenu ids={ids} onZip={onZip} onClose={onClose} zipLoading={zipLoading} />
        <button
          onClick={() => { onGroup(ids); onClose() }}
          className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          Als Multisport gruppieren
        </button>
        <button
          onClick={() => { onRecompute(ids); onClose() }}
          className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Neu berechnen
        </button>
        <div className="border-t border-neutral-100 dark:border-neutral-700 my-1" />
        <button
          onClick={() => { onDelete(ids); onClose() }}
          className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          {count} Aktivitäten löschen
        </button>
      </div>
    )
  }

  // Single activity menu
  return (
    <div
      ref={menuRef}
      style={style}
      onMouseDown={e => e.stopPropagation()}
      className={menuClass}
    >
      <a
        href={`/activities/${state.activityId}`}
        target="_blank"
        rel="noopener noreferrer"
        onClick={onClose}
        className="flex items-center gap-2 px-3 py-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer no-underline"
      >
        <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
        </svg>
        In neuem Tab öffnen
      </a>
      <ExportSubMenu activityId={state.activityId} onClose={onClose} />
      <button
        onClick={() => { onRecompute([state.activityId]); onClose() }}
        className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
      >
        <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        Neu berechnen
      </button>
      {state.sportType === 'multisport' && (
        <button
          onClick={() => { onUngroup(state.activityId); onClose() }}
          className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          Multisport-Gruppe auflösen
        </button>
      )}
      <div className="border-t border-neutral-100 dark:border-neutral-700 my-1" />
      <button
        onClick={() => { onDelete([state.activityId], state.activityTitle); onClose() }}
        className="w-full text-left px-3 py-2 text-sm flex items-center gap-2 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors cursor-pointer"
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
        </svg>
        Löschen
      </button>
    </div>
  )
}

export default function ActivitiesPage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  // ── Seitenposition: aus URL (page) + sessionStorage (cursors) wiederherstellen ──
  const initialPageFromUrl = Math.max(0, (parseInt(searchParams.get('page') ?? '1', 10) || 1) - 1)

  const [pageSize, setPageSize] = useState<PageSize>(20)
  const [cursors, setCursors] = useState<(string | undefined)[]>(() => {
    const stored = loadCursors()
    if (stored && stored.length > initialPageFromUrl) return stored
    return [undefined]
  })
  const [pageIndex, setPageIndex] = useState(() => {
    const stored = loadCursors()
    if (stored && stored.length > initialPageFromUrl) return initialPageFromUrl
    return 0
  })
  const [pageInput, setPageInput] = useState(() => String(pageIndex + 1))
  const [total, setTotal] = useState<number | null>(null)
  const [isJumping, setIsJumping] = useState(false)
  const [jumpDate, setJumpDate] = useState('')
  const [infiniteKey, setInfiniteKey] = useState(0)
  const [infiniteInitialCursor, setInfiniteInitialCursor] = useState<string | undefined>(undefined)

  const [lastActivityId, setLastActivityId] = useState<string | null>(() => loadLastActivity())

  // Kontext-Menü-State
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)

  // Multi-Select-State
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [lastSelectedIdx, setLastSelectedIdx] = useState<number | null>(null)

  // Multisport-Expand-State
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const [childrenMap, setChildrenMap] = useState<Record<string, Activity[]>>({})

  const toggleExpand = async (id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
    if (!childrenMap[id]) {
      const resp = await apiFetch(`/api/v1/activities/${id}/children`)
      if (resp.ok) {
        const data = await resp.json() as Activity[]
        setChildrenMap(prev => ({ ...prev, [id]: data }))
      }
    }
  }

  // Modal-States
  const [recomputeIds, setRecomputeIds] = useState<string[] | null>(null)
  const [deleteIds, setDeleteIds] = useState<string[] | null>(null)
  const [deleteTitle, setDeleteTitle] = useState<string | null>(null)
  const [zipLoading, setZipLoading] = useState(false)
  const [openTabsItems, setOpenTabsItems] = useState<{ id: string; title: string | null }[] | null>(null)
  const [groupModalIds, setGroupModalIds] = useState<string[] | null>(null)

  // Filter state (persisted in URL)
  const [filters, setFiltersRaw] = useState<ActivityFilterState>(() => paramsToFilters(searchParams))

  // Search with debounce
  const [searchInput, setSearchInput] = useState(filters.search)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Sort state
  const [sortBy, setSortBy] = useState<SortColumn | null>(null)
  const [sortDir, setSortDir] = useState<SortDir | null>(null)

  const effectiveSortBy = sortBy ?? 'started_at'
  const effectiveSortDir = sortDir ?? 'desc'

  const isInfinite = pageSize === 0
  const cursor = cursors[pageIndex]

  const apiFilters = useMemo(() => filtersToApiParams(filters), [filters])
  const filterKey = useMemo(() => JSON.stringify(apiFilters ?? {}), [apiFilters])

  // ── Multi-Select-Hilfsfunktionen ─────────────────────────────────────────
  function toggleSelect(id: string, idx: number, shiftKey: boolean) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (shiftKey && lastSelectedIdx !== null && activities) {
        const from = Math.min(lastSelectedIdx, idx)
        const to = Math.max(lastSelectedIdx, idx)
        const rangeIds = activities.slice(from, to + 1).map(a => a.id)
        rangeIds.forEach(aid => next.add(aid))
      } else {
        if (next.has(id)) next.delete(id)
        else next.add(id)
      }
      return next
    })
    setLastSelectedIdx(idx)
  }

  function selectAll() {
    if (!activities) return
    setSelectedIds(new Set(activities.map(a => a.id)))
  }

  function clearSelection() {
    setSelectedIds(new Set())
    setLastSelectedIdx(null)
  }

  async function handleZipDownload(ids: string[], fmt: ExportFormat = 'original') {
    setZipLoading(true)
    try {
      await downloadActivitiesZip(ids, fmt)
    } catch (err) {
      console.error('ZIP download failed', err)
    } finally {
      setZipLoading(false)
    }
  }

  // ── Cursors in sessionStorage speichern ──────────────────────────────────
  useEffect(() => {
    saveCursors(cursors)
  }, [cursors])

  // ── Seitenzahl in URL schreiben ──────────────────────────────────────────
  useEffect(() => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (pageIndex === 0) {
        next.delete('page')
      } else {
        next.set('page', String(pageIndex + 1))
      }
      return next
    }, { replace: true })
  }, [pageIndex]) // eslint-disable-line react-hooks/exhaustive-deps

  // Update URL when filters change
  const setFilters = useCallback((next: ActivityFilterState) => {
    setFiltersRaw(next)
    const p = filtersToParams(next)
    setSearchParams(p, { replace: true })
  }, [setSearchParams])

  // Debounced search
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    searchTimerRef.current = setTimeout(() => {
      setFiltersRaw(prev => {
        const next = { ...prev, search: value }
        setSearchParams(filtersToParams(next), { replace: true })
        return next
      })
    }, 300)
  }, [setSearchParams])

  // Reset pagination when filters change
  useEffect(() => {
    setCursors([undefined])
    setPageIndex(0)
    setPageInput('1')
    setTotal(null)
    setLastActivityId(null)
    clearLastActivity()
    clearSelection()
    if (isInfinite) {
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    }
  }, [filterKey]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Sortierung umschalten ────────────────────────────────────────────────
  function toggleSort(column: SortColumn) {
    let newSortBy: SortColumn | null
    let newSortDir: SortDir | null

    if (sortBy !== column) {
      newSortBy = column
      newSortDir = 'asc'
    } else if (sortDir === 'asc') {
      newSortBy = column
      newSortDir = 'desc'
    } else if (sortDir === 'desc') {
      newSortBy = null
      newSortDir = null
    } else {
      newSortBy = column
      newSortDir = 'asc'
    }

    setSortBy(newSortBy)
    setSortDir(newSortDir)
    setCursors([undefined])
    setPageIndex(0)
    setPageInput('1')
    setLastActivityId(null)
    clearLastActivity()
    clearSelection()
    if (isInfinite) {
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    }
    setJumpDate('')
  }

  function sortIndicator(column: SortColumn): string {
    if (sortBy !== column) return ''
    return sortDir === 'asc' ? ' \u25B2' : ' \u25BC'
  }

  // ── Paginierter Modus ────────────────────────────────────────────────────
  const { data: pageData, isLoading: pageLoading } = useQuery({
    queryKey: ['activities', cursor, pageSize, effectiveSortBy, effectiveSortDir, filterKey],
    queryFn: () => fetchActivities(cursor, pageSize as number, effectiveSortBy, effectiveSortDir, apiFilters),
    enabled: !isInfinite,
  })

  useEffect(() => {
    if (!pageData) return
    setTotal(pageData.total)
    if (pageData.next_cursor != null && cursors[pageIndex + 1] == null) {
      setCursors(prev => {
        const next = [...prev]
        next[pageIndex + 1] = pageData.next_cursor!
        return next
      })
    }
  }, [pageData, pageIndex]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Infinite-Scroll-Modus ────────────────────────────────────────────────
  const {
    data: infiniteData,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: infiniteLoading,
  } = useInfiniteQuery({
    queryKey: ['activities-infinite', infiniteKey, effectiveSortBy, effectiveSortDir, filterKey],
    queryFn: ({ pageParam }) => fetchActivities(pageParam as string | undefined, 20, effectiveSortBy, effectiveSortDir, apiFilters),
    initialPageParam: infiniteInitialCursor as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    enabled: isInfinite,
  })

  useEffect(() => {
    if (infiniteData?.pages[0]?.total != null) setTotal(infiniteData.pages[0].total)
  }, [infiniteData])

  // Sentinel für Infinite Scroll
  const sentinelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!isInfinite || !sentinelRef.current) return
    const el = sentinelRef.current
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
          void fetchNextPage()
        }
      },
      { rootMargin: '200px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [isInfinite, hasNextPage, isFetchingNextPage, fetchNextPage])

  // ── Navigation ───────────────────────────────────────────────────────────
  function goToPage(idx: number) {
    if (idx < 0 || idx >= cursors.length) return
    setPageIndex(idx)
    setPageInput(String(idx + 1))
    clearSelection()
  }

  function onPageSizeChange(size: PageSize) {
    setPageSize(size)
    if (jumpDate) {
      const dateCursor = buildCursorForDate(jumpDate, effectiveSortDir)
      if (size === 0) {
        setInfiniteInitialCursor(dateCursor)
        setInfiniteKey(prev => prev + 1)
      } else {
        setCursors([dateCursor])
      }
    } else {
      setCursors([undefined])
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    }
    setPageIndex(0)
    setPageInput('1')
    clearSelection()
  }

  async function jumpToPage(targetIdx: number) {
    if (targetIdx < 0) targetIdx = 0

    if (targetIdx < cursors.length) {
      goToPage(targetIdx)
      return
    }

    setIsJumping(true)
    let localCursors = [...cursors]
    try {
      while (localCursors.length <= targetIdx) {
        const cur = localCursors[localCursors.length - 1]
        const data = await queryClient.fetchQuery({
          queryKey: ['activities', cur, pageSize, effectiveSortBy, effectiveSortDir, filterKey],
          queryFn: () => fetchActivities(cur, pageSize as number, effectiveSortBy, effectiveSortDir, apiFilters),
        })
        if (!data.next_cursor) {
          targetIdx = localCursors.length - 1
          break
        }
        localCursors = [...localCursors, data.next_cursor]
      }
      setCursors(localCursors)
      setPageIndex(targetIdx)
      setPageInput(String(targetIdx + 1))
      clearSelection()
    } finally {
      setIsJumping(false)
    }
  }

  function submitPageInput(raw: string) {
    const n = parseInt(raw, 10)
    if (isNaN(n)) { setPageInput(String(pageIndex + 1)); return }
    void jumpToPage(n - 1)
  }

  // ── Datumssprung ─────────────────────────────────────────────────────────
  const isDateJumpEnabled = sortBy === null || sortBy === 'started_at'

  function handleJumpDateChange(dateStr: string) {
    setJumpDate(dateStr)
    if (dateStr.length === 10) {
      const year = parseInt(dateStr.substring(0, 4), 10)
      if (year >= 1900 && !isNaN(Date.parse(dateStr + 'T00:00:00'))) {
        performDateJump(dateStr)
      }
    }
  }

  function performDateJump(dateStr: string) {
    const dateCursor = buildCursorForDate(dateStr, effectiveSortDir)
    if (isInfinite) {
      setInfiniteInitialCursor(dateCursor)
      setInfiniteKey(prev => prev + 1)
    } else {
      setCursors([dateCursor])
      setPageIndex(0)
      setPageInput('1')
    }
    clearSelection()
  }

  function resetDateJump() {
    setJumpDate('')
    if (isInfinite) {
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    } else {
      setCursors([undefined])
      setPageIndex(0)
      setPageInput('1')
    }
    clearSelection()
  }

  // ── Daten ────────────────────────────────────────────────────────────────
  const activities = isInfinite
    ? (infiniteData?.pages.flatMap(p => p.activities) ?? [])
    : (pageData?.activities ?? [])

  const isLoading = isInfinite ? infiniteLoading : pageLoading
  const totalPages = total != null && pageSize > 0
    ? Math.ceil(total / pageSize)
    : null
  const maxNavigablePage = cursors.length

  // ── Sortierbare Spaltenheader ─────────────────────────────────────────────
  const sortableHeaderClass = (column: SortColumn) => {
    const base = 'px-4 py-3 font-medium select-none cursor-pointer transition-colors'
    const active = sortBy === column
      ? 'text-neutral-800 dark:text-neutral-200'
      : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-300'
    return `${base} ${active}`
  }

  const allOnPageSelected = activities.length > 0 && activities.every(a => selectedIds.has(a.id))
  const someSelected = selectedIds.size > 0 && !allOnPageSelected

  // ── Render ────────────────────────────────────────────────────────────────
  if (isLoading && activities.length === 0) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Aktivitäten...</div>
  }

  return (
    <div>
      {/* Kontext-Menü */}
      {contextMenu && (
        <ActivityContextMenu
          state={contextMenu}
          selectedIds={selectedIds}
          zipLoading={zipLoading}
          onClose={() => setContextMenu(null)}
          onRecompute={(ids) => setRecomputeIds(ids)}
          onDelete={(ids, title) => { setDeleteIds(ids); setDeleteTitle(title ?? null) }}
          onZip={handleZipDownload}
          onOpenTabs={(items) => setOpenTabsItems(items)}
          onGroup={(ids) => setGroupModalIds(ids)}
          onUngroup={async (id) => {
            await apiFetch(`/api/v1/activities/${id}/group`, { method: 'DELETE' })
            queryClient.invalidateQueries({ queryKey: ['activities'] })
          }}
        />
      )}

      {/* Modals */}
      {recomputeIds && (
        <RecomputeModal
          activityIds={recomputeIds}
          onClose={() => setRecomputeIds(null)}
          onSuccess={clearSelection}
        />
      )}
      {deleteIds && (
        <DeleteConfirmModal
          activityIds={deleteIds}
          activityTitle={deleteTitle}
          onClose={() => { setDeleteIds(null); setDeleteTitle(null) }}
          onDeleted={() => {
            clearSelection()
            queryClient.invalidateQueries({ queryKey: ['activities'] })
          }}
        />
      )}
      {openTabsItems && (
        <OpenTabsDialog items={openTabsItems} onClose={() => setOpenTabsItems(null)} />
      )}
      {groupModalIds && (
        <GroupModal
          activityIds={groupModalIds}
          onClose={() => setGroupModalIds(null)}
          onGrouped={(parentId) => {
            clearSelection()
            queryClient.invalidateQueries({ queryKey: ['activities'] })
            navigate(`/activities/${parentId}`)
          }}
        />
      )}

      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-baseline gap-3">
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Aktivitäten</h1>
          {total != null && (
            <span className="text-sm text-neutral-400">
              {`${total.toLocaleString('de-DE')} gesamt`}
            </span>
          )}
        </div>

        <div className="flex items-center gap-4">
          {/* Suche (server-side, debounced) */}
          <div className="relative">
            <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-neutral-400 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
            </svg>
            <input
              type="text"
              value={searchInput}
              onChange={e => handleSearchChange(e.target.value)}
              placeholder="Suchen…"
              className="border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 pl-8 text-xs w-48
                         bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                         placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                         focus:outline-none focus:ring-1 focus:ring-neutral-400"
            />
            {searchInput && (
              <button
                onClick={() => handleSearchChange('')}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors text-xs"
              >
                &times;
              </button>
            )}
          </div>

          {/* Activity Filters */}
          <ActivityFilters filters={filters} onChange={setFilters} />

          {/* Datumssprung (nur bei Sortierung nach Datum) */}
          <div className="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400">
            <label htmlFor="jump-date" className={`whitespace-nowrap${!isDateJumpEnabled ? ' opacity-40' : ''}`}>Springe zu</label>
            <input
              id="jump-date"
              type="date"
              value={jumpDate}
              max={format(new Date(), 'yyyy-MM-dd')}
              disabled={!isDateJumpEnabled}
              onChange={e => handleJumpDateChange(e.target.value)}
              className={`border border-neutral-200 dark:border-neutral-700 rounded px-2 py-1 text-xs
                         bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                         focus:outline-none focus:ring-1 focus:ring-neutral-400
                         ${!isDateJumpEnabled ? 'opacity-40 cursor-not-allowed' : ''}`}
            />
            {jumpDate && isDateJumpEnabled && (
              <button
                onClick={resetDateJump}
                title="Datumsfilter zurücksetzen"
                className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors text-xs"
              >
                &times;
              </button>
            )}
          </div>

          {/* Seitengrößen-Auswahl */}
          <div className="flex items-center gap-2 text-sm text-neutral-500 dark:text-neutral-400">
            <span>Zeige</span>
            <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
              {PAGE_SIZES.map(size => (
                <button
                  key={size}
                  onClick={() => onPageSizeChange(size)}
                  className={
                    size === pageSize
                      ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                      : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                  }
                >
                  {size}
                </button>
              ))}
              <button
                onClick={() => onPageSizeChange(0)}
                title="Endlos scrollen"
                className={
                  isInfinite
                    ? 'px-3 py-1.5 bg-neutral-900 dark:bg-neutral-100 text-white dark:text-neutral-900 font-medium'
                    : 'px-3 py-1.5 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-600 dark:text-neutral-300 transition-colors'
                }
              >
                ∞
              </button>
            </div>
            {!isInfinite && <span>pro Seite</span>}
          </div>
        </div>
      </div>

      {/* Tabelle */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
              {/* Checkbox-Spalte */}
              <th className="w-8 pl-3 pr-2 py-3">
                <input
                  type="checkbox"
                  checked={allOnPageSelected}
                  ref={el => { if (el) el.indeterminate = someSelected }}
                  onChange={() => allOnPageSelected ? clearSelection() : selectAll()}
                  className="w-4 h-4 rounded accent-orange-500 cursor-pointer"
                  aria-label="Alle auswählen"
                />
              </th>
              <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Titel</th>
              <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Sport</th>
              <th
                className={`text-left ${sortableHeaderClass('started_at')}`}
                onClick={() => toggleSort('started_at')}
              >
                Datum{sortIndicator('started_at')}
              </th>
              <th
                className={`text-right ${sortableHeaderClass('distance_m')}`}
                onClick={() => toggleSort('distance_m')}
              >
                Distanz{sortIndicator('distance_m')}
              </th>
              <th
                className={`text-right ${sortableHeaderClass('elapsed_time_s')}`}
                onClick={() => toggleSort('elapsed_time_s')}
              >
                Zeit{sortIndicator('elapsed_time_s')}
              </th>
              <th
                className={`text-right ${sortableHeaderClass('elevation_gain_m')}`}
                onClick={() => toggleSort('elevation_gain_m')}
              >
                Höhenmeter{sortIndicator('elevation_gain_m')}
              </th>
              {/* 3-Punkt-Menü-Spalte (Platzhalter für Ausrichtung) */}
              <th className="w-8 pr-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
            {activities.map((activity, idx) => (
              <React.Fragment key={activity.id}>
                <ActivityRow
                  activity={activity}
                  idx={idx}
                  highlighted={!!jumpDate && activity.started_at.startsWith(jumpDate)}
                  isLastVisited={activity.id === lastActivityId}
                  isSelected={selectedIds.has(activity.id)}
                  isMultisport={activity.sport_type === 'multisport'}
                  isExpanded={expandedIds.has(activity.id)}
                  onExpand={toggleExpand}
                  onNavigate={(id) => {
                    saveLastActivity(id)
                    setLastActivityId(id)
                  }}
                  onContextMenu={(e, id, title, sportType) => {
                    e.preventDefault()
                    const x = Math.min(e.clientX, window.innerWidth - 220)
                    const y = Math.min(e.clientY, window.innerHeight - 80)
                    setContextMenu({ x, y, activityId: id, activityTitle: title, sportType })
                  }}
                  onToggleSelect={toggleSelect}
                  onThreeDotClick={(e, id, title, sportType) => {
                    e.stopPropagation()
                    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
                    const x = Math.min(rect.left, window.innerWidth - 220)
                    const y = Math.min(rect.bottom + 4, window.innerHeight - 80)
                    setContextMenu({ x, y, activityId: id, activityTitle: title, sportType })
                  }}
                />
                {activity.sport_type === 'multisport' && expandedIds.has(activity.id) &&
                  (childrenMap[activity.id] ?? []).map(child => (
                    <tr key={child.id} className="bg-neutral-50/80 dark:bg-neutral-800/30">
                      <td className="py-1.5 pl-3 pr-2 w-8">
                        <div className="w-4 h-4 border-l-2 border-b-2 border-neutral-300 dark:border-neutral-600 ml-2" />
                      </td>
                      <td colSpan={6} className="py-1.5 pr-2">
                        <a
                          href={`/activities/${child.id}`}
                          className="flex items-center gap-2 hover:underline text-sm text-neutral-700 dark:text-neutral-300 no-underline"
                          onClick={e => { e.stopPropagation(); saveLastActivity(child.id); setLastActivityId(child.id) }}
                        >
                          <SportIcon sport={child.sport_type} subType={child.sub_type} className="w-4 h-4 shrink-0" />
                          <span className="font-medium">{child.title ?? child.title_auto ?? sportLabel(child.sport_type, child.sub_type)}</span>
                          <span className="text-neutral-400 text-xs ml-auto flex items-center gap-1">
                            {child.elapsed_time_s != null && <span>{formatDuration(child.elapsed_time_s)}</span>}
                            {child.elapsed_time_s != null && child.distance_m != null && <span>·</span>}
                            {child.distance_m != null && <span>{formatDistance(child.distance_m)}</span>}
                          </span>
                        </a>
                      </td>
                      <td className="w-8" />
                    </tr>
                  ))
                }
              </React.Fragment>
            ))}
          </tbody>
        </table>
      </div>

      {/* Infinite-Scroll-Sentinel */}
      {isInfinite && (
        <div ref={sentinelRef} className="flex justify-center py-6">
          {isFetchingNextPage && (
            <span className="text-sm text-neutral-400">Lade weitere Aktivitäten...</span>
          )}
          {!hasNextPage && activities.length > 0 && (
            <span className="text-sm text-neutral-400">Alle {total?.toLocaleString('de-DE')} Aktivitäten geladen</span>
          )}
        </div>
      )}

      {/* Bulk-Aktionsleiste */}
      {selectedIds.size > 0 && (
        <div className="sticky bottom-0 z-20 bg-white dark:bg-neutral-900 border-t border-neutral-200 dark:border-neutral-800 px-4 py-3 flex items-center gap-3 shadow-[0_-4px_12px_rgba(0,0,0,0.08)]">
          <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mr-auto">
            {selectedIds.size} ausgewählt
          </span>
          <button
            onClick={clearSelection}
            className="text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
          >
            Abwählen
          </button>
          <button
            onClick={() => {
              const actMap = new Map(activities.map(a => [a.id, a.title ?? a.title_auto ?? null]))
              setOpenTabsItems([...selectedIds].map(id => ({ id, title: actMap.get(id) ?? null })))
            }}
            className="text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
          >
            Alle öffnen
          </button>
          {selectedIds.size >= 2 && selectedIds.size <= 3 && (
            <button
              onClick={() => navigate(`/activities/compare?ids=${[...selectedIds].join(',')}`)}
              className="text-xs px-3 py-1.5 rounded-lg border border-accent-DEFAULT text-accent-DEFAULT hover:bg-accent-50 dark:hover:bg-accent-900/20 cursor-pointer font-medium"
            >
              Vergleichen
            </button>
          )}
          <ZipDropdown
            ids={[...selectedIds]}
            loading={zipLoading}
            onDownload={handleZipDownload}
          />
          <button
            onClick={() => setGroupModalIds([...selectedIds])}
            className="text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
          >
            Gruppieren
          </button>
          <button
            onClick={() => setRecomputeIds([...selectedIds])}
            className="text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
          >
            Neu berechnen
          </button>
          <button
            onClick={() => setDeleteIds([...selectedIds])}
            className="text-xs px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-700 text-white font-medium cursor-pointer flex items-center gap-1.5"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
            Löschen
          </button>
        </div>
      )}

      {/* Pagination (nur im paginierten Modus) */}
      {!isInfinite && (
        <div className="flex items-center justify-between mt-4 gap-4">
          <button
            onClick={() => goToPage(pageIndex - 1)}
            disabled={isJumping || pageIndex === 0}
            className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-300
                       hover:bg-neutral-50 dark:hover:bg-neutral-800
                       disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            ← Zurück
          </button>

          <div className="flex items-center gap-1.5 text-sm text-neutral-500 dark:text-neutral-400 whitespace-nowrap">
            <span>Seite</span>
            <input
              type="number"
              min={1}
              max={maxNavigablePage}
              value={pageInput}
              onChange={(e) => setPageInput(e.target.value)}
              onBlur={(e) => submitPageInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') submitPageInput(pageInput) }}
              className="w-12 text-center border border-neutral-200 dark:border-neutral-700
                         rounded px-1 py-0.5 text-sm
                         bg-white dark:bg-neutral-900
                         text-neutral-800 dark:text-neutral-200
                         focus:outline-none focus:ring-1 focus:ring-neutral-400"
            />
            {totalPages != null && (
              <span className="text-neutral-400 dark:text-neutral-600">
                von {totalPages}
              </span>
            )}
          </div>

          {isJumping && (
            <span className="text-xs text-neutral-400 px-2">Lade...</span>
          )}

          <button
            onClick={() => goToPage(pageIndex + 1)}
            disabled={isJumping || (pageData?.next_cursor == null && pageIndex + 1 >= cursors.length)}
            className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-300
                       hover:bg-neutral-50 dark:hover:bg-neutral-800
                       disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            Weiter →
          </button>
        </div>
      )}
    </div>
  )
}

interface ActivityRowProps {
  activity: Activity
  idx: number
  highlighted?: boolean
  isLastVisited?: boolean
  isSelected: boolean
  isMultisport?: boolean
  isExpanded?: boolean
  onExpand?: (id: string) => void
  onNavigate: (id: string) => void
  onContextMenu: (e: React.MouseEvent, id: string, title: string | null | undefined, sportType: string) => void
  onToggleSelect: (id: string, idx: number, shiftKey: boolean) => void
  onThreeDotClick: (e: React.MouseEvent<HTMLButtonElement>, id: string, title: string | null | undefined, sportType: string) => void
}

function ActivityRow({
  activity,
  idx,
  highlighted,
  isLastVisited,
  isSelected,
  isMultisport,
  isExpanded,
  onExpand,
  onNavigate,
  onContextMenu,
  onToggleSelect,
  onThreeDotClick,
}: ActivityRowProps) {
  const navigate = useNavigate()
  const label = sportLabel(activity.sport_type, activity.sub_type)
  const date = new Date(activity.started_at)
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)

  function handleClick() {
    onNavigate(activity.id)
    navigate(`/activities/${activity.id}`)
  }

  let rowClass = 'even:bg-neutral-50/50 dark:even:bg-neutral-800/20 hover:bg-neutral-100 dark:hover:bg-neutral-900/60 transition-colors cursor-pointer group'
  if (isSelected) {
    rowClass = 'bg-orange-50/70 dark:bg-orange-900/15 hover:bg-accent-100/60 dark:hover:bg-orange-900/25 transition-colors cursor-pointer group'
  } else if (isLastVisited) {
    rowClass = 'bg-accent-100/60 dark:bg-orange-900/25 ring-1 ring-inset ring-orange-400/40 dark:ring-accent-DEFAULT/30 hover:bg-accent-100/80 dark:hover:bg-orange-900/35 transition-colors cursor-pointer group'
  } else if (highlighted) {
    rowClass = 'bg-orange-50 dark:bg-orange-900/20 even:bg-orange-50 dark:even:bg-orange-900/20 hover:bg-neutral-100 dark:hover:bg-neutral-900/60 transition-colors cursor-pointer group'
  }

  const titleClass = isLastVisited && !isSelected
    ? 'text-accent-600 dark:text-orange-400 font-medium'
    : 'text-neutral-800 dark:text-neutral-200 font-medium group-hover:text-accent-DEFAULT dark:group-hover:text-orange-400 transition-colors'

  return (
    <tr
      onClick={handleClick}
      onContextMenu={e => onContextMenu(e, activity.id, activity.title ?? activity.title_auto, activity.sport_type)}
      className={rowClass}
    >
      {/* Checkbox + optional Multisport-Chevron */}
      <td className="py-3 pl-3 pr-2 w-8" onClick={e => e.stopPropagation()}>
        <div className="flex items-center gap-1">
          <input
            type="checkbox"
            checked={isSelected}
            onChange={e => onToggleSelect(activity.id, idx, (e.nativeEvent as MouseEvent).shiftKey ?? false)}
            className="w-4 h-4 rounded accent-orange-500 cursor-pointer"
            aria-label={`Aktivität ${activity.title ?? activity.title_auto ?? activity.id} auswählen`}
          />
          {isMultisport && (
            <button
              onClick={e => { e.stopPropagation(); onExpand?.(activity.id) }}
              className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
              title="Disziplinen anzeigen"
            >
              <svg className={`w-3 h-3 transition-transform ${isExpanded ? 'rotate-90' : ''}`} fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
              </svg>
            </button>
          )}
        </div>
      </td>
      <td className={`px-4 py-3 ${titleClass}`}>
        {activity.title ?? activity.title_auto ?? (
          <span className="text-neutral-400 font-normal italic">Kein Titel</span>
        )}
      </td>
      <td className="px-4 py-3">
        <span className="inline-flex items-center gap-1.5 text-neutral-500 dark:text-neutral-400">
          <SportIcon sport={activity.sport_type} subType={activity.sub_type} className="w-4 h-4 flex-shrink-0" />
          <span className="text-xs">{label}</span>
          {showIndoorBadge(activity.is_indoor, activity.sub_type) && <span className="text-neutral-400 dark:text-neutral-600 text-xs">indoor</span>}
        </span>
      </td>
      <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400">
        {format(date, 'EEE, dd. MMM yyyy', { locale: de })}
        <span className="ml-2 text-neutral-400 dark:text-neutral-600 text-xs">{format(date, 'HH:mm')}</span>
      </td>
      <td className="px-4 py-3 text-right text-neutral-700 dark:text-neutral-300 tabular-nums">
        {formatDistance(activity.distance_m)}
      </td>
      <td className="px-4 py-3 text-right text-neutral-700 dark:text-neutral-300 tabular-nums">
        {formatDuration(activity.elapsed_time_s ?? 0)}
      </td>
      {/* Höhenmeter */}
      <td className="px-4 py-3">
        {(gain ?? loss) ? (
          <div className="flex items-center justify-end gap-1 tabular-nums text-sm">
            <span className="inline-block w-[4.5rem] text-right text-emerald-600 dark:text-emerald-500">
              {gain ?? ''}
            </span>
            <span className="text-neutral-300 dark:text-neutral-700 w-3 text-center">
              {gain && loss ? '/' : ''}
            </span>
            <span className="inline-block w-[4.5rem] text-right text-rose-500 dark:text-rose-400">
              {loss ?? ''}
            </span>
          </div>
        ) : (
          <div className="text-right text-neutral-400 dark:text-neutral-600">—</div>
        )}
      </td>
      {/* 3-Punkt-Menü */}
      <td className="py-3 pl-0 pr-2 w-8" onClick={e => e.stopPropagation()}>
        <button
          onClick={e => onThreeDotClick(e, activity.id, activity.title ?? activity.title_auto, activity.sport_type)}
          className="p-1 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity"
          aria-label="Aktivitätsmenü"
        >
          <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
            <path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z" />
          </svg>
        </button>
      </td>
    </tr>
  )
}

// ── GroupModal ────────────────────────────────────────────────────────────────
function GroupModal({
  activityIds,
  onClose,
  onGrouped,
}: {
  activityIds: string[]
  onClose: () => void
  onGrouped: (parentId: string) => void
}) {
  const [title, setTitle] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleCreate() {
    setLoading(true)
    setError(null)
    try {
      const resp = await apiFetch('/api/v1/activities/group', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ activity_ids: activityIds, title: title.trim() || 'Multisport' }),
      })
      if (!resp.ok) {
        const text = await resp.text()
        setError(`Fehler ${resp.status}: ${text}`)
        return
      }
      const parent = await resp.json() as { id: string }
      onGrouped(parent.id)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div
        className="bg-white dark:bg-neutral-900 rounded-xl shadow-xl w-full max-w-sm p-6 space-y-4"
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
          Als Multisport gruppieren
        </h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {activityIds.length} Aktivitäten werden zu einer Multisport-Gruppe zusammengefasst.
          Die Originalaktivitäten bleiben vollständig erhalten.
        </p>
        <div>
          <label className="block text-xs text-neutral-500 dark:text-neutral-400 mb-1">
            Gruppenname (optional)
          </label>
          <input
            type="text"
            value={title}
            onChange={e => setTitle(e.target.value)}
            placeholder="Multisport"
            className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
            onKeyDown={e => { if (e.key === 'Enter') handleCreate() }}
            autoFocus
          />
        </div>
        {error && (
          <p className="text-xs text-red-600 dark:text-red-400">{error}</p>
        )}
        <div className="flex gap-2 justify-end pt-1">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer"
          >
            Abbrechen
          </button>
          <button
            onClick={handleCreate}
            disabled={loading}
            className="px-4 py-2 text-sm rounded-lg bg-accent-DEFAULT hover:bg-accent-600 text-white font-medium disabled:opacity-50 cursor-pointer"
          >
            {loading ? 'Erstelle...' : 'Gruppe erstellen'}
          </button>
        </div>
      </div>
    </div>
  )
}
