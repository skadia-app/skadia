import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchSegments, fetchSegmentLeaderboard, deleteSegment, deleteSegments, recomputeSegment, exportSegments, type Segment } from '@/api/segments'
import { getMe } from '@/api/auth'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { fmtDuration, fmtDist } from '@/lib/format'
import SegmentEditorModal from '@/components/SegmentEditorModal'
import SegmentEditModal from '@/components/SegmentEditModal'
import DifficultyBadge from '@/components/DifficultyBadge'
import { Lock, Trash2 } from 'lucide-react'

const SPORT_GROUP_LABELS: Record<string, string> = {
  bike: 'Radfahren',
  foot: 'Laufsport',
  swim: 'Schwimmen',
  ski: 'Wintersport',
  paddle: 'Wassersport',
  generic: 'Sonstiges',
}

const SPORT_GROUP_ICONS: Record<string, string> = {
  bike: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  foot: 'M13.5 5.5c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2z',
  swim: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  ski: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  paddle: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
  generic: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z',
}

const SPORT_GROUP_COLORS: Record<string, string> = {
  bike: 'text-accent-DEFAULT',
  foot: 'text-green-500',
  swim: 'text-cyan-500',
  ski: 'text-blue-500',
  paddle: 'text-blue-400',
  generic: 'text-neutral-500',
}

const SEGMENT_TYPE_LABELS: Record<string, string> = {
  standard: 'Standard',
  trail: 'Trail',
  lift: 'Lift',
}


