import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listRoutes,
  deleteRoute,
  duplicateRoute,
  exportRouteGPX,
  type PlannedRoute,
} from '@/api/routes'
import { useModal } from '@/lib/modal'
import { useToast } from '@/lib/toast'
import { fmtDist } from '@/lib/format'
import { triggerBlobDownload } from '@/lib/utils'

// ── Sport type labels for filter dropdown ─────────────────────────────────────

const SPORT_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: 'hiking', label: 'Wandern' },
  { value: 'running', label: 'Laufen' },
  { value: 'cycling', label: 'Radfahren' },
  { value: 'mountain_biking', label: 'Mountainbike' },
  { value: 'gravel_cycling', label: 'Gravel' },
  { value: 'trail_running', label: 'Trail Running' },
  { value: 'skiing', label: 'Ski Alpin' },
  { value: 'cross_country_skiing', label: 'Langlauf' },
  { value: 'swimming', label: 'Schwimmen' },
  { value: 'generic', label: 'Sonstiges' },
]

const SPORT_TYPE_LABELS: Record<string, string> = Object.fromEntries(
  SPORT_TYPE_OPTIONS.map(({ value, label }) => [value, label]),
)

const SPORT_TYPE_COLORS: Record<string, string> = {
  hiking: 'text-green-500',
  running: 'text-blue-500',
  trail_running: 'text-green-600',
  cycling: 'text-accent-DEFAULT',
  mountain_biking: 'text-orange-500',
  gravel_cycling: 'text-amber-500',
  skiing: 'text-cyan-500',
  cross_country_skiing: 'text-sky-400',
  swimming: 'text-blue-400',
  generic: 'text-neutral-500',
}

// ── Sort options ──────────────────────────────────────────────────────────────

type SortOption = 'created_at' | 'name' | 'distance' | 'elevation'

const SORT_LABELS: Record<SortOption, string> = {
  created_at: 'Neueste',
  name: 'Name A–Z',
  distance: 'Distanz',
  elevation: 'Höhenmeter',
}

// ── Icon helpers ──────────────────────────────────────────────────────────────

function SportIcon({ sportType, className }: { sportType: string; className?: string }) {
  const color = SPORT_TYPE_COLORS[sportType] ?? 'text-neutral-500'
  const cls = `${color} ${className ?? ''}`

  // Bike-family
  if (['cycling', 'mountain_biking', 'gravel_cycling'].includes(sportType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <circle cx="5.5" cy="17.5" r="3.5" />
        <circle cx="18.5" cy="17.5" r="3.5" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M15 6h-3l-2 5.5m0 0L8 17.5m2-6h5.5L18.5 17.5M10 11.5l2-5.5" />
      </svg>
    )
  }
  // Run-family
  if (['running', 'trail_running'].includes(sportType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 5.5a2 2 0 100-4 2 2 0 000 4zm-2 2.5l-2 4 3 3-2 4m4-11l2 2-3 4" />
      </svg>
    )
  }
  // Ski
  if (['skiing', 'cross_country_skiing'].includes(sportType)) {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 18l18-6M8 15l3-9 3 3 4-5" />
      </svg>
    )
  }
  // Swimming
  if (sportType === 'swimming') {
    return (
      <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 17c1.5 0 2.5-1 4-1s2.5 1 4 1 2.5-1 4-1 2.5 1 4 1M3 13c1.5 0 2.5-1 4-1s2.5 1 4 1 2.5-1 4-1 2.5 1 4 1m1-6a2 2 0 11-4 0 2 2 0 014 0z" />
      </svg>
    )
  }
  // Default: hiking/map pin
  return (
    <svg className={cls} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 10.5a3 3 0 11-6 0 3 3 0 016 0z" />
      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1115 0z" />
    </svg>
  )
}

// ── Mini route preview (simplified SVG polyline) ──────────────────────────────

