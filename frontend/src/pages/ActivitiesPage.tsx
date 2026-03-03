import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'
import {
  formatDuration, formatDistance,
  formatElevationGain, formatElevationLoss, sportLabel, showIndoorBadge,
} from '@/lib/utils'
import { SportIcon } from '@/components/SportIcon'
import ActivityFilters, { paramsToFilters, filtersToParams, filtersToApiParams } from '@/components/ActivityFilters'
import type { ActivityFilterState } from '@/components/ActivityFilters'

const PAGE_SIZES = [20, 50, 100] as const

// 0 = Infinite-Scroll-Modus
type PageSize = 20 | 50 | 100 | 0

type SortColumn = 'started_at' | 'distance_m' | 'elevation_gain_m' | 'elapsed_time_s'
type SortDir = 'asc' | 'desc'

/**
 * Erzeugt einen Cursor für ein bestimmtes Datum.
 * Format: base64url(RFC3339Nano_timestamp + "|" + uuid)
 *
 * DESC: Cursor = Ende des Tages + max-UUID → erste Seite beginnt mit Aktivitäten an/vor diesem Tag.
 * ASC:  Cursor = Anfang des Tages + min-UUID → erste Seite beginnt mit Aktivitäten ab diesem Tag.
 *
 * Die UUID muss gültig sein (Typ uuid in PostgreSQL), daher max/min-UUID statt Fake-ID.
 */
