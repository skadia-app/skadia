import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchSegments, deleteSegment, recomputeSegment, type Segment } from '@/api/segments'
import { getMe } from '@/api/auth'
import { useModal } from '@/lib/modal'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { fmtDist } from '@/lib/format'
import SegmentEditorModal from '@/components/SegmentEditorModal'
import SegmentEditModal from '@/components/SegmentEditModal'
import DifficultyBadge from '@/components/DifficultyBadge'
import { Lock, Trash2, FilterX, ChevronDown, ChevronUp } from 'lucide-react'
import { SAC_SCALE_OPTIONS, MTB_SCALE_OPTIONS, SURFACE_DROPDOWN_OPTIONS } from '@/components/segment-editor/constants'

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
  const sacFilter = searchParams.get('sac_scale')?.split(',').filter(Boolean) ?? []
  const mtbFilter = searchParams.get('mtb_scale')?.split(',').filter(Boolean) ?? []
  const surfaceFilter = searchParams.get('surface')?.split(',').filter(Boolean) ?? []
  const visibility = searchParams.get('visibility') ?? ''

  const [showAdvanced, setShowAdvanced] = useState(() => 
    sacFilter.length > 0 || mtbFilter.length > 0 || surfaceFilter.length > 0 || !!visibility
  )
  
  const setFilter = (key: string, value: string | string[] | null) => {
    const next = new URLSearchParams(searchParams)
    const strValue = Array.isArray(value) ? value.join(',') : value
    if (strValue) next.set(key, strValue)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { openModal } = useModal()
  const { isJobActiveForEntity } = useActiveJobs()
  const { hideUnused } = useGlobalSportSettings()
  const [localPendingIds, setLocalPendingIds] = useState<Set<string>>(new Set())
  const [searchInput, setSearchInput] = useState(search)
  const [editorMode, setEditorMode] = useState<'draw' | null>(null)
  const [editingSegment, setEditingSegment] = useState<Segment | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [selectionMode, setSelectionMode] = useState(false)

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
    sac_scale: sacFilter.length > 0 ? sacFilter.join(',') : undefined,
    mtb_scale: mtbFilter.length > 0 ? mtbFilter.join(',') : undefined,
    surface: surfaceFilter.length > 0 ? surfaceFilter.join(',') : undefined,
    visibility: (visibility as any) || undefined,
  }), [sportGroup, search, terrainFilter, showAllLifts, sacFilter, mtbFilter, surfaceFilter, visibility])

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

  const activeFilterCount = (sacFilter.length + mtbFilter.length + surfaceFilter.length + (visibility ? 1 : 0))

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Segmente</h1>
        <div className="flex items-center gap-2">
          <Link to="/map" state={{ activateSegmentsOverlay: true }} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors">
            Auf Karte
          </Link>
          <button onClick={() => setEditorMode('draw')} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-accent-100 dark:hover:bg-accent-100/20 hover:text-accent-600 dark:hover:text-accent-400 cursor-pointer transition-colors">
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
      <div className="space-y-3 mb-6">
        <div className="flex flex-wrap items-center gap-3 bg-neutral-50 dark:bg-neutral-800/40 p-3 rounded-xl border border-neutral-200 dark:border-neutral-800">
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
            <input
              type="text"
              value={searchInput}
              onChange={e => handleSearchChange(e.target.value)}
              placeholder="Segment suchen..."
              className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm pl-8 pr-3 py-1.5 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
            />
          </div>

          <button
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg text-neutral-600 dark:text-neutral-400 hover:text-accent-DEFAULT transition-colors cursor-pointer"
          >
            {showAdvanced ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
            Erweiterte Filter
            {activeFilterCount > 0 && (
              <span className="ml-1 rounded-full bg-accent-DEFAULT/20 px-2 text-[10px] text-accent-DEFAULT">
                {activeFilterCount}
              </span>
            )}
          </button>

          {(sportGroup || activeFilterCount > 0 || search) && (
            <button
              onClick={() => {
                setSearchParams(new URLSearchParams(), { replace: true })
                setSearchInput('')
              }}
              className="p-1.5 rounded-lg text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors cursor-pointer"
              title="Filter zurücksetzen"
            >
              <FilterX className="w-4 h-4" />
            </button>
          )}
        </div>

        {showAdvanced && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 p-4 bg-neutral-50/50 dark:bg-neutral-800/20 rounded-xl border border-neutral-200 dark:border-neutral-800 animate-in fade-in slide-in-from-top-2 duration-200">
            {/* SAC Scale */}
            <div className="space-y-2">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 uppercase">SAC-Skala</label>
              <div className="flex flex-wrap gap-x-3 gap-y-1.5">
                {SAC_SCALE_OPTIONS.map(opt => (
                  <label key={opt.value} className="flex items-center gap-1.5 cursor-pointer group">
                    <input
                      type="checkbox"
                      checked={sacFilter.includes(opt.value)}
                      onChange={e => {
                        const next = e.target.checked ? [...sacFilter, opt.value] : sacFilter.filter(v => v !== opt.value)
                        setFilter('sac_scale', next.length > 0 ? next : null)
                      }}
                      className="w-3.5 h-3.5 rounded accent-accent-DEFAULT"
                    />
                    <span className="text-xs text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-200">{opt.value}</span>
                  </label>
                ))}
              </div>
            </div>

            {/* MTB Scale */}
            <div className="space-y-2">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 uppercase">MTB-Skala</label>
              <div className="flex flex-wrap gap-x-3 gap-y-1.5">
                {MTB_SCALE_OPTIONS.map(opt => (
                  <label key={opt.value} className="flex items-center gap-1.5 cursor-pointer group">
                    <input
                      type="checkbox"
                      checked={mtbFilter.includes(opt.value)}
                      onChange={e => {
                        const next = e.target.checked ? [...mtbFilter, opt.value] : mtbFilter.filter(v => v !== opt.value)
                        setFilter('mtb_scale', next.length > 0 ? next : null)
                      }}
                      className="w-3.5 h-3.5 rounded accent-accent-DEFAULT"
                    />
                    <span className="text-xs text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-200">{opt.value}</span>
                  </label>
                ))}
              </div>
            </div>

            {/* Surface */}
            <div className="space-y-2">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 uppercase">Oberfläche</label>
              <select
                value={surfaceFilter[0] || ''}
                onChange={e => setFilter('surface', e.target.value || null)}
                className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-xs bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
              >
                <option value="">Alle Oberflächen</option>
                {SURFACE_DROPDOWN_OPTIONS.filter(o => o !== 'Wechselnd').map(opt => (
                  <option key={opt} value={opt}>{opt}</option>
                ))}
              </select>
            </div>

            {/* Visibility */}
            <div className="space-y-2">
              <label className="text-[11px] font-semibold tracking-wider text-neutral-500 uppercase">Sichtbarkeit</label>
              <select
                value={visibility}
                onChange={e => setFilter('visibility', e.target.value || null)}
                className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-xs bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-neutral-400"
              >
                <option value="">Alle Sichtbarkeiten</option>
                <option value="public">Öffentlich</option>
                <option value="private">Nur meine privaten</option>
              </select>
            </div>
          </div>
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
                  <DifficultyBadge 
                    sac={segment.segment_type_meta?.sac_scale as string} 
                    mtb={segment.segment_type_meta?.mtb_scale as string} 
                    color={segment.segment_type_meta?.difficulty_color as string} 
                  />
                </div>
                <div className={`flex items-center gap-1 flex-shrink-0 ${selectionMode ? 'hidden' : isSegmentRecomputing(segment.id) ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}>
                  <button onClick={(e) => { e.stopPropagation(); setEditingSegment(segment) }} className="p-1 rounded text-neutral-400 hover:text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-100/20 cursor-pointer"><svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg></button>
                  <button onClick={(e) => handleRecompute(e, segment.id)} disabled={isSegmentRecomputing(segment.id)} className={`p-1 rounded cursor-pointer ${isSegmentRecomputing(segment.id) ? 'text-accent-DEFAULT' : 'text-neutral-400 hover:text-blue-500'}`}><svg className={`w-4 h-4 ${isSegmentRecomputing(segment.id) ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg></button>
                  {segment.segment_type !== 'lift' && (segment.created_by === me?.user_id || me?.is_admin) && (
                    <button onClick={(e) => handleDelete(e, segment.id)} className="p-1 rounded text-neutral-400 hover:text-red-500 cursor-pointer"><Trash2 className="w-4 h-4" /></button>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-neutral-500">
                  {segment.distance_m != null && <span>{fmtDist(segment.distance_m)}</span>}
                  <span>{segment.effort_count} {segment.effort_count === 1 ? 'Effort' : 'Efforts'}</span>
                  {!!segment.segment_type_meta?.surface && (
                    <span className="text-neutral-400 italic">{String(segment.segment_type_meta.surface)}</span>
                  )}
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