function RouteMiniMap({ geojson, className }: { geojson?: string; className?: string }) {
  const points = useMemo(() => {
    if (!geojson) return null
    try {
      const parsed = JSON.parse(geojson) as {
        type: string
        coordinates?: number[][]
        geometries?: { type: string; coordinates?: number[][] }[]
      }
      let coords: number[][] | null = null
      if (parsed.type === 'LineString' && parsed.coordinates) {
        coords = parsed.coordinates
      } else if (parsed.type === 'GeometryCollection' && parsed.geometries) {
        const ls = parsed.geometries.find((g) => g.type === 'LineString')
        coords = ls?.coordinates ?? null
      }
      if (!coords || coords.length < 2) return null

      // Normalize to SVG viewBox [0,100]x[0,100]
      const lons = coords.map((c) => c[0])
      const lats = coords.map((c) => c[1])
      const minLon = Math.min(...lons)
      const maxLon = Math.max(...lons)
      const minLat = Math.min(...lats)
      const maxLat = Math.max(...lats)
      const rangeX = maxLon - minLon || 1
      const rangeY = maxLat - minLat || 1

      // Uniformly subsample for performance (max 150 points)
      const step = Math.max(1, Math.floor(coords.length / 150))
      const sampled = coords.filter((_, i) => i % step === 0 || i === coords!.length - 1)

      return sampled
        .map((c) => {
          const x = ((c[0] - minLon) / rangeX) * 90 + 5
          const y = 95 - ((c[1] - minLat) / rangeY) * 90
          return `${x.toFixed(1)},${y.toFixed(1)}`
        })
        .join(' ')
    } catch {
      return null
    }
  }, [geojson])

  if (!points) {
    return (
      <div className={`flex items-center justify-center bg-neutral-100 dark:bg-neutral-800 rounded ${className ?? ''}`}>
        <svg className="w-6 h-6 text-neutral-300 dark:text-neutral-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
        </svg>
      </div>
    )
  }

  return (
    <div className={`bg-neutral-100 dark:bg-neutral-800 rounded overflow-hidden ${className ?? ''}`}>
      <svg viewBox="0 0 100 100" className="w-full h-full" preserveAspectRatio="xMidYMid meet">
        <polyline
          points={points}
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="text-accent-DEFAULT"
        />
      </svg>
    </div>
  )
}

// ── Format estimated time ─────────────────────────────────────────────────────

function fmtTime(minutes?: number): string {
  if (minutes == null) return '--'
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  if (h === 0) return `${m} min`
  if (m === 0) return `${h} h`
  return `${h} h ${m} min`
}

// ── Route Card ────────────────────────────────────────────────────────────────

interface RouteCardProps {
  route: PlannedRoute
  onEdit: () => void
  onDuplicate: () => void
  onDelete: () => void
  onExportGPX: () => void
  isDuplicating: boolean
  isDeleting: boolean
}