function buildCursorForDate(dateStr: string, direction: SortDir): string {
  // dateStr ist "YYYY-MM-DD"
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

export default function ActivitiesPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()

  const [pageSize, setPageSize] = useState<PageSize>(20)
  // cursors[i] = cursor um Seite i+1 zu laden (cursors[0] = undefined = erste Seite)
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined])
  const [pageIndex, setPageIndex] = useState(0)
  const [pageInput, setPageInput] = useState('1')
  const [total, setTotal] = useState<number | null>(null)
  const [isJumping, setIsJumping] = useState(false)
  const [jumpDate, setJumpDate] = useState('')
  // Key für Infinite-Query-Reset bei Datumssprung
  const [infiniteKey, setInfiniteKey] = useState(0)
  const [infiniteInitialCursor, setInfiniteInitialCursor] = useState<string | undefined>(undefined)

  // Filter state (persisted in URL)
  const [filters, setFiltersRaw] = useState<ActivityFilterState>(() => paramsToFilters(searchParams))

  // Search with debounce
  const [searchInput, setSearchInput] = useState(filters.search)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Sort state: null = unsorted (default: started_at DESC)
  const [sortBy, setSortBy] = useState<SortColumn | null>(null)
  const [sortDir, setSortDir] = useState<SortDir | null>(null)

  // Effective sort values for API calls
  const effectiveSortBy = sortBy ?? 'started_at'
  const effectiveSortDir = sortDir ?? 'desc'

  const isInfinite = pageSize === 0
  const cursor = cursors[pageIndex]

  // API filter params (memoized for query key stability)
  const apiFilters = useMemo(() => filtersToApiParams(filters), [filters])
  const filterKey = useMemo(() => JSON.stringify(apiFilters ?? {}), [apiFilters])

  // Update URL when filters change
  const setFilters = useCallback((next: ActivityFilterState) => {
    setFiltersRaw(next)
    const p = filtersToParams(next)
    setSearchParams(p, { replace: true })
  }, [setSearchParams])

  // Debounced search: update filter.search after 300ms
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
    if (isInfinite) {
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    }
  }, [filterKey]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Sortierung umschalten ──────────────────────────────────────────────────
  // Cycle: unsorted → asc → desc → unsorted
  function toggleSort(column: SortColumn) {
    let newSortBy: SortColumn | null
    let newSortDir: SortDir | null

    if (sortBy !== column) {
      // Neue Spalte anklicken → aufsteigend starten
      newSortBy = column
      newSortDir = 'asc'
    } else if (sortDir === 'asc') {
      // Gleiche Spalte, asc → desc
      newSortBy = column
      newSortDir = 'desc'
    } else if (sortDir === 'desc') {
      // Gleiche Spalte, desc → unsorted (zurück zum Default)
      newSortBy = null
      newSortDir = null
    } else {
      // sortDir === null (sollte nicht vorkommen, Fallback)
      newSortBy = column
      newSortDir = 'asc'
    }

    setSortBy(newSortBy)
    setSortDir(newSortDir)

    // Reset pagination when sort changes
    setCursors([undefined])
    setPageIndex(0)
    setPageInput('1')
    if (isInfinite) {
      setInfiniteInitialCursor(undefined)
      setInfiniteKey(prev => prev + 1)
    }
    // Reset date jump as cursor format changes with sort column
    setJumpDate('')
  }

  function sortIndicator(column: SortColumn): string {
    if (sortBy !== column) return ''
    return sortDir === 'asc' ? ' \u25B2' : ' \u25BC'
  }

  // ── Paginierter Modus ──────────────────────────────────────────────────────
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

  // ── Infinite-Scroll-Modus ─────────────────────────────────────────────────
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

  // ── Navigation ─────────────────────────────────────────────────────────────
  function goToPage(idx: number) {
    if (idx < 0 || idx >= cursors.length) return
    setPageIndex(idx)
    setPageInput(String(idx + 1))
  }

  function onPageSizeChange(size: PageSize) {
    setPageSize(size)
    // Datumssprung beibehalten beim Wechsel der Seitengröße
    if (jumpDate) {
      const dateCursor = buildCursorForDate(jumpDate, effectiveSortDir)
      if (size === 0) {
        // Wechsel zu Infinite
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
  }

  // Jumps to an arbitrary page index, fetching intermediate cursors as needed.
  async function jumpToPage(targetIdx: number) {
    if (targetIdx < 0) targetIdx = 0

    // Fast path: cursor already in cache
    if (targetIdx < cursors.length) {
      goToPage(targetIdx)
      return
    }

    // Sequentially prefetch pages until we have the cursor for targetIdx
    setIsJumping(true)
    let localCursors = [...cursors]
    try {
      while (localCursors.length <= targetIdx) {
        const cursor = localCursors[localCursors.length - 1]
        const data = await queryClient.fetchQuery({
          queryKey: ['activities', cursor, pageSize, effectiveSortBy, effectiveSortDir, filterKey],
          queryFn: () => fetchActivities(cursor, pageSize as number, effectiveSortBy, effectiveSortDir, apiFilters),
        })
        if (!data.next_cursor) {
          targetIdx = localCursors.length - 1 // clamp to last available page
          break
        }
        localCursors = [...localCursors, data.next_cursor]
      }
      setCursors(localCursors)
      setPageIndex(targetIdx)
      setPageInput(String(targetIdx + 1))
    } finally {
      setIsJumping(false)
    }
  }

  function submitPageInput(raw: string) {
    const n = parseInt(raw, 10)
    if (isNaN(n)) { setPageInput(String(pageIndex + 1)); return }
    void jumpToPage(n - 1)
  }

  // ── Datumssprung ────────────────────────────────────────────────────────────
  // Date jump is only valid when sorting by started_at (default)
  const isDateJumpEnabled = sortBy === null || sortBy === 'started_at'

  function handleJumpDateChange(dateStr: string) {
    setJumpDate(dateStr)
    // Only perform the actual jump when the date is complete and valid.
    // HTML date inputs fire onChange on every keystroke in the year field,
    // producing intermediate values like "0002", "0020", etc.
    // We require a full "YYYY-MM-DD" (length 10) with a 4-digit year >= 1900.
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
      // Reset Infinite-Scroll mit neuem Startcursor
      setInfiniteInitialCursor(dateCursor)
      setInfiniteKey(prev => prev + 1)
    } else {
      // Im paginierten Modus: Setze Cursor direkt auf das Datum
      setCursors([dateCursor])
      setPageIndex(0)
      setPageInput('1')
    }
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
  }

  // ── Daten ──────────────────────────────────────────────────────────────────
  const activities = isInfinite
    ? (infiniteData?.pages.flatMap(p => p.activities) ?? [])
    : (pageData?.activities ?? [])

  const isLoading = isInfinite ? infiniteLoading : pageLoading
  const totalPages = total != null && pageSize > 0
    ? Math.ceil(total / pageSize)
    : null
  const maxNavigablePage = cursors.length // Seiten für die wir einen Cursor haben

  // ── Sortierbare Spaltenheader ──────────────────────────────────────────────
  const sortableHeaderClass = (column: SortColumn) => {
    const base = 'px-4 py-3 font-medium select-none cursor-pointer transition-colors'
    const active = sortBy === column
      ? 'text-neutral-800 dark:text-neutral-200'
      : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-300'
    return `${base} ${active}`
  }

  // ── Render ─────────────────────────────────────────────────────────────────
  if (isLoading && activities.length === 0) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Aktivitäten...</div>
  }

  return (
    <div>
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
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
            {activities.map(activity => (
              <ActivityRow key={activity.id} activity={activity} highlighted={!!jumpDate && activity.started_at.startsWith(jumpDate)} />
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

          {/* Seite X von Y mit Eingabefeld */}
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

function ActivityRow({ activity, highlighted }: { activity: Activity; highlighted?: boolean }) {
  const navigate = useNavigate()
  const label = sportLabel(activity.sport_type, activity.sub_type)
  const date = new Date(activity.started_at)
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)

  return (
    <tr
      onClick={() => navigate(`/activities/${activity.id}`)}
      className={`even:bg-neutral-50/50 dark:even:bg-neutral-800/20 hover:bg-neutral-100 dark:hover:bg-neutral-900/60 transition-colors cursor-pointer group${highlighted ? ' bg-orange-50 dark:bg-orange-900/20 even:bg-orange-50 dark:even:bg-orange-900/20' : ''}`}
    >
      <td className="px-4 py-3 text-neutral-800 dark:text-neutral-200 font-medium group-hover:text-orange-500 transition-colors">
        {activity.title ?? (
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
      {/* Höhenmeter: zwei feste Spaltenbreiten für 4-stellige Werte */}
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
    </tr>
  )
}
