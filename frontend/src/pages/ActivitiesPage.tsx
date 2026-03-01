import { useEffect, useRef, useState } from 'react'
import { useQuery, useInfiniteQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'
import {
  formatDuration, formatDistance,
  formatElevationGain, formatElevationLoss, sportLabel,
} from '@/lib/utils'
import { SportIcon } from '@/components/SportIcon'

const PAGE_SIZES = [20, 50, 100] as const

// 0 = Infinite-Scroll-Modus
type PageSize = 20 | 50 | 100 | 0

export default function ActivitiesPage() {
  const [pageSize, setPageSize] = useState<PageSize>(20)
  // cursors[i] = cursor um Seite i+1 zu laden (cursors[0] = undefined = erste Seite)
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined])
  const [pageIndex, setPageIndex] = useState(0)
  const [pageInput, setPageInput] = useState('1')
  const [total, setTotal] = useState<number | null>(null)

  const isInfinite = pageSize === 0
  const cursor = cursors[pageIndex]

  // ── Paginierter Modus ──────────────────────────────────────────────────────
  const { data: pageData, isLoading: pageLoading } = useQuery({
    queryKey: ['activities', cursor, pageSize],
    queryFn: () => fetchActivities(cursor, pageSize as number),
    enabled: !isInfinite,
  })

  useEffect(() => {
    if (!pageData) return
    if (pageData.total > 0) setTotal(pageData.total)
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
    queryKey: ['activities-infinite'],
    queryFn: ({ pageParam }) => fetchActivities(pageParam as string | undefined, 20),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    enabled: isInfinite,
  })

  useEffect(() => {
    if (infiniteData?.pages[0]?.total) setTotal(infiniteData.pages[0].total)
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
    setCursors([undefined])
    setPageIndex(0)
    setPageInput('1')
  }

  function submitPageInput(raw: string) {
    const n = parseInt(raw, 10)
    if (isNaN(n)) { setPageInput(String(pageIndex + 1)); return }
    const idx = n - 1
    if (idx >= 0 && idx < cursors.length) {
      goToPage(idx)
    } else {
      setPageInput(String(pageIndex + 1)) // revert, Seite noch nicht geladen
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

  // ── Render ─────────────────────────────────────────────────────────────────
  if (isLoading && activities.length === 0) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Aktivitäten…</div>
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-baseline gap-3">
          <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Aktivitäten</h1>
          {total != null && (
            <span className="text-sm text-neutral-400">
              {total.toLocaleString('de-DE')} gesamt
            </span>
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

      {/* Tabelle */}
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
              <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Titel</th>
              <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Sport</th>
              <th className="text-left px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Datum</th>
              <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Distanz</th>
              <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Zeit</th>
              <th className="text-right px-4 py-3 text-neutral-500 dark:text-neutral-400 font-medium">Höhenmeter</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
            {activities.map(activity => (
              <ActivityRow key={activity.id} activity={activity} />
            ))}
          </tbody>
        </table>
      </div>

      {/* Infinite-Scroll-Sentinel */}
      {isInfinite && (
        <div ref={sentinelRef} className="flex justify-center py-6">
          {isFetchingNextPage && (
            <span className="text-sm text-neutral-400">Lade weitere Aktivitäten…</span>
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
            disabled={pageIndex === 0}
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

          <button
            onClick={() => goToPage(pageIndex + 1)}
            disabled={pageData?.next_cursor == null && pageIndex + 1 >= cursors.length}
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

function ActivityRow({ activity }: { activity: Activity }) {
  const label = sportLabel(activity.sport_type, activity.sub_type)
  const date = new Date(activity.started_at)
  const gain = formatElevationGain(activity.elevation_gain_m)
  const loss = formatElevationLoss(activity.elevation_loss_m)

  return (
    <tr className="hover:bg-neutral-50 dark:hover:bg-neutral-900/40 transition-colors">
      <td className="px-4 py-3 text-neutral-800 dark:text-neutral-200 font-medium">
        {activity.title ?? (
          <span className="text-neutral-400 font-normal italic">Kein Titel</span>
        )}
      </td>
      <td className="px-4 py-3">
        <span className="inline-flex items-center gap-1.5 text-neutral-500 dark:text-neutral-400">
          <SportIcon sport={activity.sport_type} subType={activity.sub_type} className="w-4 h-4 flex-shrink-0" />
          <span className="text-xs">{label}</span>
          {activity.is_indoor && <span className="text-neutral-400 dark:text-neutral-600 text-xs">indoor</span>}
        </span>
      </td>
      <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400">
        {format(date, 'EEE, d. MMM yyyy', { locale: de })}
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
        ) : null}
      </td>
    </tr>
  )
}
