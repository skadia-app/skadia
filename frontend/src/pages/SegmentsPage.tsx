import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchSegments, fetchSegmentLeaderboard, deleteSegment, deleteSegments, recomputeSegment, exportSegments, type Segment } from '@/api/segments'
import { getMe } from '@/api/auth'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { fmtDuration, fmtDist } from '@/lib/format'
import { triggerBlobDownload } from '@/lib/utils'
import SegmentEditorModal from '@/components/SegmentEditorModal'
import SegmentEditModal from '@/components/SegmentEditModal'
import DifficultyBadge from '@/components/DifficultyBadge'
import { Lock, Trash2, FilterX } from 'lucide-react'

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
  lap: 'Runde',
  lift: 'Lift',
  wintersport: 'Wintersport',
  wassersport: 'Wassersport',
}


export default function SegmentsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  
  const sportGroup = searchParams.get('sport_group') ?? ''
  const terrainFilter = searchParams.get('segment_types')?.split(',') ?? ['standard', 'trail', 'wintersport', 'wassersport']
  const showAllLifts = searchParams.get('show_lifts') === 'true'
  const search = searchParams.get('search') ?? ''
  const sacScale = searchParams.get('sac_scale') ?? ''
  const mtbScale = searchParams.get('mtb_scale') ?? ''
  const visibility = searchParams.get('visibility') ?? ''
  
  const setFilter = (key: string, value: string | null) => {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { openModal } = useModal()
  const { addToast } = useToast()
  const { isJobActiveForEntity } = useActiveJobs()
  const { hideUnused } = useGlobalSportSettings()
  const [localPendingIds, setLocalPendingIds] = useState<Set<string>>(new Set())
  const [searchInput, setSearchInput] = useState(search)
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

  // Fetch all sport groups for hideUnused filtering
  const { data: allGroupsData } = useQuery({
    queryKey: ['segments-all-groups'],
    queryFn: () => fetchSegments({ segment_types: 'standard,trail,lift,wintersport,wassersport', limit: 500 }),
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
    searchTimerRef.current = setTimeout(() => setFilter('search', value || null), 300)
  }, [searchParams])

  const filterParams = useMemo(() => ({
    sport_group: sportGroup || undefined,
    search: search || undefined,
    segment_types: terrainFilter.length === 0
      ? 'standard,trail,wintersport,wassersport'
      : terrainFilter.join(','),
    only_ridden: terrainFilter.includes('lift') && !showAllLifts ? true : undefined,
    sac_scale: sacScale || undefined,
    mtb_scale: mtbScale || undefined,
    visibility: (visibility as any) || undefined,
  }), [sportGroup, search, terrainFilter, showAllLifts, sacScale, mtbScale, visibility])

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    refetch,
  } = useInfiniteQuery({
    queryKey: ['segments', filterParams],
    queryFn: ({ pageParam }) => fetchSegments({ ...filterParams, cursor: pageParam, limit: 50 }),
    getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.next_cursor : undefined,
    initialPageParam: undefined as string | undefined,
  })

  const segments = useMemo(
    () => data?.pages.flatMap(p => p.segments) ?? [],
    [data]
  )

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
      const result = await exportSegments([...selectedIds], format)
      triggerBlobDownload(result.blob, result.filename ?? 'segments.zip')
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
        { label: 'Abbrechen', variant: 'default', onClick: () => {} },
        { label: 'Löschen', variant: 'destructive', onClick: () => deleteMut.mutate(id) }
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

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-3 text-neutral-400">
        <p className="text-sm">Segmente konnten nicht geladen werden.</p>
        <button onClick={() => refetch()} className="px-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors">Erneut versuchen</button>
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Segmente</h1>
        <div className="flex items-center gap-2">
          <Link to="/map" state={{ activateSegmentsOverlay: true }} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" /></svg>
            Auf Karte
          </Link>
          <button onClick={() => setEditorMode('draw')} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-accent-100 dark:hover:bg-accent-100/20 hover:text-accent-600 dark:hover:text-accent-400 cursor-pointer transition-colors">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" /></svg>
            Segment zeichnen
          </button>
          <button
            onClick={() => { setSelectionMode(m => !m); setSelectedIds(new Set()) }}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border cursor-pointer transition-colors ${selectionMode ? 'border-neutral-400 dark:border-neutral-500 bg-neutral-100 dark:bg-neutral-800 text-neutral-800 dark:text-neutral-200' : 'border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'}`}
          >
            {selectionMode ? 'Fertig' : 'Auswählen'}
          </button>
        </div>
      </div>

      {editorMode && (
        <SegmentEditorModal
          mode={editorMode}
          onSuccess={() => { setEditorMode(null); queryClient.invalidateQueries({ queryKey: ['segments'] }) }}
          onClose={() => setEditorMode(null)}
        />
      )}

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-3 mb-6 bg-neutral-50 dark:bg-neutral-800/40 p-3 rounded-xl border border-neutral-200 dark:border-neutral-800">
        <select
          value={sportGroup}
          onChange={e => setFilter('sport_group', e.target.value || null)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Sportarten</option>
          {visibleSportGroups.map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>

        <select
          value={visibility}
          onChange={e => setFilter('visibility', e.target.value || null)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Sichtbarkeiten</option>
          <option value="public">Öffentlich</option>
          <option value="private">Privat</option>
        </select>

        <div className="flex items-center gap-2">
          <input
            placeholder="SAC"
            value={sacScale}
            onChange={e => setFilter('sac_scale', e.target.value || null)}
            className="w-16 border border-neutral-200 dark:border-neutral-700 rounded-lg px-2.5 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
          <input
            placeholder="MTB"
            value={mtbScale}
            onChange={e => setFilter('mtb_scale', e.target.value || null)}
            className="w-16 border border-neutral-200 dark:border-neutral-700 rounded-lg px-2.5 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
        </div>

        <div className="flex items-center gap-3 flex-wrap border-l border-neutral-200 dark:border-neutral-700 pl-3">
          {(Object.entries(SEGMENT_TYPE_LABELS) as [string, string][]).map(([id, label]) => (
            <label key={id} className="flex items-center gap-1.5 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={terrainFilter.includes(id)}
                onChange={e => {
                  let next: string[]
                  if (e.target.checked) next = [...terrainFilter, id]
                  else next = terrainFilter.filter(t => t !== id)
                  setFilter('segment_types', next.length > 0 ? next.join(',') : null)
                }}
                className="w-3.5 h-3.5 rounded accent-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
            </label>
          ))}
        </div>

        <div className="flex-1 min-w-[200px] relative">
          <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-neutral-400 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" /></svg>
          <input
            type="text"
            value={searchInput}
            onChange={e => handleSearchChange(e.target.value)}
            placeholder="Segment suchen..."
            className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm pl-8 pr-3 py-1.5 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
        </div>

        {(sportGroup || sacScale || mtbScale || visibility || search) && (
          <button
            onClick={() => {
              setSearchParams(new URLSearchParams(), { replace: true })
              setSearchInput('')
            }}
            className="p-1.5 rounded-lg text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
            title="Filter zurücksetzen"
          >
            <FilterX className="w-4 h-4" />
          </button>
        )}
      </div>

      {/* Segment Cards */}
      {segments.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-48 text-neutral-400">
           <p className="text-sm">Keine Segmente gefunden.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {segments.map(segment => (
            <div
              key={segment.id}
              onClick={() => selectionMode ? toggleSelect(segment.id) : navigate(`/segments/${segment.id}`)}
              className={`relative rounded-xl border p-4 transition-all cursor-pointer group ${
                selectionMode && selectedIds.has(segment.id)
                  ? 'border-accent-DEFAULT bg-accent-50 dark:bg-accent-900/20 shadow-sm'
                  : 'border-neutral-200 dark:border-neutral-800 hover:border-neutral-300 dark:hover:border-neutral-700 hover:shadow-sm'
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
                  <svg className={`w-5 h-5 flex-shrink-0 ${SPORT_GROUP_COLORS[segment.sport_group] ?? 'text-neutral-500'}`} viewBox="0 0 24 24" fill="currentColor"><path d={SPORT_GROUP_ICONS[segment.sport_group] ?? SPORT_GROUP_ICONS.generic} /></svg>
                  <h3 className="font-medium text-neutral-900 dark:text-neutral-100 truncate">{segment.name}</h3>
                  {segment.visibility === 'private' && <Lock className="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />}
                  {['trail', 'wintersport'].includes(segment.segment_type) && <DifficultyBadge sac={segment.segment_type_meta?.sac_scale as string} mtb={segment.segment_type_meta?.mtb_scale as string} color={segment.segment_type_meta?.difficulty_color as string} />}
                </div>
                <div className={`flex items-center gap-1 flex-shrink-0 ${selectionMode ? 'hidden' : isSegmentRecomputing(segment.id) ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}>
                  <button onClick={(e) => { e.stopPropagation(); setEditingSegment(segment) }} className="p-1 rounded text-neutral-400 hover:text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-100/20 cursor-pointer"><svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg></button>
                  <button onClick={(e) => handleRecompute(e, segment.id)} disabled={isSegmentRecomputing(segment.id)} className={`p-1 rounded cursor-pointer ${isSegmentRecomputing(segment.id) ? 'text-accent-DEFAULT' : 'text-neutral-400 hover:text-blue-500'}`}><svg className={`w-4 h-4 ${isSegmentRecomputing(segment.id) ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg></button>
                  {segment.segment_type !== 'lift' && (segment.created_by === me?.user_id || me?.is_admin) && (
                    <button onClick={(e) => handleDelete(e, segment.id)} className="p-1 rounded text-neutral-400 hover:text-red-500 cursor-pointer"><Trash2 className="w-4 h-4" /></button>
                  )}
                </div>
              </div>
              <div className="text-xs text-neutral-400">
                  {segment.distance_m != null && <span>{fmtDist(segment.distance_m)}</span>}
                  <span className="ml-2">{segment.effort_count} {segment.effort_count === 1 ? 'Effort' : 'Efforts'}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {hasNextPage && <div ref={sentinelRef} className="h-8 flex items-center justify-center mt-4">{isFetchingNextPage && <span className="text-sm text-neutral-400">Lädt...</span>}</div>}
      {editingSegment && <SegmentEditModal segment={editingSegment} onClose={() => setEditingSegment(null)} />}
    </div>
  )
}