export default function SegmentsPage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { openModal } = useModal()
  const { addToast } = useToast()
  const { isJobActiveForEntity } = useActiveJobs()
  const { hideUnused } = useGlobalSportSettings()
  const [localPendingIds, setLocalPendingIds] = useState<Set<string>>(new Set())
  const [sportGroup, setSportGroup] = useState('')
  const [terrainFilter, setTerrainFilter] = useState<string[]>([])
  const [showAllLifts, setShowAllLifts] = useState(false)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [selectedSegmentId, setSelectedSegmentId] = useState<string | null>(null)
  const [editorMode, setEditorMode] = useState<'draw' | null>(null)
  const [editingSegment, setEditingSegment] = useState<Segment | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [selectionMode, setSelectionMode] = useState(false)
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)

  const { data: me } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 60_000,
  })

  // Fetch all sport groups that have at least one segment (for hideUnused filtering)
  const { data: allGroupsData } = useQuery({
    queryKey: ['segments-all-groups'],
    queryFn: () => fetchSegments({ segment_types: 'standard,trail,lift', search: undefined }),
    enabled: hideUnused,
    staleTime: 60_000,
  })
  const usedSportGroups = useMemo<Set<string>>(() => {
    if (!allGroupsData) return new Set(Object.keys(SPORT_GROUP_LABELS))
    return new Set(allGroupsData.segments.map(s => s.sport_group))
  }, [allGroupsData])
  const visibleSportGroups = useMemo(() => {
    if (!hideUnused || !allGroupsData) return Object.entries(SPORT_GROUP_LABELS)
    return Object.entries(SPORT_GROUP_LABELS).filter(([key]) => usedSportGroups.has(key))
  }, [hideUnused, allGroupsData, usedSportGroups])

  // Debounced search
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    searchTimerRef.current = setTimeout(() => setSearch(value), 300)
  }, [])

  const filterParams = useMemo(() => ({
    sport_group: sportGroup || undefined,
    search: search || undefined,
    segment_types: terrainFilter.length === 0
      ? 'standard,trail'
      : terrainFilter.join(','),
    only_ridden: terrainFilter.includes('lift') && !showAllLifts ? true : undefined,
  }), [sportGroup, search, terrainFilter, showAllLifts])

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
  } = useInfiniteQuery({
    queryKey: ['segments', filterParams],
    queryFn: ({ pageParam }) => fetchSegments({ ...filterParams, cursor: pageParam }),
    getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.next_cursor : undefined,
    initialPageParam: undefined as string | undefined,
  })

  const segments = useMemo(
    () => data?.pages.flatMap(p => p.segments) ?? [],
    [data]
  )

  // IntersectionObserver for infinite scroll
  const sentinelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = sentinelRef.current
    if (!el || !hasNextPage) return
    const observer = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting) fetchNextPage() },
      { rootMargin: '200px' }
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [fetchNextPage, hasNextPage])

  const deleteMut = useMutation({
    mutationFn: deleteSegment,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['segments'] })
    },
  })

  const recomputeMut = useMutation({
    mutationFn: recomputeSegment,
    onMutate: (segmentId) => {
      setLocalPendingIds(prev => new Set(prev).add(segmentId))
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
    },
    onSuccess: (_data, segmentId) => {
      setTimeout(() => queryClient.invalidateQueries({ queryKey: ['segments'] }), 3000)
      // Nach 10s bereinigen — bis dahin hat der Poll den Job längst bestätigt
      setTimeout(() => {
        setLocalPendingIds(prev => { const s = new Set(prev); s.delete(segmentId); return s })
      }, 10000)
    },
    onError: (_err, segmentId) => {
      setLocalPendingIds(prev => { const s = new Set(prev); s.delete(segmentId); return s })
    },
  })

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const bulkDeleteMut = useMutation({
    mutationFn: () => deleteSegments([...selectedIds]),
    onSuccess: ({ deleted }) => {
      const msg = deleted === 1 ? '1 Segment gelöscht' : `${deleted} Segmente gelöscht`
      addToast({ type: 'success', title: msg })
      setSelectedIds(new Set())
      setSelectionMode(false)
      setConfirmBulkDelete(false)
      queryClient.invalidateQueries({ queryKey: ['segments'] })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Löschen fehlgeschlagen', message: 'Die Segmente konnten nicht gelöscht werden', duration: 0 })
      setConfirmBulkDelete(false)
    },
  })

  const handleBulkExport = async (format: 'gpx' | 'geojson') => {
    try {
      const { blob, filename } = await exportSegments([...selectedIds], format)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename ?? 'segments.zip'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch {
      addToast({ type: 'error', title: 'Export fehlgeschlagen', message: 'Die Segmente konnten nicht exportiert werden', duration: 0 })
    }
  }

  const handleDelete = (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    openModal({
      title: 'Segment löschen?',
      message: 'Löschen ist nicht rückgängig zu machen.',
      buttons: [
        {
          label: 'Abbrechen',
          variant: 'default',
          onClick: () => {}
        },
        {
          label: 'Löschen',
          variant: 'destructive',
          onClick: () => deleteMut.mutate(id)
        }
      ]
    })
  }

  const handleRecompute = (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    recomputeMut.mutate(id)
  }

  function isSegmentRecomputing(segId: string): boolean {
    return (recomputeMut.isPending && recomputeMut.variables === segId)
      || localPendingIds.has(segId)
      || isJobActiveForEntity(segId)
  }

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Segmente...</div>
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Segmente</h1>
        <div className="flex items-center gap-2">
          <Link
            to="/map"
            state={{ activateSegmentsOverlay: true }}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 hover:border-neutral-300 dark:hover:border-neutral-600 transition-colors"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
            </svg>
            Auf Karte
          </Link>
          <button onClick={() => setEditorMode('draw')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-accent-100 dark:hover:bg-accent-100/20 hover:border-accent-400 dark:hover:border-accent-600 hover:text-accent-600 dark:hover:text-accent-400 cursor-pointer transition-colors">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Segment zeichnen
          </button>
          <button
            onClick={() => { setSelectionMode(m => !m); setSelectedIds(new Set()) }}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border cursor-pointer transition-colors ${
              selectionMode
                ? 'border-neutral-400 dark:border-neutral-500 bg-neutral-100 dark:bg-neutral-800 text-neutral-800 dark:text-neutral-200'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 hover:border-neutral-300 dark:hover:border-neutral-600'
            }`}
          >
            {selectionMode ? 'Fertig' : 'Auswählen'}
          </button>
        </div>
      </div>

      {/* Editor Modal */}
      {editorMode && (
        <SegmentEditorModal
          mode={editorMode}
          onSuccess={() => {
            setEditorMode(null)
            queryClient.invalidateQueries({ queryKey: ['segments'] })
          }}
          onClose={() => setEditorMode(null)}
        />
      )}

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-3 mb-6">
        <select
          value={sportGroup}
          onChange={e => setSportGroup(e.target.value)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Sportarten</option>
          {visibleSportGroups.map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>

        <div className="flex items-center gap-3 flex-wrap">
          {(Object.entries(SEGMENT_TYPE_LABELS) as [string, string][]).map(([id, label]) => (
            <label key={id} className="flex items-center gap-1.5 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={terrainFilter.includes(id)}
                onChange={e => {
                  if (e.target.checked) {
                    setTerrainFilter(prev => [...prev, id])
                  } else {
                    setTerrainFilter(prev => prev.filter(t => t !== id))
                    if (id === 'lift') setShowAllLifts(false)
                  }
                }}
                className="w-3.5 h-3.5 rounded accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
            </label>
          ))}
          {terrainFilter.includes('lift') && (
            <label className="flex items-center gap-1.5 cursor-pointer select-none border-l border-neutral-200 dark:border-neutral-700 pl-3">
              <input
                type="checkbox"
                checked={showAllLifts}
                onChange={e => setShowAllLifts(e.target.checked)}
                className="w-3.5 h-3.5 rounded accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-500 dark:text-neutral-400">Alle anzeigen</span>
            </label>
          )}
        </div>

        <div className="relative">
          <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-neutral-400 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
          </svg>
          <input
            type="text"
            value={searchInput}
            onChange={e => handleSearchChange(e.target.value)}
            placeholder="Segment suchen..."
            className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 pl-8 text-sm w-56
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
        </div>
      </div>

      {/* Bulk Action Toolbar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 p-3 bg-neutral-100 dark:bg-neutral-800 rounded-lg mb-4">
          <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {selectedIds.size === 1 ? '1 ausgewählt' : `${selectedIds.size} ausgewählt`}
          </span>
          <button
            onClick={() => setSelectedIds(new Set(segments.map((s: Segment) => s.id)))}
            className="px-2.5 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
          >
            Alle auswählen
          </button>
          <button
            onClick={() => setSelectedIds(new Set())}
            className="px-2.5 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
          >
            Auswahl aufheben
          </button>
          <button
            onClick={() => handleBulkExport('gpx')}
            className="px-2.5 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
          >
            GPX exportieren
          </button>
          <button
            onClick={() => handleBulkExport('geojson')}
            className="px-2.5 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-neutral-700 cursor-pointer transition-colors"
          >
            GeoJSON exportieren
          </button>
          <button
            onClick={() => setConfirmBulkDelete(true)}
            className="px-2.5 py-1 text-xs font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 cursor-pointer transition-colors"
          >
            Löschen
          </button>
        </div>
      )}

      {/* Bulk Delete Confirmation Dialog */}
      {confirmBulkDelete && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setConfirmBulkDelete(false)}>
          <div className="bg-white dark:bg-neutral-900 p-6 rounded-xl shadow-2xl max-w-sm w-full mx-4" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-neutral-900 dark:text-neutral-100 mb-2">Segmente löschen?</h3>
            <p className="text-sm text-neutral-600 dark:text-neutral-400 mb-5">
              {selectedIds.size === 1
                ? '1 Segment wird unwiderruflich gelöscht.'
                : `${selectedIds.size} Segmente werden unwiderruflich gelöscht.`}
              {' '}OSM-Liftabschnitte werden dabei übersprungen.
            </p>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setConfirmBulkDelete(false)}
                className="px-3 py-1.5 text-sm font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
              >
                Abbrechen
              </button>
              <button
                onClick={() => bulkDeleteMut.mutate()}
                disabled={bulkDeleteMut.isPending}
                className="px-3 py-1.5 text-sm font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                {bulkDeleteMut.isPending ? 'Lösche...' : 'Endgültig löschen'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Segment Cards */}
      {segments.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-48 text-neutral-400 dark:text-neutral-500">
          <svg className="w-12 h-12 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
          </svg>
          <p className="text-sm">Noch keine Segmente vorhanden</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {segments.map(segment => (
            <div
              key={segment.id}
              onClick={() => selectionMode ? toggleSelect(segment.id) : navigate(`/segments/${segment.id}`)}
              className={`relative rounded-xl border p-4 transition-all cursor-pointer group ${
                selectionMode && selectedIds.has(segment.id)
                  ? 'border-accent-DEFAULT dark:border-accent-DEFAULT bg-accent-50 dark:bg-accent-900/20 shadow-sm'
                  : 'border-neutral-200 dark:border-neutral-800 hover:border-neutral-300 dark:hover:border-neutral-700 hover:shadow-sm'
              } ${
                segment.visibility === 'private' && segment.created_by !== me?.user_id
                  ? 'opacity-60 ring-1 ring-neutral-600'
                  : ''
              }`}
            >
              {selectionMode && (
                <input
                  type="checkbox"
                  checked={selectedIds.has(segment.id)}
                  onChange={() => toggleSelect(segment.id)}
                  onClick={e => e.stopPropagation()}
                  className="absolute top-3 left-3 h-4 w-4 accent-primary z-10 cursor-pointer"
                />
              )}
              <div className={`flex items-start justify-between mb-2 ${selectionMode ? 'pl-6' : ''}`}>
                <div className="flex items-center gap-2 min-w-0">
                  <svg className={`w-5 h-5 flex-shrink-0 ${SPORT_GROUP_COLORS[segment.sport_group] ?? 'text-neutral-500'}`} viewBox="0 0 24 24" fill="currentColor">
                    <path d={SPORT_GROUP_ICONS[segment.sport_group] ?? SPORT_GROUP_ICONS.generic} />
                  </svg>
                  <h3 className="font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors truncate">
                    {segment.name}
                  </h3>
                  {segment.visibility === 'private' && (
                    <Lock className="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />
                  )}
                  {segment.segment_type === 'trail' && (
                    <DifficultyBadge meta={segment.segment_type_meta ?? {}} />
                  )}
                </div>
                <div className={`flex items-center gap-1 flex-shrink-0 ${selectionMode ? 'hidden' : isSegmentRecomputing(segment.id) ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}>
                  <button
                    onClick={(e) => { e.stopPropagation(); setEditingSegment(segment) }}
                    className="p-1 rounded text-neutral-400 hover:text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-100/20 cursor-pointer transition-colors"
                    title="Segment bearbeiten"
                  >
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                    </svg>
                  </button>
                  <button
                    onClick={(e) => handleRecompute(e, segment.id)}
                    disabled={isSegmentRecomputing(segment.id)}
                    className={`p-1 rounded cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${isSegmentRecomputing(segment.id) ? 'text-accent-DEFAULT' : 'text-neutral-400 hover:text-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20'}`}
                    title="Matching neu berechnen"
                  >
                    <svg className={`w-4 h-4 ${isSegmentRecomputing(segment.id) ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                  </button>
                  {/* Show delete if creator OR admin. Admin sees all segments including private. */}
                  {segment.segment_type !== 'lift' && (segment.created_by === me?.user_id || me?.is_admin) && (
                    <button
                      onClick={(e) => handleDelete(e, segment.id)}
                      className="p-1 rounded text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 cursor-pointer transition-colors"
                      title="Segment löschen"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  )}
                </div>
              </div>

              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-neutral-500 dark:text-neutral-400">
                {segment.distance_m != null && (
                  <span>{fmtDist(segment.distance_m)}</span>
                )}
                {segment.elevation_gain_m != null && segment.elevation_gain_m > 0 && (
                  <span>+{Math.round(segment.elevation_gain_m)} m</span>
                )}
                <span>
                  {segment.effort_count} {segment.effort_count === 1 ? 'Effort' : 'Efforts'}
                  {segment.ridden_count > 0 && (
                    <span className="text-neutral-400 ml-1">· {segment.ridden_count} eigene</span>
                  )}
                </span>
              </div>

              <div className="flex items-center gap-2 mt-2">
                <span className="inline-block px-2 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
                  {SPORT_GROUP_LABELS[segment.sport_group] ?? segment.sport_group}
                </span>
                {segment.segment_type !== 'standard' && (
                  <span className="inline-block px-2 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400">
                    {SEGMENT_TYPE_LABELS[segment.segment_type] ?? segment.segment_type}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Infinite Scroll Sentinel */}
      {hasNextPage && (
        <div className="h-8 flex items-center justify-center mt-4">
          {isFetchingNextPage ? (
            <span className="text-sm text-neutral-400">Weitere laden...</span>
          ) : (
            <div ref={sentinelRef} />
          )}
        </div>
      )}

      {/* Leaderboard Modal */}
      {selectedSegmentId && (
        <LeaderboardModal
          segmentId={selectedSegmentId}
          segmentName={segments.find(s => s.id === selectedSegmentId)?.name ?? 'Segment'}
          onClose={() => setSelectedSegmentId(null)}
        />
      )}

      {/* Edit Modal */}
      {editingSegment && (
        <SegmentEditModal segment={editingSegment} onClose={() => setEditingSegment(null)} />
      )}
    </div>
  )
}

function LeaderboardModal({ segmentId, segmentName, onClose }: { segmentId: string; segmentName: string; onClose: () => void }) {
  const [direction, setDirection] = useState<'all' | 'forward' | 'reverse'>('all')
  const [scope, setScope] = useState<'all' | 'mine'>('all')

  const isReverse = direction === 'all' ? undefined : direction === 'forward' ? false : true

  const { data: entries = [], isLoading } = useQuery({
    queryKey: ['segment-leaderboard', segmentId, direction, scope],
    queryFn: () => fetchSegmentLeaderboard(segmentId, isReverse, scope === 'mine'),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-2xl w-full mx-4 max-h-[80vh] flex flex-col"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-neutral-200 dark:border-neutral-800">
          <div>
            <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">Bestenliste</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400">{segmentName}</p>
          </div>
          <div className="flex items-center gap-3">
            {/* Scope: Alle / Nur ich */}
            <div className="flex gap-1">
              {(['all', 'mine'] as const).map(s => (
                <button key={s}
                  onClick={() => setScope(s)}
                  className={`px-2 py-1 text-xs rounded cursor-pointer ${scope === s ? 'bg-neutral-700 text-white dark:bg-neutral-200 dark:text-neutral-900' : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'}`}
                >
                  {s === 'all' ? 'Alle' : 'Nur ich'}
                </button>
              ))}
            </div>
            {/* Direction */}
            <div className="flex gap-1">
              {(['all', 'forward', 'reverse'] as const).map(dir => (
                <button key={dir}
                  onClick={() => setDirection(dir)}
                  className={`px-2 py-1 text-xs rounded cursor-pointer ${direction === dir ? 'bg-accent-DEFAULT text-white' : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'}`}
                >
                  {dir === 'all' ? 'Alle' : dir === 'forward' ? '→' : '←'}
                </button>
              ))}
            </div>
            <button
              onClick={onClose}
              className="cursor-pointer p-2 rounded-lg text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            >
              <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <div className="overflow-y-auto flex-1 p-4">
          {isLoading ? (
            <div className="flex items-center justify-center h-32 text-neutral-400">Lade Bestenliste...</div>
          ) : entries.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-neutral-400">Noch keine Einträge</div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 dark:border-neutral-700">
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Rang</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Athlet</th>
                  <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Zeit</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Datum</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Ausrüstung</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry, i) => (
                  <tr
                    key={`${entry.activity_id}-${i}`}
                    className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
                  >
                    <td className="px-3 py-2 text-neutral-900 dark:text-neutral-100">
                      {entry.rank_overall != null ? (
                        entry.rank_overall === 1 ? (
                          <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-bold text-xs">
                            1
                          </span>
                        ) : (
                          <span className="text-neutral-600 dark:text-neutral-400">#{entry.rank_overall}</span>
                        )
                      ) : (
                        <span className="text-neutral-600 dark:text-neutral-400">–</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-neutral-900 dark:text-neutral-100">{entry.display_name}</td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100">
                      {fmtDuration(entry.elapsed_time_s)}
                    </td>
                    <td className="px-3 py-2 text-neutral-500 dark:text-neutral-400">
                      {new Date(entry.started_at).toLocaleDateString('de-DE')}
                    </td>
                    <td className="px-3 py-2 text-neutral-500 dark:text-neutral-400">
                      {entry.gear_name ?? '--'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}