function RouteCard({
  route,
  onEdit,
  onDuplicate,
  onDelete,
  onExportGPX,
  isDuplicating,
  isDeleting,
}: RouteCardProps) {
  const navigate = useNavigate()
  const sportLabel = SPORT_TYPE_LABELS[route.sport_type] ?? route.sport_type

  return (
    <div
      onClick={() => navigate(`/routes/${route.id}`)}
      className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden
                 hover:border-neutral-300 dark:hover:border-neutral-700
                 hover:shadow-sm transition-all cursor-pointer group bg-white dark:bg-neutral-900"
    >
      {/* Mini map */}
      <RouteMiniMap
        geojson={route.geometry_geojson}
        className="h-28 w-full"
      />

      {/* Content */}
      <div className="p-3">
        {/* Name + actions */}
        <div className="flex items-start justify-between gap-2 mb-1.5">
          <h3 className="font-medium text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors text-sm leading-snug line-clamp-2 min-w-0">
            {route.name}
          </h3>
          <div
            className="flex items-center gap-0.5 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Edit */}
            <button
              onClick={onEdit}
              title="Route bearbeiten"
              className="p-1 rounded text-neutral-400 hover:text-accent-DEFAULT hover:bg-accent-100 dark:hover:bg-accent-100/20 cursor-pointer transition-colors"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
              </svg>
            </button>
            {/* Duplicate */}
            <button
              onClick={onDuplicate}
              disabled={isDuplicating}
              title="Duplizieren"
              className="p-1 rounded text-neutral-400 hover:text-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <svg className={`w-3.5 h-3.5 ${isDuplicating ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
            </button>
            {/* GPX Export */}
            <button
              onClick={onExportGPX}
              title="GPX exportieren"
              className="p-1 rounded text-neutral-400 hover:text-green-500 hover:bg-green-50 dark:hover:bg-green-900/20 cursor-pointer transition-colors"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </button>
            {/* Delete */}
            <button
              onClick={onDelete}
              disabled={isDeleting}
              title="Löschen"
              className="p-1 rounded text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
        </div>

        {/* Sport + difficulty */}
        <div className="flex items-center gap-1.5 mb-2">
          <SportIcon sportType={route.sport_type} className="w-3.5 h-3.5 flex-shrink-0" />
          <span className="text-xs text-neutral-500 dark:text-neutral-400">{sportLabel}</span>
          {route.difficulty && (
            <>
              <span className="text-neutral-300 dark:text-neutral-600">·</span>
              <span className="text-xs text-neutral-500 dark:text-neutral-400">{route.difficulty}</span>
            </>
          )}
        </div>

        {/* Stats row */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-neutral-500 dark:text-neutral-400">
          {route.total_distance_m != null && (
            <span className="flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
              </svg>
              {fmtDist(route.total_distance_m)}
            </span>
          )}
          {route.total_elevation_gain != null && route.total_elevation_gain > 0 && (
            <span className="flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 10l7-7m0 0l7 7m-7-7v18" />
              </svg>
              {Math.round(route.total_elevation_gain)} m
            </span>
          )}
          {route.estimated_time_min != null && (
            <span className="flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <circle cx="12" cy="12" r="9" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 7v5l3 3" />
              </svg>
              {fmtTime(route.estimated_time_min)}
            </span>
          )}
        </div>

        {/* Tags */}
        {route.tags && route.tags.length > 0 && (
          <div className="flex flex-wrap gap-1 mt-2">
            {route.tags.slice(0, 3).map((tag) => (
              <span
                key={tag.id}
                className="inline-block px-1.5 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400"
                style={tag.color ? { backgroundColor: `${tag.color}22`, color: tag.color } : undefined}
              >
                {tag.name}
              </span>
            ))}
            {route.tags.length > 3 && (
              <span className="inline-block px-1.5 py-0.5 text-xs rounded-full bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-500">
                +{route.tags.length - 3}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ── RoutesPage ────────────────────────────────────────────────────────────────

export default function RoutesPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { openModal } = useModal()
  const { addToast } = useToast()

  const [sportType, setSportType] = useState('')
  const [sortBy, setSortBy] = useState<SortOption>('created_at')
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')

  // Debounced search
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    searchTimerRef.current = setTimeout(() => setSearch(value), 300)
  }, [])

  // M4: Clean up debounce timer on unmount
  useEffect(() => () => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
  }, [])

  const queryParams = useMemo(
    () => ({
      sort: sortBy,
      sport_type: sportType || undefined,
      search: search || undefined,
    }),
    [sortBy, sportType, search],
  )

  const { data: routesResponse, isLoading, isError } = useQuery({
    queryKey: ['routes', queryParams],
    queryFn: () => listRoutes(queryParams),
    staleTime: 30_000,
  })

  const routes = routesResponse?.routes ?? []

  // Client-side filtering when backend doesn't support search
  const filteredRoutes = useMemo(() => {
    if (!search) return routes
    const q = search.toLowerCase()
    return routes.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        (r.description?.toLowerCase().includes(q) ?? false),
    )
  }, [routes, search])

  const deleteMut = useMutation({
    mutationFn: deleteRoute,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      addToast({ type: 'success', title: 'Route gelöscht' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Löschen', duration: 0 })
    },
  })

  const duplicateMut = useMutation({
    mutationFn: duplicateRoute,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      addToast({ type: 'success', title: 'Route dupliziert' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Duplizieren fehlgeschlagen', duration: 0 })
    },
  })

  const handleDelete = useCallback(
    (route: PlannedRoute) => {
      openModal({
        title: 'Route löschen?',
        message: `"${route.name}" wird unwiderruflich gelöscht.`,
        buttons: [
          { label: 'Abbrechen', variant: 'default', onClick: () => {} },
          { label: 'Löschen', variant: 'destructive', onClick: () => deleteMut.mutate(route.id) },
        ],
      })
    },
    [openModal, deleteMut],
  )

  const handleExportGPX = useCallback(
    async (route: PlannedRoute) => {
      try {
        const { blob, filename } = await exportRouteGPX(route.id)
        triggerBlobDownload(blob, filename || `route-${route.id}.gpx`)
        addToast({ type: 'success', title: 'GPX-Export erfolgreich' })
      } catch {
        addToast({ type: 'error', title: 'GPX-Export fehlgeschlagen', duration: 0 })
      }
    },
    [addToast],
  )

  const handleNewRoute = () => {
    navigate('/map', { state: { activateRoutePlanner: true } })
  }

  // ── Render ────────────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-neutral-400">
        Lade Routen...
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-3 text-neutral-400">
        <svg className="w-10 h-10" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126z" />
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 15.75h.007v.008H12v-.008z" />
        </svg>
        <p className="text-sm">Routen konnten nicht geladen werden.</p>
        <button
          onClick={() => queryClient.invalidateQueries({ queryKey: ['routes'] })}
          className="px-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
        >
          Erneut versuchen
        </button>
      </div>
    )
  }

  return (
    <div className="relative">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Routen</h1>
        <button
          onClick={handleNewRoute}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg
                     bg-accent-DEFAULT text-white hover:bg-accent-600 cursor-pointer transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          Neue Route
        </button>
      </div>

      {/* Filter / Sort bar */}
      <div className="flex flex-wrap items-center gap-3 mb-6">
        {/* Sport type */}
        <select
          value={sportType}
          onChange={(e) => setSportType(e.target.value)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          <option value="">Alle Sportarten</option>
          {SPORT_TYPE_OPTIONS.map(({ value, label }) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>

        {/* Sort */}
        <select
          value={sortBy}
          onChange={(e) => setSortBy(e.target.value as SortOption)}
          className="border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm
                     bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                     focus:outline-none focus:ring-1 focus:ring-neutral-400"
        >
          {(Object.entries(SORT_LABELS) as [SortOption, string][]).map(([key, label]) => (
            <option key={key} value={key}>{label}</option>
          ))}
        </select>

        {/* Search */}
        <div className="relative flex-1 min-w-[180px] max-w-xs">
          <svg
            className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-neutral-400 pointer-events-none"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
          </svg>
          <input
            type="text"
            value={searchInput}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Route suchen..."
            className="w-full border border-neutral-200 dark:border-neutral-700 rounded-lg px-3 py-1.5 pl-8 text-sm
                       bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-1 focus:ring-neutral-400"
          />
          {searchInput && (
            <button
              onClick={() => { setSearchInput(''); setSearch('') }}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>

        {/* Result count */}
        {filteredRoutes.length > 0 && (
          <span className="text-xs text-neutral-400 dark:text-neutral-500 ml-auto">
            {filteredRoutes.length} {filteredRoutes.length === 1 ? 'Route' : 'Routen'}
          </span>
        )}
      </div>

      {/* Empty state */}
      {filteredRoutes.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-64 text-neutral-400 dark:text-neutral-500 gap-4">
          <svg className="w-14 h-14" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={0.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l5.447 2.724A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
          </svg>
          {search || sportType ? (
            <>
              <p className="text-sm">Keine Routen gefunden.</p>
              <button
                onClick={() => { setSearch(''); setSearchInput(''); setSportType('') }}
                className="px-3 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
              >
                Filter zurücksetzen
              </button>
            </>
          ) : (
            <>
              <p className="text-sm font-medium">Noch keine Routen vorhanden</p>
              <p className="text-xs text-center max-w-xs">
                Plane deine erste Route im Routenplaner und speichere sie hier.
              </p>
              <button
                onClick={handleNewRoute}
                className="flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-lg
                           bg-accent-DEFAULT text-white hover:bg-accent-600 cursor-pointer transition-colors"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                </svg>
                Route planen
              </button>
            </>
          )}
        </div>
      ) : (
        /* Route grid */
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {filteredRoutes.map((route) => (
            <RouteCard
              key={route.id}
              route={route}
              onEdit={() => navigate('/map', { state: { activateRoutePlanner: true, editRouteId: route.id } })}
              onDuplicate={() => duplicateMut.mutate(route.id)}
              onDelete={() => handleDelete(route)}
              onExportGPX={() => handleExportGPX(route)}
              isDuplicating={duplicateMut.isPending && duplicateMut.variables === route.id}
              isDeleting={deleteMut.isPending && deleteMut.variables === route.id}
            />
          ))}
        </div>
      )}

      {/* FAB — visible on mobile (sm and below), otherwise "Neue Route" button in header is enough) */}
      <button
        onClick={handleNewRoute}
        title="Neue Route planen"
        className="fixed bottom-6 right-6 sm:hidden z-40 w-14 h-14 rounded-full shadow-lg
                   bg-accent-DEFAULT text-white hover:bg-accent-600 cursor-pointer transition-colors
                   flex items-center justify-center"
      >
        <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
        </svg>
      </button>
    </div>
  )
}
