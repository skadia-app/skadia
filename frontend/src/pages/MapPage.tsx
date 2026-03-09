import { useRef, useEffect, useState, useCallback, useMemo, type ChangeEvent, type KeyboardEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useLocation } from 'react-router-dom'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'
import CustomMapsPanel from '@/components/CustomMapsPanel'
import IndoorAssignPanel from '@/components/IndoorAssignPanel'
import ColorPicker from '@/components/ColorPicker'
import { getAllSportKeys, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { SPORT_DEFS, categoryForSport, sportColor } from '@/lib/sportTaxonomy'
import { BASE_STYLES, buildRasterStyle, type BaseStyle } from '@/lib/mapStyles'
import { useTheme } from '@/lib/theme'
import { useAuthImage } from '@/hooks/useAuthImage'
import { useModal } from '@/lib/modal'
import { getHeatmapSettings } from '@/api/settings'
import { listCustomMaps, listCustomMapMarkers, createCustomMapMarker, updateCustomMapMarker } from '@/api/customMaps'
import { listMapPhotos, type MapPhotoEntry } from '@/api/activityPhotos'
import { apiDownload } from '@/api/client'
import AuthenticatedImage from '@/components/AuthenticatedImage'
import { fetchSegments, type Segment } from '@/api/segments'
import { fetchTags, updateTag, mergeTags, fetchTagActivities, syncTagLocation, clearTagCoverImage, assignActivitiesToTag, type Tag, type TagActivity } from '@/api/tags'
import { fetchActivity, updateActivity } from '@/api/activities'
import { SegmentsOverlay, DEFAULT_SEGMENT_COLORS, DEFAULT_SEGMENT_FILTERS, type SegmentColorConfig, type SegmentsOverlayFilters } from '@/components/Map/SegmentsOverlay'
import { fetchTerraIncognita, type TerraIncognitaCoverage } from '@/api/terraIncognita'

// Re-export so existing consumers (if any) of the old MapPage exports still compile.
export type { SportOption } from '@/lib/sportOptions'
export { SPORT_OPTIONS, getAllSportKeys } from '@/lib/sportOptions'

type ViewMode = 'tracks' | 'heatmap' | 'terra_incognita'
const VALID_VIEW_MODES: readonly ViewMode[] = ['tracks', 'heatmap', 'terra_incognita']

const ALL_SPORT_KEYS = getAllSportKeys()

export interface SportColorConfig {
  mode: 'uniform' | 'per-type'
  uniformColor: string
  colors: Record<string, string>
}

export interface MapFilterState {
  sportTypes: Set<string>
  dateFrom: string
  dateTo: string
  opacity: number
  colorConfig: SportColorConfig
  showLocationTags: boolean
  heatmapColor: string
  tagFilters: import('@/components/TagFilterInput').TagFilter[]
  gearFilters: import('@/components/GearFilterInput').GearFilter[]
}

interface MapOverlayState {
  showPeaks: boolean
  peaksOnlyVisited: boolean
  showWaters: boolean
  showForests: boolean
  showVisitedPois: boolean
  showAmenities: boolean
  showSegmentsOverlay: boolean
}

const defaultOverlayState: MapOverlayState = {
  showPeaks: false,
  peaksOnlyVisited: false,
  showWaters: false,
  showForests: false,
  showVisitedPois: false,
  showAmenities: false,
  showSegmentsOverlay: false,
}

const PEAK_ICON_VISITED = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">' +
  '<path d="M2 20 L8 8 L12 14 L16 8 L22 20 Z" fill="#f97316" stroke="white" stroke-width="1.5"/>' +
  '</svg>'
)}`
const PEAK_ICON_UNVISITED = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">' +
  '<path d="M2 20 L8 8 L12 14 L16 8 L22 20 Z" fill="#9ca3af" stroke="white" stroke-width="1.5"/>' +
  '</svg>'
)}`

const defaultFilters: MapFilterState = {
  sportTypes: new Set(ALL_SPORT_KEYS),
  dateFrom: '',
  dateTo: '',
  opacity: 70,
  colorConfig: {
    mode: 'uniform',
    uniformColor: '#F97316',
    colors: { ...DEFAULT_SPORT_COLORS },
  },
  showLocationTags: true,
  heatmapColor: '#F97316',
  tagFilters: [],
  gearFilters: [],
}

function loadColorConfig(): SportColorConfig {
  try {
    const saved = localStorage.getItem('skadia-sport-colors')
    if (saved) return { ...defaultFilters.colorConfig, ...JSON.parse(saved) }
  } catch { /* ignorieren */ }
  return defaultFilters.colorConfig
}

function loadHeatmapColor(): string {
  try {
    const saved = localStorage.getItem('skadia-sport-colors')
    if (saved) {
      const parsed = JSON.parse(saved)
      if (typeof parsed.heatmapColor === 'string') return parsed.heatmapColor
    }
  } catch { /* ignorieren */ }
  return defaultFilters.heatmapColor
}

// ── Viewport persistence ─────────────────────────────────────────────────
const VIEWPORT_KEY = 'skadia-map-viewport'
const MAP_DEFAULTS_KEY = 'skadia-map-defaults'

interface StoredViewport {
  center: [number, number]
  zoom: number
  viewMode: ViewMode
  baseStyle: BaseStyle
}

function loadViewport(): StoredViewport | null {
  try {
    const s = localStorage.getItem(VIEWPORT_KEY)
    return s ? JSON.parse(s) : null
  } catch { return null }
}

function saveViewport(v: StoredViewport) {
  try { localStorage.setItem(VIEWPORT_KEY, JSON.stringify(v)) } catch { /* ignore */ }
}

function loadDefaultBaseStyle(): BaseStyle {
  try {
    const s = localStorage.getItem(MAP_DEFAULTS_KEY)
    if (s) {
      const p = JSON.parse(s)
      if (p.defaultBaseStyle && p.defaultBaseStyle in BASE_STYLES) return p.defaultBaseStyle
    }
  } catch { /* ignore */ }
  return 'osm'
}

// ── Geo-Hilfe: Haversine-Distanz in Metern ───────────────────────────────
function haversineM(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const R = 6_371_000
  const dLat = (lat2 - lat1) * (Math.PI / 180)
  const dLng = (lng2 - lng1) * (Math.PI / 180)
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * (Math.PI / 180)) * Math.cos(lat2 * (Math.PI / 180)) * Math.sin(dLng / 2) ** 2
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

function findNearbyLocationTag(lat: number, lng: number, tags: Tag[], thresholdM: number): Tag | null {
  let nearest: Tag | null = null
  let nearestDist = Infinity
  for (const tag of tags) {
    if (tag.lat == null || tag.lng == null) continue
    const d = haversineM(lat, lng, tag.lat, tag.lng)
    if (d <= thresholdM && d < nearestDist) { nearest = tag; nearestDist = d }
  }
  return nearest
}

function buildTileUrl(
  source: 'heatmap_lines' | 'heatmap_points',
  filters: MapFilterState,
  teleportThresholdM: number, // 0 = kein Filter
  samplingIntervalM?: number, // undefined = kein Filter (Standardwert aus DB)
  userId?: string, // privacy: filter by current user
): string {
  // MapLibre v5 lädt Tiles in einem Web Worker. Worker können relative URLs
  // (/tiles/...) nicht auflösen → immer absolute URL erzwingen.
  // VITE_MARTIN_URL muss daher eine absolute URL sein (z.B. https://skadia.example.com/tiles).
  const martinBase = import.meta.env.VITE_MARTIN_URL ?? `${window.location.origin}/tiles`
  const base = `${martinBase}/${source}/{z}/{x}/{y}`
  const params = new URLSearchParams()

  if (userId) params.set('user_id', userId)

  const allSelected = filters.sportTypes.size === ALL_SPORT_KEYS.length
  if (!allSelected && filters.sportTypes.size > 0) {
    // Split composite keys ("sport:subType") back into sport_type and sub_type sets.
    // A key without a colon (e.g. "cycling" for road cycling) has no sub_type restriction —
    // if such a key is selected alongside sub-typed keys we must NOT send a sub_type
    // filter for that sport, or the road cycling activities would be excluded.
    const sportTypes = new Set<string>()
    // Track which sports appear with a bare (no-subType) key — those must not be
    // constrained by sub_type in the API query.
    const sportsWithBareKey = new Set<string>()
    const subTypes = new Set<string>()

    for (const key of filters.sportTypes) {
      const colonIdx = key.indexOf(':')
      if (colonIdx === -1) {
        // Bare key — no sub_type restriction for this sport
        sportTypes.add(key)
        sportsWithBareKey.add(key)
      } else {
        const sport = key.slice(0, colonIdx)
        const sub = key.slice(colonIdx + 1)
        sportTypes.add(sport)
        subTypes.add(sub)
      }
    }

    params.set('sport_type', Array.from(sportTypes).join(','))

    // Only send sub_type when NO selected sport has a bare key (i.e. every selected
    // cycling variant has an explicit sub_type). This prevents the filter from
    // accidentally excluding road cycling when MTB/Gravel are also selected.
    const subTypedSports = Array.from(subTypes).length > 0
      ? Array.from(sportTypes).filter(s => !sportsWithBareKey.has(s))
      : []
    if (subTypedSports.length > 0 && sportsWithBareKey.size === 0) {
      // All cycling selections carry a sub_type — safe to filter by sub_type
      params.set('sub_type', Array.from(subTypes).join(','))
    }
  }
  if (filters.dateFrom) params.set('date_from', filters.dateFrom)
  if (filters.dateTo) params.set('date_to', filters.dateTo)
  if (teleportThresholdM > 0) {
    params.set('teleport_threshold_m', String(teleportThresholdM))
  }
  if (samplingIntervalM != null && samplingIntervalM > 0) {
    params.set('sample_interval_m', String(samplingIntervalM))
  }
  const includeTags = (filters.tagFilters ?? []).filter(f => f.state === 'include').map(f => f.id)
  const excludeTags = (filters.tagFilters ?? []).filter(f => f.state === 'exclude').map(f => f.id)
  const includeGear = (filters.gearFilters ?? []).filter(f => f.state === 'include').map(f => f.id)
  const excludeGear = (filters.gearFilters ?? []).filter(f => f.state === 'exclude').map(f => f.id)
  if (includeTags.length) params.set('include_tags', includeTags.join(','))
  if (excludeTags.length) params.set('exclude_tags', excludeTags.join(','))
  if (includeGear.length) params.set('include_gear', includeGear.join(','))
  if (excludeGear.length) params.set('exclude_gear', excludeGear.join(','))

  const qs = params.toString()
  return qs ? `${base}?${qs}` : base
}

interface NominatimResult {
  place_id: number
  display_name: string
  lat: string
  lon: string
  type: string
  addresstype: string
}

function GeoSearch({ mapRef }: { mapRef: React.RefObject<maplibregl.Map | null> }) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<NominatimResult[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const searchNominatim = useCallback(async (q: string) => {
    if (q.length < 3) {
      setResults([])
      setIsOpen(false)
      return
    }
    setIsLoading(true)
    try {
      const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(q)}&limit=5&addressdetails=1`
      const resp = await fetch(url, {
        headers: { 'User-Agent': 'Skadia/1.0' },
      })
      if (!resp.ok) throw new Error(`Nominatim: ${resp.status}`)
      const data: NominatimResult[] = await resp.json()
      setResults(data)
      setIsOpen(data.length > 0)
      setActiveIndex(-1)
    } catch (err) {
      console.error('Geocoding error:', err)
      setResults([])
      setIsOpen(false)
    } finally {
      setIsLoading(false)
    }
  }, [])

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (value.length < 3) {
      setResults([])
      setIsOpen(false)
      return
    }
    debounceRef.current = setTimeout(() => searchNominatim(value), 500)
  }

  const handleSelect = (result: NominatimResult) => {
    const map = mapRef.current
    if (!map) return
    const lat = parseFloat(result.lat)
    const lon = parseFloat(result.lon)
    map.flyTo({ center: [lon, lat], zoom: 14, duration: 1500 })
    setQuery(result.display_name)
    setIsOpen(false)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      setIsOpen(false)
      return
    }
    if (!isOpen || results.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((prev) => (prev < results.length - 1 ? prev + 1 : 0))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((prev) => (prev > 0 ? prev - 1 : results.length - 1))
    } else if (e.key === 'Enter' && activeIndex >= 0) {
      e.preventDefault()
      handleSelect(results[activeIndex])
    }
  }

  const handleClear = () => {
    setQuery('')
    setResults([])
    setIsOpen(false)
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }

  return (
    <div ref={containerRef} className="absolute top-3 left-1/2 -translate-x-1/2 z-10 w-80">
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onFocus={() => { if (results.length > 0) setIsOpen(true) }}
          placeholder="Ort suchen..."
          className="w-full px-3 py-2 pr-8 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 dark:placeholder-neutral-500 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        {isLoading && (
          <div className="absolute right-8 top-1/2 -translate-y-1/2">
            <div className="w-4 h-4 border-2 border-neutral-300 border-t-orange-500 rounded-full animate-spin" />
          </div>
        )}
        {query && (
          <button
            onClick={handleClear}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
            aria-label="Suche leeren"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        )}
      </div>
      {isOpen && results.length > 0 && (
        <ul className="mt-1 max-h-60 overflow-y-auto rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/95 dark:bg-neutral-800/95 backdrop-blur-sm shadow-lg">
          {results.map((r, i) => (
            <li key={r.place_id}>
              <button
                onClick={() => handleSelect(r)}
                className={`w-full text-left px-3 py-2 text-sm cursor-pointer transition-colors ${
                  i === activeIndex
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700'
                }`}
              >
                <span className="line-clamp-2">{r.display_name}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export default function MapPage() {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const navigate = useNavigate()
  const location = useLocation()
  const { theme: uiTheme, setTheme: setUiTheme } = useTheme()
  const themeBeforeDarkMap = useRef<'light' | 'dark'>('light')
  const [savedViewport] = useState<StoredViewport | null>(loadViewport)
  const [baseStyle, setBaseStyle] = useState<BaseStyle>(
    savedViewport?.baseStyle ?? loadDefaultBaseStyle()
  )
  // Ref zum Zugriff auf aktuellen baseStyle in Unmount-Cleanup (Closure wäre stale)
  const baseStyleRef = useRef<BaseStyle>(savedViewport?.baseStyle ?? loadDefaultBaseStyle())
  const [filters, setFilters] = useState<MapFilterState>({
    ...defaultFilters,
    colorConfig: loadColorConfig(),
    heatmapColor: loadHeatmapColor(),
  })
  const [viewMode, setViewMode] = useState<ViewMode>(
    VALID_VIEW_MODES.includes(savedViewport?.viewMode as ViewMode)
      ? (savedViewport!.viewMode as ViewMode)
      : 'tracks'
  )
  // Dark-Karte → UI-Dark-Mode: beim Wechsel auf 'dark' Karte schaltet die gesamte UI
  // in den Dark Mode; beim Wegwechseln wird der vorherige Zustand wiederhergestellt.
  useEffect(() => {
    baseStyleRef.current = baseStyle
    if (baseStyle === 'dark') {
      themeBeforeDarkMap.current = uiTheme
      setUiTheme('dark')
    } else {
      setUiTheme(themeBeforeDarkMap.current)
    }
    // uiTheme absichtlich nicht als Dependency — nur baseStyle-Wechsel soll triggern
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseStyle])

  // Beim Verlassen der Seite: Dark-Theme zurücksetzen falls Dark-Karte aktiv war.
  // Der normale else-Branch im Effect oben läuft beim Unmount nicht — daher eigener Cleanup.
  useEffect(() => {
    return () => {
      if (baseStyleRef.current === 'dark') {
        setUiTheme(themeBeforeDarkMap.current)
      }
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Track welche user_id zuletzt für den jeweiligen Source verwendet wurde,
  // damit bei Änderung (z.B. nach Laden der /me-Antwort) neu geladen wird.
  const lastPeaksUserId = useRef<string>('')
  const lastForestsUserId = useRef<string>('')
  const lastAmenitiesUserId = useRef<string>('')

  // Custom Maps
  const [showCustomMaps, setShowCustomMaps] = useState(false)
  const [addMarkerModeMapID, setAddMarkerModeMapID] = useState<string | null>(null)

  // Photos
  const [showPhotos, setShowPhotos] = useState(false)
  const [selectedPhoto, setSelectedPhoto] = useState<MapPhotoEntry | null>(null)
  const photoMarkersRef = useRef<maplibregl.Marker[]>([])

  // Segments
  const [showSegments, setShowSegments] = useState(false)
  const [segmentBbox, setSegmentBbox] = useState<string>('')
  const segmentPopupRef = useRef<maplibregl.Popup | null>(null)
  const [segmentColors, setSegmentColors] = useState<Partial<SegmentColorConfig>>(() => {
    try {
      const saved = localStorage.getItem('skadia-segment-colors')
      return saved ? JSON.parse(saved) : {}
    } catch { return {} }
  })
  const [segmentFilters, setSegmentFilters] = useState<SegmentsOverlayFilters>(DEFAULT_SEGMENT_FILTERS)

  // Terra Incognita
  const terraIncognitaRef = useRef<TerraIncognitaCoverage | null>(null)
  const { data: terraIncognitaData, isFetching: terraIncognitaFetching } = useQuery({
    queryKey: ['terra-incognita'],
    queryFn: () => fetchTerraIncognita(),
    staleTime: 5 * 60 * 1000, // 5 min cache
  })

  const [isMapTilesLoading, setIsMapTilesLoading] = useState(false)

  // Location Tags
  const locationTagMarkersRef = useRef<maplibregl.Marker[]>([])
  const snapMarkerRef = useRef<{ marker: maplibregl.Marker; el: HTMLElement } | null>(null)
  const [selectedLocationTagId, setSelectedLocationTagId] = useState<string | null>(null)
  const [tagActivitiesLoading, setTagActivitiesLoading] = useState(false)
  const [tagActivities, setTagActivities] = useState<TagActivity[]>([])
  const [newLocationPick, setNewLocationPick] = useState<{
    name: string
    color: string
    activityIDs: string[]
    dropLngLat?: { lng: number; lat: number }
  } | null>(null)
  const [showIndoorAssign, setShowIndoorAssign] = useState(false)
  const locationTagsRef = useRef<Tag[]>([])
  const locationSyncTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  // Drag-to-merge: dialog state
  const [tagMergeDialog, setTagMergeDialog] = useState<{
    draggedTag: Tag
    nearbyTag: Tag
    marker: maplibregl.Marker
    originalLngLat: { lng: number; lat: number }
  } | null>(null)

  // Style version counter for re-adding layers after style changes
  const [styleVersion, setStyleVersion] = useState(0)

  const queryClient = useQueryClient()

  // Cleanup locationSyncTimers on unmount
  useEffect(() => () => {
    locationSyncTimersRef.current.forEach(t => clearTimeout(t))
  }, [])

  // Heatmap-Settings laden (Teleportfilter)
  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
  })

  // Custom Maps Daten laden
  const { data: customMaps = [] } = useQuery({
    queryKey: ['custom-maps'],
    queryFn: listCustomMaps,
  })

  // Alle Marker fuer sichtbare Karten laden
  const { data: allMarkers = [] } = useQuery({
    queryKey: ['all-custom-map-markers', customMaps.filter(m => m.is_visible).map(m => m.id).join(',')],
    queryFn: async () => {
      const results = await Promise.all(
        customMaps.filter(m => m.is_visible).map(m =>
          listCustomMapMarkers(m.id).then(markers => markers.map(mk => ({
            ...mk,
            mapColor: m.color,
          })))
        )
      )
      return results.flat()
    },
    enabled: customMaps.length > 0,
  })

  // Map Photos
  const { data: mapPhotos = [] } = useQuery({
    queryKey: ['map-photos'],
    queryFn: listMapPhotos,
    enabled: showPhotos,
  })

  // Segments
  const { data: segmentRaw } = useQuery({
    queryKey: ['map-segments', segmentBbox],
    queryFn: () => fetchSegments({ ...(segmentBbox ? { bbox: segmentBbox } : {}), include_geojson: true, limit: -1 }),
    enabled: showSegments,
  })
  const segmentData = segmentRaw?.segments ?? []

  const [overlayState, setOverlayState] = useState<MapOverlayState>(() => {
    try {
      const saved = localStorage.getItem('skadia-map-overlays')
      return saved ? { ...defaultOverlayState, ...JSON.parse(saved) } : defaultOverlayState
    } catch {
      return defaultOverlayState
    }
  })

  const updateOverlay = (key: keyof MapOverlayState, value: boolean) => {
    setOverlayState(prev => {
      const next = { ...prev, [key]: value }
      localStorage.setItem('skadia-map-overlays', JSON.stringify(next))
      return next
    })
  }

  // location.state: activateSegmentsOverlay — z.B. von SegmentsPage aus
  useEffect(() => {
    if (location.state?.activateSegmentsOverlay) {
      updateOverlay('showSegmentsOverlay', true)
      // State clearen damit beim Zurück-Navigieren kein erneutes Aktivieren passiert
      navigate(location.pathname, { replace: true, state: null })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const { data: meData } = useQuery({
    queryKey: ['me'],
    queryFn: () => fetch('/api/v1/me', { credentials: 'include' }).then(r => r.json()) as Promise<{ user_id: string }>,
    staleTime: Infinity,
  })
  const currentUserId = meData?.user_id ?? ''

  const { data: visitedPoisGeoJSON } = useQuery({
    queryKey: ['visited-pois'],
    queryFn: () => fetch('/api/v1/geo/visited-pois', { credentials: 'include' }).then(r => r.json()) as Promise<GeoJSON.FeatureCollection>,
    enabled: overlayState.showVisitedPois,
  })

  // Create marker mutation
  const createMarkerMutation = useMutation({
    mutationFn: ({ mapID, lat, lng, name }: { mapID: string; lat: number; lng: number; name: string }) =>
      createCustomMapMarker(mapID, { name, lat, lng }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
      queryClient.invalidateQueries({ queryKey: ['custom-maps'] })
      queryClient.invalidateQueries({ queryKey: ['custom-map-markers'] })
    },
  })

  // Location Tags
  const { data: allTags = [] } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    enabled: filters.showLocationTags || showIndoorAssign || !!selectedLocationTagId,
  })
  const locationTags = useMemo(
    () => allTags.filter(t => t.lat != null && t.lng != null),
    [allTags]
  )
  locationTagsRef.current = locationTags

  // Derive selected tag from fresh query data to avoid stale state
  const selectedLocationTag = selectedLocationTagId
    ? (locationTags.find(t => t.id === selectedLocationTagId) ?? null)
    : null
  const coverImageBlobUrl = useAuthImage(selectedLocationTag?.cover_image_url)

  const updateTagLocationMutation = useMutation({
    mutationFn: ({ id, lat, lng }: { id: string; lat: number; lng: number }) =>
      updateTag(id, { lat, lng }),
    onSuccess: (updatedTag) => {
      // Optimistic update already done on dragend; sync with server response
      queryClient.setQueryData<Tag[]>(['tags'], (prev) =>
        prev ? prev.map(t => t.id === updatedTag.id ? updatedTag : t) : prev
      )
    },
  })

  const mergeTagMutation = useMutation({
    mutationFn: ({ targetId, sourceId }: { targetId: string; sourceId: string }) =>
      mergeTags(targetId, sourceId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      setTagMergeDialog(null)
    },
  })

  const teleportThresholdM = heatmapSettings?.teleport_filter_map
    ? (heatmapSettings.teleport_threshold_m ?? 2000)
    : 0

  const samplingIntervalM = heatmapSettings?.sampling_interval_m ?? 20

  // Refs damit der style.load-Handler immer die aktuellen Werte sieht,
  // ohne jedes Mal neu registriert werden zu müssen.
  const filtersRef = useRef(filters)
  const viewModeRef = useRef(viewMode)
  const teleportRef = useRef(teleportThresholdM)
  const samplingRef = useRef(samplingIntervalM)
  // Kamera-Zustand vor setStyle() sichern → persistenter style.load-Handler stellt ihn wieder her
  const pendingCameraRestoreRef = useRef<{ center: maplibregl.LngLat; zoom: number } | null>(null)
  useEffect(() => { filtersRef.current = filters }, [filters])
  useEffect(() => {
    viewModeRef.current = viewMode
    const stored = loadViewport()
    if (stored) {
      saveViewport({ ...stored, viewMode })
    }
  }, [viewMode])
  const currentUserIdRef = useRef(currentUserId)
  useEffect(() => { currentUserIdRef.current = currentUserId }, [currentUserId])
  useEffect(() => { teleportRef.current = teleportThresholdM }, [teleportThresholdM])
  useEffect(() => { samplingRef.current = samplingIntervalM }, [samplingIntervalM])

  // ── Einmalige Karteninitialisierung ─────────────────────────────────────
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: buildRasterStyle(savedViewport?.baseStyle ?? loadDefaultBaseStyle()),
      center: savedViewport ? savedViewport.center : [10.45, 51.16],
      zoom: savedViewport ? savedViewport.zoom : 6,
      attributionControl: { compact: false },
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    // Persistenter Handler: feuert bei JEDEM setStyle(), also initial und bei
    // Kartenstil-Wechsel. removeOverlayLayers() wird zuerst aufgerufen, damit
    // kein "Source already exists"-Fehler entsteht falls der Handler mehrfach
    // feuert (z. B. React StrictMode double-invoke).
    map.on('style.load', () => {
      // Kameraposition nach Stilwechsel atomar wiederherstellen (verhindert Race Condition
      // mit map.once('style.load') das bei synchronem setStyle() zu spät registriert wird)
      if (pendingCameraRestoreRef.current) {
        map.jumpTo(pendingCameraRestoreRef.current)
        pendingCameraRestoreRef.current = null
      }
      map.resize()
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
      if (viewModeRef.current === 'terra_incognita') {
        if (terraIncognitaRef.current?.terra_incognita_area) {
          addTerraIncognitaLayers(map, terraIncognitaRef.current.terra_incognita_area)
        }
      } else {
        addOverlayLayers(map, filtersRef.current, viewModeRef.current, teleportRef.current, samplingRef.current, currentUserIdRef.current)
      }
      setStyleVersion(v => v + 1)
    })

    // Save viewport position on move/zoom (debounced)
    let viewportSaveTimer: ReturnType<typeof setTimeout> | null = null
    function scheduleViewportSave() {
      if (viewportSaveTimer) clearTimeout(viewportSaveTimer)
      viewportSaveTimer = setTimeout(() => {
        const c = map.getCenter()
        saveViewport({
          center: [c.lng, c.lat],
          zoom: map.getZoom(),
          viewMode: viewModeRef.current,
          baseStyle: prevBaseStyleRef.current,
        })
      }, 500)
    }
    map.on('moveend', scheduleViewportSave)
    map.on('zoomend', scheduleViewportSave)

    // Tile-Lade-Indikator: sourcedataloading nur für 'heatmap'-Source (Tracks+Heatmap+TI-Overlay),
    // idle feuert wenn alle Tiles geladen sind.
    map.on('sourcedataloading', (e) => {
      if (e.sourceId === 'heatmap') setIsMapTilesLoading(true)
    })
    map.on('idle', () => setIsMapTilesLoading(false))

    const ro = new ResizeObserver(() => map.resize())
    ro.observe(mapContainerRef.current)

    mapRef.current = map

    return () => {
      ro.disconnect()
      map.remove()
      mapRef.current = null
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Kartenstil wechseln ──────────────────────────────────────────────────
  // prevBaseStyleRef verhindert den redundanten setStyle()-Aufruf beim ersten
  // Render (baseStyle='osm' == Initialstil der Map-Instanz). Ohne diesen Schutz
  // würde MapLibre "Style is not done loading.. Rebuilding from scratch" werfen.
  const prevBaseStyleRef = useRef<BaseStyle>(savedViewport?.baseStyle ?? loadDefaultBaseStyle())
  useEffect(() => {
    const map = mapRef.current
    if (!map || prevBaseStyleRef.current === baseStyle) return
    prevBaseStyleRef.current = baseStyle

    // Kamera vor setStyle() sichern — persistenter style.load-Handler stellt sie wieder her.
    // map.once() würde die Race Condition nicht lösen: bei inline StyleSpecification-Objekten
    // kann style.load synchron feuern, bevor map.once() registriert ist.
    pendingCameraRestoreRef.current = { center: map.getCenter(), zoom: map.getZoom() }

    map.setStyle(buildRasterStyle(baseStyle))

    // Persist baseStyle change to viewport
    const stored = loadViewport()
    if (stored) {
      saveViewport({ ...stored, baseStyle })
    }
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Overlay bei Filter-/Modus-/Settings-Änderung aktualisieren ──────────
  // showLocationTags absichtlich NICHT in deps — das Flag beeinflusst nur
  // MapLibre-Marker, nicht die Tile-Layer → kein unnötiger Layer-Rebuild.
  // styleVersion muss in deps: nach setStyle() werden alle Custom-Sources/-Layer
  // gelöscht; style.load-Handler fügt sie neu hinzu und inkrementiert styleVersion,
  // damit dieser Effect erneut feuert und den Layer-Zustand sauber herstellt.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    if (viewMode === 'terra_incognita') {
      // Beim Wechsel zu Terra Incognita: Overlay-Source komplett entfernen (inkl. Tile-Cache),
      // da die Source für Terra Incognita nicht mehr benötigt wird.
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
      // Sofortiger Nebel (bei fehlendem terraIncognitaData: volle Welt-Abdeckung als Fallback).
      const geom = terraIncognitaRef.current?.terra_incognita_area
        ?? ({ type: 'Polygon', coordinates: [] } as GeoJSON.Geometry)
      addTerraIncognitaLayers(map, geom)
    } else {
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
      if (filters.sportTypes.size > 0) {
        addOverlayLayers(map, filters, viewMode, teleportThresholdM, samplingIntervalM, currentUserId)
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.sportTypes, filters.dateFrom, filters.dateTo, filters.opacity, filters.colorConfig, filters.heatmapColor, filters.tagFilters, filters.gearFilters, viewMode, heatmapSettings, teleportThresholdM, samplingIntervalM, styleVersion])

  // ── Terra Incognita Overlay Layer ────────────────────────────────────────
  // styleVersion in deps: nach Stilwechsel muss der Overlay neu aufgebaut werden,
  // weil setStyle() alle Custom-Layers löscht. Dieser Effect aktualisiert die Ref
  // und baut den Layer neu auf. Hinweis: der style.load-Handler liest terraIncognitaRef
  // synchron, kann dabei aber eine veraltete (noch nicht durch diesen Effect befüllte)
  // Ref sehen. styleVersion stellt sicher, dass der Layer spätestens beim nächsten
  // styleVersion-Trigger korrekt vorhanden ist.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    terraIncognitaRef.current = terraIncognitaData ?? null
    removeTerraIncognitaLayers(map)
    if (viewMode === 'terra_incognita' && terraIncognitaData?.terra_incognita_area) {
      addTerraIncognitaLayers(map, terraIncognitaData.terra_incognita_area)
    }
  }, [viewMode, terraIncognitaData, styleVersion])

  // ── Custom Map Marker-Layer ──────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    // Entferne alte Marker-Layer
    if (map.getLayer('custom-markers-labels')) map.removeLayer('custom-markers-labels')
    if (map.getLayer('custom-markers-circles')) map.removeLayer('custom-markers-circles')
    if (map.getSource('custom-markers')) map.removeSource('custom-markers')

    if (!showCustomMaps || allMarkers.length === 0) return

    const geojson: GeoJSON.FeatureCollection = {
      type: 'FeatureCollection',
      features: allMarkers.map(mk => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [mk.lng, mk.lat] },
        properties: {
          id: mk.id,
          name: mk.name,
          color: mk.color ?? mk.mapColor ?? '#F97316',
          map_id: mk.map_id,
        },
      })),
    }

    map.addSource('custom-markers', { type: 'geojson', data: geojson })

    map.addLayer({
      id: 'custom-markers-circles',
      type: 'circle',
      source: 'custom-markers',
      paint: {
        'circle-radius': 8,
        'circle-color': ['get', 'color'],
        'circle-stroke-width': 2,
        'circle-stroke-color': '#ffffff',
      },
    })

    map.addLayer({
      id: 'custom-markers-labels',
      type: 'symbol',
      source: 'custom-markers',
      layout: {
        'text-field': ['get', 'name'],
        'text-size': 11,
        'text-offset': [0, 1.5],
        'text-anchor': 'top',
      },
      paint: {
        'text-color': '#1a1a1a',
        'text-halo-color': '#ffffff',
        'text-halo-width': 2,
      },
    })
  }, [showCustomMaps, allMarkers, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Custom Map Marker Drag ────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !showCustomMaps) return

    let dragging: { id: string; mapId: string } | null = null

    function onMouseDown(e: maplibregl.MapLayerMouseEvent) {
      const f = e.features?.[0]
      if (!f) return
      e.preventDefault()
      dragging = { id: f.properties?.id as string, mapId: f.properties?.map_id as string }
      map!.getCanvas().style.cursor = 'grabbing'
      map!.dragPan.disable()
    }

    function onMouseMove(e: maplibregl.MapMouseEvent) {
      if (!dragging) return
      const src = map!.getSource('custom-markers') as maplibregl.GeoJSONSource | undefined
      if (!src) return
      const { lng, lat } = e.lngLat
      // Update GeoJSON feature position in real-time
      const geojson: GeoJSON.FeatureCollection = {
        type: 'FeatureCollection',
        features: allMarkers.map(mk => ({
          type: 'Feature' as const,
          geometry: {
            type: 'Point' as const,
            coordinates: mk.id === dragging!.id ? [lng, lat] : [mk.lng, mk.lat],
          },
          properties: {
            id: mk.id,
            name: mk.name,
            color: mk.color ?? mk.mapColor ?? '#F97316',
            map_id: mk.map_id,
          },
        })),
      }
      src.setData(geojson)
    }

    function onMouseUp(e: maplibregl.MapMouseEvent) {
      if (!dragging) return
      const { lng, lat } = e.lngLat
      const d = dragging
      dragging = null
      map!.getCanvas().style.cursor = ''
      map!.dragPan.enable()
      updateCustomMapMarker(d.mapId, d.id, { lat, lng })
        .then(() => {
          queryClient.invalidateQueries({ queryKey: ['all-custom-map-markers'] })
          queryClient.invalidateQueries({ queryKey: ['custom-map-markers', d.mapId] })
        })
        .catch(() => { /* silent */ })
    }

    map.on('mousedown', 'custom-markers-circles', onMouseDown)
    map.on('mousemove', onMouseMove)
    map.on('mouseup', onMouseUp)
    map.on('mouseenter', 'custom-markers-circles', () => {
      if (!dragging) map.getCanvas().style.cursor = 'grab'
    })
    map.on('mouseleave', 'custom-markers-circles', () => {
      if (!dragging) map.getCanvas().style.cursor = ''
    })

    return () => {
      map.off('mousedown', 'custom-markers-circles', onMouseDown)
      map.off('mousemove', onMouseMove)
      map.off('mouseup', onMouseUp)
    }
  }, [showCustomMaps, allMarkers, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Click-Handler fuer Marker-Hinzufuegen ──────────────────────────────
  const { openPromptModal } = useModal()

  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (!addMarkerModeMapID) {
      map.getCanvas().style.cursor = ''
      return
    }
    map.getCanvas().style.cursor = 'crosshair'

    function handleClick(e: maplibregl.MapMouseEvent) {
      if (!addMarkerModeMapID) return
      const { lng, lat } = e.lngLat

      openPromptModal({
        title: 'Marker-Name',
        placeholder: 'z.B. "Lieblingskaffee"',
        onConfirm: async (name) => {
          if (name.trim()) {
            createMarkerMutation.mutate({ mapID: addMarkerModeMapID, lat, lng, name })
          }
          setAddMarkerModeMapID(null)
          map!.getCanvas().style.cursor = ''
        },
        onCancel: () => {
          setAddMarkerModeMapID(null)
          map!.getCanvas().style.cursor = ''
        },
      })
    }

    map.on('click', handleClick)
    return () => {
      map.off('click', handleClick)
      map.getCanvas().style.cursor = ''
    }
  }, [addMarkerModeMapID, openPromptModal]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Photo Markers ──────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    // Alte Foto-Marker entfernen
    photoMarkersRef.current.forEach(m => m.remove())
    photoMarkersRef.current = []

    if (!showPhotos || mapPhotos.length === 0) return

    mapPhotos.forEach(photo => {
      const el = document.createElement('div')
      el.style.cssText = `
        width: 48px; height: 48px;
        border-radius: 4px;
        border: 2px solid white;
        box-shadow: 0 2px 4px rgba(0,0,0,0.3);
        overflow: hidden;
        cursor: pointer;
        background: #333;
      `
      const img = document.createElement('img')
      img.style.cssText = 'width: 100%; height: 100%; object-fit: cover;'
      apiDownload(photo.thumb_url).then(({ blob }) => {
        const blobUrl = URL.createObjectURL(blob)
        img.src = blobUrl
        img.addEventListener('load', () => URL.revokeObjectURL(blobUrl), { once: true })
      }).catch(() => { /* thumbnail load failed — marker shows background color */ })
      el.appendChild(img)

      el.addEventListener('click', (e) => {
        e.stopPropagation()
        setSelectedPhoto(photo)
      })

      const marker = new maplibregl.Marker({ element: el })
        .setLngLat([photo.lng, photo.lat])
        .addTo(map)

      photoMarkersRef.current.push(marker)
    })

    return () => {
      photoMarkersRef.current.forEach(m => m.remove())
      photoMarkersRef.current = []
    }
  }, [showPhotos, mapPhotos, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Location Tag Markers ────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    // Remove old location markers
    locationTagMarkersRef.current.forEach(m => m.remove())
    locationTagMarkersRef.current = []

    if (!filters.showLocationTags || locationTags.length === 0) return

    locationTags.forEach(tag => {
      if (tag.lat == null || tag.lng == null) return

      // el = MapLibre-Root (trägt nur den MapLibre-Transform, kein eigenes Scale)
      const el = document.createElement('div')
      el.style.cssText = `width: 36px; cursor: pointer;`
      el.dataset.tagId = tag.id

      // inner = skalierbares Kind-Element (kein MapLibre-Transform hier)
      const inner = document.createElement('div')
      inner.style.cssText = `
        display: inline-flex;
        flex-direction: column;
        align-items: center;
        width: 36px;
        transform-origin: bottom center;
      `

      // Pin circle
      const circle = document.createElement('div')
      const hasZone = tag.zone != null
      circle.style.cssText = `
        width: 32px;
        height: 32px;
        border-radius: 50%;
        background: ${tag.color || '#6366F1'};
        border: ${hasZone ? '3px solid rgba(255,255,255,0.95)' : '2px solid rgba(255,255,255,0.9)'};
        box-shadow: ${hasZone ? `0 0 0 2px ${tag.color || '#6366F1'}, 0 2px 8px rgba(0,0,0,0.4)` : '0 2px 6px rgba(0,0,0,0.4)'};
        display: flex;
        align-items: center;
        justify-content: center;
        color: ${tag.text_color || '#ffffff'};
        font-size: 11px;
        font-weight: 700;
        flex-shrink: 0;
      `
      circle.textContent = tag.activity_count > 0 ? String(tag.activity_count) : tag.name.charAt(0).toUpperCase()

      // Pin tail
      const tail = document.createElement('div')
      tail.style.cssText = `
        width: 0;
        height: 0;
        border-left: 6px solid transparent;
        border-right: 6px solid transparent;
        border-top: 8px solid ${tag.color || '#6366F1'};
        flex-shrink: 0;
      `

      inner.appendChild(circle)
      inner.appendChild(tail)
      el.appendChild(inner)

      el.addEventListener('click', (e) => {
        e.stopPropagation()
        setSelectedLocationTagId(tag.id)
      })

      const marker = new maplibregl.Marker({ element: el, draggable: true, anchor: 'bottom' })
        .setLngLat([tag.lng, tag.lat])
        .addTo(map)

      // Remember the position at drag start so we can roll back on merge-cancel
      let originalLngLat = { lng: tag.lng, lat: tag.lat }
      marker.on('dragstart', () => {
        const pos = marker.getLngLat()
        originalLngLat = { lng: pos.lng, lat: pos.lat }
      })

      marker.on('dragend', () => {
        const { lat, lng } = marker.getLngLat()

        // Check for a nearby location tag within 60 m (excluding self)
        const others = locationTagsRef.current.filter(t => t.id !== tag.id)
        const nearby = findNearbyLocationTag(lat, lng, others, 60)

        if (nearby) {
          // Show merge-confirmation dialog; do NOT save new position yet
          setTagMergeDialog({ draggedTag: tag, nearbyTag: nearby, marker, originalLngLat })
        } else {
          // Normal position update
          queryClient.setQueryData<Tag[]>(['tags'], (prev) =>
            prev ? prev.map(t => t.id === tag.id ? { ...t, lat, lng } : t) : prev
          )
          updateTagLocationMutation.mutate({ id: tag.id, lat, lng })
          // Debounced sync (3s): location_lat/location_lng auf verknüpfte Aktivitäten übertragen
          const existing = locationSyncTimersRef.current.get(tag.id)
          if (existing) clearTimeout(existing)
          const timer = setTimeout(() => {
            locationSyncTimersRef.current.delete(tag.id)
            syncTagLocation(tag.id).catch(() => {})
          }, 3000)
          locationSyncTimersRef.current.set(tag.id, timer)
        }
      })

      locationTagMarkersRef.current.push(marker)
    })

    return () => {
      locationTagMarkersRef.current.forEach(m => m.remove())
      locationTagMarkersRef.current = []
    }
  }, [filters.showLocationTags, locationTags, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Drop-Location: Name-Dialog + direktes Erstellen ──────────────────────
  const [dropNameInput, setDropNameInput] = useState('')
  const [dropColorInput, setDropColorInput] = useState('#f97316')
  const isDropMode = !!(newLocationPick?.dropLngLat)

  async function handleDropConfirm() {
    if (!newLocationPick?.dropLngLat || !dropNameInput.trim()) return
    const { lng, lat } = newLocationPick.dropLngLat
    const { createTag } = await import('@/api/tags')
    try {
      const newTag = await createTag({
        name: dropNameInput.trim(),
        color: dropColorInput,
        lat,
        lng,
      })
      if (newLocationPick.activityIDs.length > 0) {
        await Promise.all(
          newLocationPick.activityIDs.map(async (actId) => {
            try {
              const act = await fetchActivity(actId)
              const currentTags = act.tags ?? []
              if (!currentTags.includes(newTag.name)) {
                await updateActivity(actId, { tags: [...currentTags, newTag.name] })
              }
            } catch { /* ignore per-activity errors */ }
          })
        )
      }
      queryClient.invalidateQueries({ queryKey: ['tags'] })
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['indoor-activities'] })
      syncTagLocation(newTag.id).catch(() => {})
      setSelectedLocationTagId(newTag.id)  // auto-open popup
    } catch (err) {
      console.error('Failed to create location tag from drop:', err)
    }
    setNewLocationPick(null)
    setDropNameInput('')
    setDropColorInput('#f97316')
  }

  // ── Crosshair-Modus für neues Standort-Tag ──────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    // Skip crosshair mode when in drop mode (coordinates already known)
    if (!newLocationPick || newLocationPick.dropLngLat) {
      map.getCanvas().style.cursor = ''
      return
    }
    map.getCanvas().style.cursor = 'crosshair'

    async function handleLocationClick(e: maplibregl.MapMouseEvent) {
      if (!newLocationPick) return
      const { lat, lng } = e.lngLat
      const { createTag } = await import('@/api/tags')

      try {
        // Create the new location tag with coordinates
        const newTag = await createTag({
          name: newLocationPick.name,
          color: newLocationPick.color,
          lat,
          lng,
        })

        // Tag all selected activities
        if (newLocationPick.activityIDs.length > 0) {
          await Promise.all(
            newLocationPick.activityIDs.map(async (actId) => {
              try {
                const act = await fetchActivity(actId)
                const currentTags = act.tags ?? []
                if (!currentTags.includes(newTag.name)) {
                  await updateActivity(actId, { tags: [...currentTags, newTag.name] })
                }
              } catch { /* ignore per-activity errors */ }
            })
          )
        }

        queryClient.invalidateQueries({ queryKey: ['tags'] })
        queryClient.invalidateQueries({ queryKey: ['activities'] })
        syncTagLocation(newTag.id).catch(() => {})
      } catch (err) {
        console.error('Failed to create location tag:', err)
      }

      setNewLocationPick(null)
      map!.getCanvas().style.cursor = ''
    }

    map.on('click', handleLocationClick)
    return () => {
      map.off('click', handleLocationClick)
      map.getCanvas().style.cursor = ''
    }
  }, [newLocationPick]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Canvas-Drag-Listener: Aktivitäten auf Karte ziehen (Indoor-Tagging) ──
  // Direktes Binden an den MapLibre-Canvas, weil React-onDrop am Container-div
  // durch den Canvas geblockt wird (MapLibre fängt Pointer-Events ab).
  // locationTagsRef hält immer den aktuellen Stand (kein Stale-Closure-Problem).
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const canvas = map.getCanvas()

    function onCanvasDragOver(e: DragEvent) {
      if (!e.dataTransfer?.types.includes('application/skadia-activities')) return
      e.preventDefault()
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'

      // Magnetic snap visual feedback
      const rect = canvas.getBoundingClientRect()
      const point: [number, number] = [e.clientX - rect.left, e.clientY - rect.top]
      const lngLat = mapRef.current?.unproject(point)
      if (!lngLat) return

      const nearby = findNearbyLocationTag(lngLat.lat, lngLat.lng, locationTagsRef.current, 100)

      const markerEntry = nearby
        ? locationTagMarkersRef.current.find(m => m.getElement().dataset.tagId === nearby.id) ?? null
        : null
      const markerEl = markerEntry ? markerEntry.getElement() : null

      // Clear previous snap
      if (snapMarkerRef.current && snapMarkerRef.current.el !== markerEl) {
        const prevInner = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
        if (prevInner) { prevInner.style.scale = ''; prevInner.style.transition = '' }
        snapMarkerRef.current = null
      }

      // Apply snap to new target (scale inner child, not el — preserves MapLibre translate)
      if (markerEl && !snapMarkerRef.current) {
        const innerEl = markerEl.firstElementChild as HTMLElement | null
        if (innerEl) {
          innerEl.style.transition = 'scale 0.15s ease'
          innerEl.style.scale = '1.35'
        }
        snapMarkerRef.current = { marker: markerEntry!, el: markerEl }
      }
    }

    function onCanvasDragLeave() {
      if (snapMarkerRef.current) {
        const innerEl = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
        if (innerEl) { innerEl.style.scale = ''; innerEl.style.transition = '' }
        snapMarkerRef.current = null
      }
    }

    async function onCanvasDrop(e: DragEvent) {
      e.preventDefault()
      // Clear magnetic snap visual
      if (snapMarkerRef.current) {
        const prevInner = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
        if (prevInner) { prevInner.style.scale = ''; prevInner.style.transition = '' }
        snapMarkerRef.current = null
      }
      const raw = e.dataTransfer?.getData('application/skadia-activities')
      if (!raw) return
      let activityIDs: string[]
      try { activityIDs = JSON.parse(raw) } catch { return }
      const rect = canvas.getBoundingClientRect()
      const point: [number, number] = [e.clientX - rect.left, e.clientY - rect.top]
      const lngLat = mapRef.current?.unproject(point)
      if (!lngLat) return

      // Magnetische Pins: nächster Location-Tag innerhalb 100 m → direkt zuweisen
      const nearbyTag = findNearbyLocationTag(lngLat.lat, lngLat.lng, locationTagsRef.current, 100)
      if (nearbyTag) {
        try {
          await assignActivitiesToTag(nearbyTag.id, activityIDs)
        } catch { /* ignorieren */ }
        queryClient.invalidateQueries({ queryKey: ['activities'] })
        queryClient.invalidateQueries({ queryKey: ['indoor-activities'] })
        queryClient.invalidateQueries({ queryKey: ['tags'] })
        syncTagLocation(nearbyTag.id).catch(() => {})
        setShowIndoorAssign(false)
        // Pulse animation on successful drop
        const targetMarker = locationTagMarkersRef.current.find(m => m.getElement().dataset.tagId === nearbyTag.id)
        if (targetMarker) {
          const el = targetMarker.getElement()
          const innerEl = el.firstElementChild as HTMLElement | null
          if (innerEl) {
            innerEl.style.transition = 'scale 0.15s ease'
            innerEl.style.scale = '1.5'
            setTimeout(() => { innerEl.style.scale = '1.0' }, 150)
            setTimeout(() => { innerEl.style.transition = ''; innerEl.style.scale = '' }, 400)
          }
        }
      } else {
        // Kein naher Pin → neuen Standort anlegen
        setShowIndoorAssign(false)
        setNewLocationPick({ name: '', color: '#f97316', activityIDs, dropLngLat: { lng: lngLat.lng, lat: lngLat.lat } })
      }
    }

    canvas.addEventListener('dragover', onCanvasDragOver)
    canvas.addEventListener('dragleave', onCanvasDragLeave)
    canvas.addEventListener('drop', onCanvasDrop)
    return () => {
      canvas.removeEventListener('dragover', onCanvasDragOver)
      canvas.removeEventListener('dragleave', onCanvasDragLeave)
      canvas.removeEventListener('drop', onCanvasDrop)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Segment Layer ──────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    // Cleanup old segment layers
    if (map.getLayer('segment-direction-arrows')) map.removeLayer('segment-direction-arrows')
    if (map.getSource('segments-tracks')) map.removeSource('segments-tracks')
    if (map.getLayer('segments-lines')) map.removeLayer('segments-lines')
    if (map.getSource('segments-geojson')) map.removeSource('segments-geojson')
    if (segmentPopupRef.current) {
      segmentPopupRef.current.remove()
      segmentPopupRef.current = null
    }

    if (!showSegments || segmentData.length === 0) return

    // Build GeoJSON point features from segments that have coordinate data
    const features: GeoJSON.Feature[] = segmentData
      .filter((s: Segment) => {
        const meta = s.segment_type_meta as Record<string, unknown> | undefined
        return meta && typeof meta.start_lat === 'number' && typeof meta.start_lng === 'number'
      })
      .map((s: Segment) => {
        const meta = s.segment_type_meta as Record<string, unknown>
        return {
          type: 'Feature' as const,
          geometry: {
            type: 'Point' as const,
            coordinates: [meta.start_lng as number, meta.start_lat as number],
          },
          properties: {
            id: s.id,
            name: s.name,
            sport_group: s.sport_group,
            distance_m: s.distance_m ?? 0,
            effort_count: s.effort_count,
          },
        }
      })

    if (features.length === 0) return

    const SEGMENT_COLORS: Record<string, string> = {
      bike: '#F97316',
      foot: '#22C55E',
      swim: '#06B6D4',
      ski: '#3B82F6',
      paddle: '#3B82F6',
      generic: '#6B7280',
    }

    map.addSource('segments-geojson', {
      type: 'geojson',
      data: { type: 'FeatureCollection', features },
    })

    map.addLayer({
      id: 'segments-lines',
      type: 'circle',
      source: 'segments-geojson',
      paint: {
        'circle-radius': 7,
        'circle-color': [
          'match', ['get', 'sport_group'],
          'bike', SEGMENT_COLORS.bike,
          'foot', SEGMENT_COLORS.foot,
          'swim', SEGMENT_COLORS.swim,
          'ski', SEGMENT_COLORS.ski,
          'paddle', SEGMENT_COLORS.paddle,
          SEGMENT_COLORS.generic,
        ] as maplibregl.ExpressionSpecification,
        'circle-stroke-width': 2,
        'circle-stroke-color': '#ffffff',
        'circle-opacity': 0.9,
      },
    })

    // Build LineString features for direction arrows (segments with track_geojson)
    const trackFeatures: GeoJSON.Feature[] = segmentData
      .filter((s: Segment) => !!s.track_geojson)
      .reduce<GeoJSON.Feature[]>((acc, s: Segment) => {
        let geometry: GeoJSON.Geometry
        try {
          geometry = JSON.parse(s.track_geojson!) as GeoJSON.Geometry
        } catch {
          return acc
        }
        acc.push({
          type: 'Feature' as const,
          geometry,
          properties: {
            id: s.id,
            name: s.name,
            sport_group: s.sport_group,
          },
        })
        return acc
      }, [])

    if (trackFeatures.length > 0) {
      map.addSource('segments-tracks', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: trackFeatures },
      })

      // Pfeil-Icon registrieren und Symbol-Layer für Richtungspfeile hinzufügen
      const arrowSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><path d="M10 3 L16 17 L10 13 L4 17 Z" fill="white" opacity="0.9"/></svg>`
      const arrowImg = new Image(20, 20)
      arrowImg.src = 'data:image/svg+xml,' + encodeURIComponent(arrowSVG)
      arrowImg.onload = () => {
        if (!map.hasImage('segment-arrow')) {
          map.addImage('segment-arrow', arrowImg)
        }
        if (!map.getLayer('segment-direction-arrows')) {
          map.addLayer({
            id: 'segment-direction-arrows',
            type: 'symbol',
            source: 'segments-tracks',
            layout: {
              'symbol-placement': 'line',
              'symbol-spacing': 120,
              'icon-image': 'segment-arrow',
              'icon-size': 0.7,
              'icon-allow-overlap': true,
              'icon-rotation-alignment': 'map',
            },
            paint: {
              'icon-opacity': 0.75,
            },
          })
        }
      }
    }

    // Hover tooltip
    const popup = new maplibregl.Popup({
      closeButton: false,
      closeOnClick: false,
      offset: 12,
    })
    segmentPopupRef.current = popup

    map.on('mouseenter', 'segments-lines', (e) => {
      map.getCanvas().style.cursor = 'pointer'
      if (e.features?.[0]) {
        const coords = (e.features[0].geometry as GeoJSON.Point).coordinates.slice() as [number, number]
        const name = e.features[0].properties?.name ?? ''
        popup.setLngLat(coords).setHTML(`<span style="font-size:12px;font-weight:500">${name}</span>`).addTo(map)
      }
    })

    map.on('mouseleave', 'segments-lines', () => {
      map.getCanvas().style.cursor = ''
      popup.remove()
    })
  }, [showSegments, segmentData, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Update segment bbox on map move ────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !showSegments) return

    function updateBbox() {
      const bounds = map!.getBounds()
      const bbox = `${bounds.getWest().toFixed(6)},${bounds.getSouth().toFixed(6)},${bounds.getEast().toFixed(6)},${bounds.getNorth().toFixed(6)}`
      setSegmentBbox(bbox)
    }

    updateBbox()
    map.on('moveend', updateBbox)
    return () => {
      map.off('moveend', updateBbox)
    }
  }, [showSegments]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleFilterChange = useCallback((next: MapFilterState) => {
    setFilters(next)
    localStorage.setItem('skadia-sport-colors', JSON.stringify({ ...next.colorConfig, heatmapColor: next.heatmapColor }))
  }, [])

  // Load activities for the selected location tag popup
  useEffect(() => {
    if (!selectedLocationTagId) {
      setTagActivities([])
      setTagActivitiesLoading(false)
      return
    }
    let cancelled = false
    setTagActivitiesLoading(true)
    fetchTagActivities(selectedLocationTagId)
      .then(acts => { if (!cancelled) setTagActivities(acts) })
      .catch(() => { if (!cancelled) setTagActivities([]) })
      .finally(() => { if (!cancelled) setTagActivitiesLoading(false) })
    return () => { cancelled = true }
  }, [selectedLocationTagId])

  // Location tag popup: gallery state + photo loading
  const [tagPhotos, setTagPhotos] = useState<import('@/api/activityPhotos').ActivityPhoto[]>([])
  const [tagGalleryOpen, setTagGalleryOpen] = useState(false)
  const [tagGallerySelectMode, setTagGallerySelectMode] = useState(false)
  const [tagLightboxPhoto, setTagLightboxPhoto] = useState<string | null>(null)

  useEffect(() => {
    if (!selectedLocationTagId || tagActivities.length === 0) {
      setTagPhotos([])
      return
    }
    Promise.all(tagActivities.map(act =>
      import('@/api/activityPhotos').then(m => m.listActivityPhotos(act.id)).catch(() => [])
    )).then(results => setTagPhotos(results.flat()))
  }, [selectedLocationTagId, tagActivities])

  // ── Gipfel-Layer ─────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/tiles`
    const peakTileUrl = currentUserId
      ? `${martinBase}/peaks/{z}/{x}/{y}?user_id=${encodeURIComponent(currentUserId)}`
      : `${martinBase}/peaks/{z}/{x}/{y}`

    const addPeaksLayer = () => {
      // Source neu laden wenn sich user_id geändert hat (visited-Flag korrekt rendern)
      if (map.getSource('peaks-source') && lastPeaksUserId.current !== currentUserId) {
        if (map.getLayer('peaks-layer')) map.removeLayer('peaks-layer')
        map.removeSource('peaks-source')
      }
      if (!map.getSource('peaks-source')) {
        map.addSource('peaks-source', {
          type: 'vector',
          tiles: [peakTileUrl],
          minzoom: 0,
          maxzoom: 14,
        })
        lastPeaksUserId.current = currentUserId
      }
      if (!map.getLayer('peaks-layer')) {
        map.addLayer({
          id: 'peaks-layer',
          type: 'symbol',
          source: 'peaks-source',
          'source-layer': 'peaks',
          minzoom: 10,
          layout: {
            'icon-image': ['case', ['get', 'visited'], 'peak-visited', 'peak-unvisited'] as maplibregl.ExpressionSpecification,
            'icon-size': 0.8,
            'icon-allow-overlap': false,
            'text-field': ['get', 'name'] as maplibregl.ExpressionSpecification,
            'text-font': ['Open Sans Regular'],
            'text-size': 11,
            'text-offset': [0, 1.2],
            'text-anchor': 'top',
            'text-optional': true,
          },
          paint: {
            'text-color': '#374151',
            'text-halo-color': 'white',
            'text-halo-width': 1,
          },
          filter: overlayState.peaksOnlyVisited
            ? ['==', ['get', 'visited'], true]
            : ['literal', true],
        })
      } else {
        map.setFilter('peaks-layer', overlayState.peaksOnlyVisited
          ? ['==', ['get', 'visited'], true]
          : ['literal', true])
      }
    }

    if (overlayState.showPeaks) {
      if (!map.hasImage('peak-visited')) {
        let loaded = 0
        const onLoad = () => { if (++loaded === 2) addPeaksLayer() }
        const img1 = new Image(24, 24)
        const img2 = new Image(24, 24)
        img1.onload = () => { if (!map.hasImage('peak-visited')) map.addImage('peak-visited', img1); onLoad() }
        img2.onload = () => { if (!map.hasImage('peak-unvisited')) map.addImage('peak-unvisited', img2); onLoad() }
        img1.onerror = onLoad
        img2.onerror = onLoad
        img1.src = PEAK_ICON_VISITED
        img2.src = PEAK_ICON_UNVISITED
      } else {
        addPeaksLayer()
      }
    } else {
      if (map.getLayer('peaks-layer')) map.removeLayer('peaks-layer')
      if (map.getSource('peaks-source')) map.removeSource('peaks-source')
    }
  }, [overlayState.showPeaks, overlayState.peaksOnlyVisited, styleVersion, currentUserId]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Gewässer-Layer ────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/tiles`

    if (overlayState.showWaters) {
      if (!map.getSource('waters-source')) {
        map.addSource('waters-source', {
          type: 'vector',
          tiles: [`${martinBase}/waters/{z}/{x}/{y}`],
          minzoom: 0,
          maxzoom: 14,
        })
      }
      if (!map.getLayer('waters-layer')) {
        map.addLayer({
          id: 'waters-layer',
          type: 'line',
          source: 'waters-source',
          'source-layer': 'waters',
          minzoom: 8,
          paint: {
            'line-color': '#3b82f6',
            'line-width': ['interpolate', ['linear'], ['zoom'], 8, 1, 14, 2] as maplibregl.ExpressionSpecification,
            'line-opacity': 0.6,
          },
        })
      }
    } else {
      if (map.getLayer('waters-layer')) map.removeLayer('waters-layer')
      if (map.getSource('waters-source')) map.removeSource('waters-source')
    }
  }, [overlayState.showWaters, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps


  // ── Wald-Layer ────────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/tiles`
    const forestTileUrl = currentUserId
      ? `${martinBase}/forests/{z}/{x}/{y}?user_id=${encodeURIComponent(currentUserId)}`
      : `${martinBase}/forests/{z}/{x}/{y}`

    if (overlayState.showForests) {
      // Source neu laden wenn sich user_id geändert hat
      if (map.getSource('forests-source') && lastForestsUserId.current !== currentUserId) {
        if (map.getLayer('forests-layer')) map.removeLayer('forests-layer')
        map.removeSource('forests-source')
      }
      if (!map.getSource('forests-source')) {
        map.addSource('forests-source', {
          type: 'vector',
          tiles: [forestTileUrl],
          minzoom: 0,
          maxzoom: 14,
        })
        lastForestsUserId.current = currentUserId
      }
      if (!map.getLayer('forests-layer')) {
        map.addLayer({
          id: 'forests-layer',
          type: 'line',
          source: 'forests-source',
          'source-layer': 'forests',
          minzoom: 9,
          paint: {
            'line-color': ['case', ['get', 'visited'], '#22c55e', '#9ca3af'] as maplibregl.ExpressionSpecification,
            'line-width': 1.5,
            'line-opacity': 0.8,
          },
        })
      }
    } else {
      if (map.getLayer('forests-layer')) map.removeLayer('forests-layer')
      if (map.getSource('forests-source')) map.removeSource('forests-source')
    }
  }, [overlayState.showForests, styleVersion, currentUserId]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Besuchte Einkehr-Layer ────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    if (overlayState.showVisitedPois && visitedPoisGeoJSON) {
      if (map.getSource('visited-pois-source')) {
        (map.getSource('visited-pois-source') as maplibregl.GeoJSONSource).setData(visitedPoisGeoJSON)
      } else {
        map.addSource('visited-pois-source', { type: 'geojson', data: visitedPoisGeoJSON })
      }
      if (!map.getLayer('visited-pois-layer')) {
        map.addLayer({
          id: 'visited-pois-layer',
          type: 'circle',
          source: 'visited-pois-source',
          paint: {
            'circle-radius': 8,
            'circle-color': '#f97316',
            'circle-stroke-color': 'white',
            'circle-stroke-width': 2,
          },
        })
      }
    } else {
      if (map.getLayer('visited-pois-layer')) map.removeLayer('visited-pois-layer')
      if (map.getSource('visited-pois-source')) map.removeSource('visited-pois-source')
    }
  }, [overlayState.showVisitedPois, visitedPoisGeoJSON, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Amenities-Layer (Martin MVT) ──────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/tiles`

    if (overlayState.showAmenities) {
      const amenityTileUrl = currentUserId
        ? `${martinBase}/amenities/{z}/{x}/{y}?user_id=${encodeURIComponent(currentUserId)}`
        : `${martinBase}/amenities/{z}/{x}/{y}`

      // Reload source when user_id changes
      if (map.getSource('amenities-source') && lastAmenitiesUserId.current !== currentUserId) {
        if (map.getLayer('amenities-layer')) map.removeLayer('amenities-layer')
        map.removeSource('amenities-source')
      }
      if (!map.getSource('amenities-source')) {
        map.addSource('amenities-source', {
          type: 'vector',
          tiles: [amenityTileUrl],
          minzoom: 0,
          maxzoom: 14,
        })
        lastAmenitiesUserId.current = currentUserId
      }
      if (!map.getLayer('amenities-layer')) {
        map.addLayer({
          id: 'amenities-layer',
          type: 'circle',
          source: 'amenities-source',
          'source-layer': 'amenities',
          minzoom: 10,
          paint: {
            'circle-color': '#f97316',
            'circle-radius': 6,
            'circle-stroke-width': 1.5,
            'circle-stroke-color': '#fff',
          },
        })
      }
    } else {
      if (map.getLayer('amenities-layer')) map.removeLayer('amenities-layer')
      if (map.getSource('amenities-source')) map.removeSource('amenities-source')
    }
  }, [overlayState.showAmenities, styleVersion, currentUserId]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="fixed inset-0 z-40 flex flex-col" style={{ top: '3.5rem' }}>
      <div className="relative flex-1 min-h-0">
        <div
          ref={mapContainerRef}
          className="w-full h-full"
        />


        {/* Geocoding search */}
        <GeoSearch mapRef={mapRef} />

        {/* Base style switcher */}
        <div className="absolute bottom-4 left-4 z-10 flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md">
          {(Object.keys(BASE_STYLES) as BaseStyle[]).map((key) => (
            <button
              key={key}
              onClick={() => setBaseStyle(key)}
              className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                baseStyle === key
                  ? 'bg-accent-DEFAULT text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              {BASE_STYLES[key].label}
            </button>
          ))}
        </div>

        {/* View mode toggle + Lade-Indikator + Terra-Incognita-Badge
            Gemeinsamer Wrapper: flex-col + items-stretch → alle Kinder teilen automatisch
            die gleiche Breite (bestimmt durch den breitesten Inhalt, i.d.R. die Tabs).
            Wächst nur breiter, wenn ein Kind breiter ist; niemals schmäler als die Tabs. */}
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 flex flex-col items-stretch gap-1.5">

          {/* Lade-Indikator — erscheint während Tiles/API laden */}
          {(isMapTilesLoading || (viewMode === 'terra_incognita' && terraIncognitaFetching)) && (
            <div className="flex items-center justify-center gap-2 px-3 py-1.5 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm border border-neutral-300 dark:border-neutral-600 shadow-md pointer-events-none">
              <div className="w-3 h-3 rounded-full border-2 border-neutral-300 dark:border-neutral-600 border-t-orange-500 animate-spin flex-shrink-0" />
              <span className="text-xs font-medium text-neutral-700 dark:text-neutral-200 whitespace-nowrap">
                {viewMode === 'tracks' && 'Tracks werden geladen'}
                {viewMode === 'heatmap' && 'Heatmap wird gerendert'}
                {viewMode === 'terra_incognita' && 'Nebel zieht auf'}
                <span className="inline-flex gap-px ml-0.5">
                  <span className="animate-[bounce_1.2s_ease-in-out_0s_infinite]">.</span>
                  <span className="animate-[bounce_1.2s_ease-in-out_0.2s_infinite]">.</span>
                  <span className="animate-[bounce_1.2s_ease-in-out_0.4s_infinite]">.</span>
                </span>
              </span>
            </div>
          )}

          {/* Terra Incognita Flächen-Badge — erst sichtbar nachdem Ladevorgang abgeschlossen */}
          {viewMode === 'terra_incognita' && terraIncognitaData && !isMapTilesLoading && !terraIncognitaFetching && (
            <div className="flex justify-center px-3 py-1.5 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm border border-neutral-300 dark:border-neutral-600 shadow-md">
              <span className="text-xs font-medium text-neutral-700 dark:text-neutral-200 whitespace-nowrap">
                {terraIncognitaData.area_km2.toFixed(1)} km² entdeckt
              </span>
            </div>
          )}

          {/* View mode tabs */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md">
            <button
              onClick={() => {
                // Direkter synchroner Layer-Wechsel via Refs für sofortiges visuelles Feedback.
                // isStyleLoaded() wird NICHT geprüft: es gibt false zurück während Tiles noch
                // von Web-Workern dekodiert werden (_hasPendingWorkerUpdate), obwohl der Style-JSON
                // fertig ist und addSource/addLayer problemlos funktionieren.
                const map = mapRef.current
                if (map) {
                  removeOverlayLayers(map)
                  removeTerraIncognitaLayers(map)
                  if (filtersRef.current.sportTypes.size > 0) {
                    addOverlayLayers(map, filtersRef.current, 'tracks', teleportRef.current, samplingRef.current, currentUserIdRef.current)
                  }
                }
                setViewMode('tracks')
              }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                viewMode === 'tracks'
                  ? 'bg-accent-DEFAULT text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              Tracks
            </button>
            <button
              onClick={() => {
                const map = mapRef.current
                if (map) {
                  removeOverlayLayers(map)
                  removeTerraIncognitaLayers(map)
                  if (filtersRef.current.sportTypes.size > 0) {
                    addOverlayLayers(map, filtersRef.current, 'heatmap', teleportRef.current, samplingRef.current, currentUserIdRef.current)
                  }
                }
                setViewMode('heatmap')
              }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                viewMode === 'heatmap'
                  ? 'bg-accent-DEFAULT text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              Heatmap
            </button>
            <button
              onClick={() => {
                const map = mapRef.current
                if (map) {
                  removeOverlayLayers(map)
                  removeTerraIncognitaLayers(map)
                  const geom = terraIncognitaRef.current?.terra_incognita_area
                    ?? ({ type: 'Polygon', coordinates: [] } as GeoJSON.Geometry)
                  addTerraIncognitaLayers(map, geom)
                }
                setViewMode('terra_incognita')
              }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                viewMode === 'terra_incognita'
                  ? 'bg-accent-DEFAULT text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              Terra Incognita
            </button>
          </div>

        </div>

        {/* Toggle buttons for custom maps + photos + segments + indoor */}
        <div className="absolute top-4 right-[17.5rem] z-10 flex gap-1.5 flex-wrap justify-end">
          <button
            onClick={() => setShowIndoorAssign(!showIndoorAssign)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showIndoorAssign
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Indoor
          </button>
          <button
            onClick={() => setShowSegments(!showSegments)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showSegments
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Segmente
          </button>
          <button
            onClick={() => setShowPhotos(!showPhotos)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showPhotos
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Bilder
          </button>
          <button
            onClick={() => setShowCustomMaps(!showCustomMaps)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showCustomMaps
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Eigene Karten
          </button>
        </div>

        {/* Segments Overlay — Segment-Linien als Karten-Ebene */}
        <SegmentsOverlay
          map={mapRef.current}
          enabled={overlayState.showSegmentsOverlay}
          filters={segmentFilters}
          colorConfig={segmentColors}
          styleVersion={styleVersion}
        />

        {/* Custom Maps Panel */}
        {showCustomMaps && (
          <CustomMapsPanel
            onAddMarkerMode={setAddMarkerModeMapID}
            addMarkerModeMapID={addMarkerModeMapID}
            onClose={() => setShowCustomMaps(false)}
          />
        )}

        {/* Indoor Assign Panel */}
        {showIndoorAssign && (
          <IndoorAssignPanel
            locationTags={locationTags}
            onClose={() => setShowIndoorAssign(false)}
            onPickLocation={(name, color, activityIDs) => {
              setNewLocationPick({ name, color, activityIDs })
              setShowIndoorAssign(false)
            }}
            onAssigned={() => {
              queryClient.invalidateQueries({ queryKey: ['activities'] })
              queryClient.invalidateQueries({ queryKey: ['tags'] })
            }}
          />
        )}

        {/* Drop-Location Name Dialog */}
        {isDropMode && (
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-72 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-xl p-4 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Neuer Standort</span>
              <button
                onClick={() => { setNewLocationPick(null); setDropNameInput(''); setDropColorInput('#f97316') }}
                className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              {newLocationPick!.activityIDs.length} Aktivität(en) werden zugewiesen.
            </p>
            <input
              type="text"
              value={dropNameInput}
              onChange={(e) => setDropNameInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleDropConfirm() }}
              placeholder="Name des Standorts"
              autoFocus
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
            />
            <div className="space-y-1.5">
              <div className="flex items-center gap-1.5 flex-wrap">
                {['#f97316','#3b82f6','#10b981','#8b5cf6','#f43f5e','#06b6d4','#eab308','#6b7280'].map(c => (
                  <button
                    key={c}
                    type="button"
                    onClick={() => setDropColorInput(c)}
                    className="w-5 h-5 rounded-full border-2 cursor-pointer"
                    style={{ backgroundColor: c, borderColor: dropColorInput === c ? '#f97316' : 'transparent', transform: dropColorInput === c ? 'scale(1.15)' : 'scale(1)' }}
                  />
                ))}
              </div>
              <ColorPicker value={dropColorInput} onChange={setDropColorInput} />
            </div>
            <button
              onClick={handleDropConfirm}
              disabled={!dropNameInput.trim()}
              className="w-full py-1.5 text-xs font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              Standort erstellen + zuweisen
            </button>
          </div>
        )}

        {/* Kartenebenen */}
        <div className="border-t border-neutral-200 dark:border-neutral-700 pt-3 mt-3 px-3">
          <h3 className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
            Kartenebenen
          </h3>
          <div className="space-y-2">
            <div className="space-y-1">
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={overlayState.showPeaks}
                  onChange={e => updateOverlay('showPeaks', e.target.checked)}
                  className="rounded"
                />
                <span>Gipfel</span>
              </label>
              {overlayState.showPeaks && (
                <label className="flex items-center gap-2 text-xs cursor-pointer ml-5 text-neutral-500 dark:text-neutral-400">
                  <input
                    type="checkbox"
                    checked={overlayState.peaksOnlyVisited}
                    onChange={e => updateOverlay('peaksOnlyVisited', e.target.checked)}
                    className="rounded"
                  />
                  <span>Nur besuchte</span>
                </label>
              )}
            </div>
            <label className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={overlayState.showWaters}
                onChange={e => updateOverlay('showWaters', e.target.checked)}
                className="rounded"
              />
              <span>Gewässer</span>
            </label>
            <label className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={overlayState.showForests}
                onChange={e => updateOverlay('showForests', e.target.checked)}
                className="rounded"
              />
              <span>Wälder</span>
            </label>
            <label className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={overlayState.showVisitedPois}
                onChange={e => updateOverlay('showVisitedPois', e.target.checked)}
                className="rounded"
              />
              <span>Besuchte Einkehr</span>
            </label>
            <label className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={overlayState.showAmenities}
                onChange={e => updateOverlay('showAmenities', e.target.checked)}
                className="rounded"
              />
              <span>Einkehr (Tiles)</span>
            </label>
            <div className="space-y-1">
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={overlayState.showSegmentsOverlay}
                  onChange={e => updateOverlay('showSegmentsOverlay', e.target.checked)}
                  className="rounded"
                />
                <span>Segmente (Linien)</span>
              </label>
              {overlayState.showSegmentsOverlay && (
                <div className="ml-5 space-y-2">
                  {/* Terrain-Typ-Filter */}
                  <div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Typ:</div>
                    <div className="flex flex-wrap gap-1">
                      {([
                        { id: 'standard', label: 'Standard' },
                        { id: 'trail', label: 'Trail' },
                        { id: 'lift', label: 'Lift' },
                      ] as const).map(({ id, label }) => (
                        <button
                          key={id}
                          onClick={() => setSegmentFilters(f => ({
                            ...f,
                            terrainTypes: f.terrainTypes.includes(id)
                              ? f.terrainTypes.filter(t => t !== id)
                              : [...f.terrainTypes, id],
                          }))}
                          className={`px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${
                            segmentFilters.terrainTypes.includes(id)
                              ? 'bg-accent-DEFAULT text-white border-transparent'
                              : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400'
                          }`}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {/* Status-Filter */}
                  <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                    <input
                      type="checkbox"
                      checked={segmentFilters.riddenOnly}
                      onChange={e => setSegmentFilters(f => ({ ...f, riddenOnly: e.target.checked }))}
                      className="rounded"
                    />
                    <span>Nur gefahrene</span>
                  </label>

                  {/* Favoriten-Filter */}
                  <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                    <input
                      type="checkbox"
                      checked={segmentFilters.favoritesOnly}
                      onChange={e => setSegmentFilters(f => ({ ...f, favoritesOnly: e.target.checked }))}
                      className="rounded"
                    />
                    <span>Nur Favoriten</span>
                  </label>

                  {/* Sport-Gruppen-Filter */}
                  <div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Sportart:</div>
                    <div className="flex flex-wrap gap-1">
                      {([
                        { id: 'bike', label: 'Rad' },
                        { id: 'foot', label: 'Lauf' },
                        { id: 'ski', label: 'Winter' },
                        { id: 'paddle', label: 'Wasser' },
                        { id: 'generic', label: 'Sonstig' },
                      ] as const).map(({ id, label }) => (
                        <button
                          key={id}
                          onClick={() => setSegmentFilters(f => ({
                            ...f,
                            sportGroups: f.sportGroups.includes(id)
                              ? f.sportGroups.filter(g => g !== id)
                              : [...f.sportGroups, id],
                          }))}
                          className={`px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${
                            segmentFilters.sportGroups.includes(id)
                              ? 'bg-accent-DEFAULT text-white border-transparent'
                              : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400'
                          }`}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {/* Farb-Konfiguration */}
                  <div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Farben:</div>
                    <div className="grid grid-cols-2 gap-x-3 gap-y-1.5">
                      {([
                        { key: 'standard', label: 'Standard' },
                        { key: 'trail', label: 'Trail' },
                        { key: 'lift', label: 'Lift' },
                        { key: 'ridden', label: 'Gefahren' },
                        { key: 'favorite', label: 'Favoriten' },
                      ] as { key: keyof SegmentColorConfig; label: string }[]).map(({ key, label }) => (
                        <div key={key} className="flex items-center gap-1.5 text-xs">
                          <ColorPicker
                            value={segmentColors[key] ?? DEFAULT_SEGMENT_COLORS[key]}
                            onChange={hex => {
                              const next = { ...segmentColors, [key]: hex }
                              setSegmentColors(next)
                              localStorage.setItem('skadia-segment-colors', JSON.stringify(next))
                            }}
                          />
                          <span className="text-neutral-600 dark:text-neutral-300">{label}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Filters panel */}
        <MapFilters filters={filters} onChange={handleFilterChange} forceDark={baseStyle === 'dark'} viewMode={viewMode} />

        {/* Drag-to-merge Bestätigungs-Dialog */}
        {tagMergeDialog && (
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-80 rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-xl p-4 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Tags fusionieren?</span>
              <button
                onClick={() => {
                  // Roll back marker to original position
                  tagMergeDialog.marker.setLngLat([
                    tagMergeDialog.originalLngLat.lng,
                    tagMergeDialog.originalLngLat.lat,
                  ])
                  setTagMergeDialog(null)
                }}
                className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
              >
                <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            </div>
            <p className="text-xs text-neutral-600 dark:text-neutral-300 leading-relaxed">
              Tag{' '}
              <span className="font-semibold" style={{ color: tagMergeDialog.draggedTag.color }}>
                {tagMergeDialog.draggedTag.name}
              </span>{' '}
              mit{' '}
              <span className="font-semibold" style={{ color: tagMergeDialog.nearbyTag.color }}>
                {tagMergeDialog.nearbyTag.name}
              </span>{' '}
              fusionieren? Alle Aktivitäten werden{' '}
              <span className="font-semibold" style={{ color: tagMergeDialog.nearbyTag.color }}>
                {tagMergeDialog.nearbyTag.name}
              </span>{' '}
              zugewiesen.{' '}
              <span className="font-semibold" style={{ color: tagMergeDialog.draggedTag.color }}>
                {tagMergeDialog.draggedTag.name}
              </span>{' '}
              wird gelöscht.
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => {
                  mergeTagMutation.mutate({
                    targetId: tagMergeDialog.nearbyTag.id,
                    sourceId: tagMergeDialog.draggedTag.id,
                  })
                }}
                disabled={mergeTagMutation.isPending}
                className="flex-1 py-1.5 text-xs font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                {mergeTagMutation.isPending ? 'Fusioniere…' : 'Fusionieren'}
              </button>
              <button
                onClick={() => {
                  tagMergeDialog.marker.setLngLat([
                    tagMergeDialog.originalLngLat.lng,
                    tagMergeDialog.originalLngLat.lat,
                  ])
                  setTagMergeDialog(null)
                }}
                className="flex-1 py-1.5 text-xs font-medium rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
              >
                Abbrechen
              </button>
            </div>
          </div>
        )}

        {/* Location Tag Info Popup */}
        {selectedLocationTag && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
            onClick={() => { setSelectedLocationTagId(null); setTagGalleryOpen(false); setTagGallerySelectMode(false) }}
          >
            <div
              className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl w-full max-w-sm mx-4 overflow-hidden flex flex-col max-h-[85vh]"
              onClick={e => e.stopPropagation()}
            >
              {/* GPS Coordinates */}
              <div className="px-4 pt-3 pb-0 flex items-center justify-between flex-shrink-0">
                <span className="text-xs text-neutral-400 font-mono">
                  {selectedLocationTag.lat?.toFixed(5)}, {selectedLocationTag.lng?.toFixed(5)}
                </span>
                <button
                  onClick={() => { setSelectedLocationTagId(null); setTagGalleryOpen(false); setTagGallerySelectMode(false) }}
                  className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
                >
                  <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                    <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                  </svg>
                </button>
              </div>

              {/* Cover Image */}
              {selectedLocationTag.cover_image_url ? (
                <div className="relative flex-shrink-0 mt-2">
                  {coverImageBlobUrl ? (
                    <img src={coverImageBlobUrl} alt="" className="w-full h-36 object-cover" />
                  ) : (
                    <div className="w-full h-36 bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
                  )}
                  <button
                    onClick={async () => {
                      await clearTagCoverImage(selectedLocationTag.id)
                      queryClient.setQueryData<Tag[]>(['tags'], old =>
                        old?.map(t => t.id === selectedLocationTag.id
                          ? { ...t, cover_image_url: undefined }
                          : t
                        ) ?? []
                      )
                    }}
                    className="absolute top-1.5 right-1.5 bg-black/50 hover:bg-black/70 text-white rounded-full w-5 h-5 flex items-center justify-center text-xs cursor-pointer"
                    title="Bild entfernen"
                  >
                    <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                      <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                    </svg>
                  </button>
                </div>
              ) : null}

              {/* Cover image buttons */}
              <div className="px-4 pt-2 pb-0 flex gap-2 flex-shrink-0">
                <label className="flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400 hover:text-accent-DEFAULT cursor-pointer">
                  <input
                    type="file"
                    accept="image/*"
                    className="hidden"
                    onChange={async e => {
                      const file = e.target.files?.[0]
                      if (!file) return
                      const { uploadTagCoverImage } = await import('@/api/tags')
                      try {
                        const result = await uploadTagCoverImage(selectedLocationTag.id, file)
                        // Sofort im Cache aktualisieren → live ohne Refetch-Verzögerung
                        queryClient.setQueryData<Tag[]>(['tags'], old =>
                          old?.map(t => t.id === selectedLocationTag.id
                            ? { ...t, cover_image_url: result.cover_image_url }
                            : t
                          ) ?? []
                        )
                      } catch (err) {
                        console.error('Cover image upload failed:', err)
                      }
                    }}
                  />
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M1 8a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 018.07 3h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0016.07 6H17a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V8zm13.5 3a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zM10 14a2.5 2.5 0 100-5 2.5 2.5 0 000 5z" clipRule="evenodd"/>
                  </svg>
                  {selectedLocationTag.cover_image_url ? 'Ersetzen' : 'Bild hinzufügen'}
                </label>
                {tagPhotos.length > 0 && (
                  <button
                    onClick={() => { setTagGalleryOpen(true); setTagGallerySelectMode(true) }}
                    className="flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400 hover:text-accent-DEFAULT cursor-pointer"
                  >
                    <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clipRule="evenodd"/>
                    </svg>
                    Aus Aktivitäten
                  </button>
                )}
              </div>

              {/* Tag name + color */}
              <div className="px-4 pt-3 pb-2 flex items-center gap-3 flex-shrink-0 border-b border-neutral-100 dark:border-neutral-800">
                <span
                  className="inline-block px-3 py-1 rounded-full text-sm font-semibold cursor-pointer hover:opacity-80 transition-opacity"
                  style={{ backgroundColor: selectedLocationTag.color, color: selectedLocationTag.text_color }}
                  onClick={() => {
                    navigate(`/workshop/tags?edit=${selectedLocationTag.id}`)
                    setSelectedLocationTagId(null)
                  }}
                >
                  {selectedLocationTag.name}
                </span>
                <ColorPicker
                  value={selectedLocationTag.color}
                  onChange={async hex => {
                    await updateTag(selectedLocationTag.id, { color: hex })
                    queryClient.invalidateQueries({ queryKey: ['tags'] })
                  }}
                />
              </div>

              {/* Activity list */}
              <div className="flex-shrink-0">
                <div className="px-4 pt-2 pb-1">
                  <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">
                    Aktivitäten ({tagActivities.length})
                  </span>
                </div>
                {tagActivitiesLoading ? (
                  <div className="flex items-center justify-center py-4 text-sm text-neutral-400">Laden...</div>
                ) : tagActivities.length === 0 ? (
                  <div className="px-4 py-3 text-sm text-neutral-400 text-center">
                    {selectedLocationTag.description && (
                      <p className="mb-1 text-xs">{selectedLocationTag.description}</p>
                    )}
                    Noch keine Aktivitäten zugeordnet
                  </div>
                ) : (
                  <ul className="divide-y divide-neutral-100 dark:divide-neutral-800 max-h-[200px] overflow-y-auto">
                    {tagActivities.map(act => {
                      const displayTitle = act.title || act.title_auto || 'Aktivität'
                      const date = new Date(act.started_at).toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric' })
                      const dist = act.distance_m != null
                        ? act.distance_m >= 1000
                          ? `${(act.distance_m / 1000).toFixed(1)} km`
                          : `${Math.round(act.distance_m)} m`
                        : null
                      return (
                        <li key={act.id}>
                          <a
                            href={`/activities/${act.id}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center justify-between px-4 py-2 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
                            onClick={() => setSelectedLocationTagId(null)}
                          >
                            <div>
                              <p className="text-sm font-medium text-neutral-800 dark:text-neutral-200 truncate max-w-[180px]">{displayTitle}</p>
                              <p className="text-xs text-neutral-400">{date}</p>
                            </div>
                            {dist && <span className="text-xs text-neutral-400 ml-2 flex-shrink-0">{dist}</span>}
                          </a>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </div>

              {/* Gallery section */}
              {tagPhotos.length > 0 && (
                <div className="border-t border-neutral-100 dark:border-neutral-800 flex-shrink-0">
                  <button
                    onClick={() => setTagGalleryOpen(!tagGalleryOpen)}
                    className="w-full flex items-center justify-between px-4 py-2 text-xs text-neutral-500 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer"
                  >
                    <span>Galerie ({tagPhotos.length} Fotos)</span>
                    <svg className={`w-3.5 h-3.5 transition-transform ${tagGalleryOpen ? 'rotate-180' : ''}`} viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd"/>
                    </svg>
                  </button>
                  {tagGalleryOpen && (
                    <div className="px-3 pb-3 max-h-48 overflow-y-auto">
                      {tagGallerySelectMode && (
                        <p className="text-xs text-accent-DEFAULT mb-2 text-center">Foto als Titelbild wählen</p>
                      )}
                      <div className="grid grid-cols-3 gap-1.5">
                        {tagPhotos.map(photo => (
                          <div
                            key={photo.id}
                            className={`relative aspect-square rounded overflow-hidden cursor-pointer ${tagGallerySelectMode ? 'hover:ring-2 hover:ring-accent-DEFAULT' : ''}`}
                            onClick={async () => {
                              if (tagGallerySelectMode) {
                                const { setTagCoverImageFromPhoto } = await import('@/api/tags')
                                const result = await setTagCoverImageFromPhoto(selectedLocationTag.id, photo.full_url)
                                queryClient.setQueryData<Tag[]>(['tags'], old =>
                                  old?.map(t => t.id === selectedLocationTag.id
                                    ? { ...t, cover_image_url: result.cover_image_url }
                                    : t
                                  ) ?? []
                                )
                                setTagGallerySelectMode(false)
                              } else {
                                setTagLightboxPhoto(photo.full_url)
                              }
                            }}
                          >
                            <AuthenticatedImage src={photo.thumb_url} alt="" className="w-full h-full object-cover" />
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Tag photo lightbox */}
        {tagLightboxPhoto && (
          <div
            className="fixed inset-0 z-[60] flex items-center justify-center bg-black/80"
            onClick={() => setTagLightboxPhoto(null)}
          >
            <img src={tagLightboxPhoto} alt="" className="max-w-full max-h-full object-contain rounded shadow-2xl" />
          </div>
        )}

        {/* Photo Lightbox */}
        {selectedPhoto && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/70"
            onClick={() => setSelectedPhoto(null)}
          >
            <div
              className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-lg w-full mx-4 overflow-hidden"
              onClick={e => e.stopPropagation()}
            >
              <AuthenticatedImage
                src={selectedPhoto.thumb_url.replace('/thumb', '/full')}
                alt={selectedPhoto.activity_title ?? 'Foto'}
                className="w-full object-contain max-h-96"
              />
              <div className="p-4">
                {selectedPhoto.activity_id && selectedPhoto.activity_title && (
                  <a
                    href={`/activities/${selectedPhoto.activity_id}`}
                    className="text-sm font-medium text-accent-DEFAULT hover:underline"
                  >
                    {selectedPhoto.activity_title}
                  </a>
                )}
                {selectedPhoto.taken_at && (
                  <p className="text-xs text-neutral-500 mt-1">
                    {new Date(selectedPhoto.taken_at).toLocaleString('de-DE')}
                  </p>
                )}
                <button
                  onClick={() => setSelectedPhoto(null)}
                  className="mt-3 w-full text-xs text-neutral-400 hover:text-neutral-600 cursor-pointer"
                >
                  Schliessen
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

const HEATMAP_LAYER_IDS = [
  'heatmap-glow-outer',
  'heatmap-glow-mid',
  'heatmap-base',
  'heatmap-bright',
  'heatmap-hot',
  'heatmap-tracks-glow',
  'heatmap-lines',
] as const

function removeOverlayLayers(map: maplibregl.Map) {
  for (const id of HEATMAP_LAYER_IDS) {
    if (map.getLayer(id)) map.removeLayer(id)
  }
  if (map.getSource('heatmap')) map.removeSource('heatmap')
}

const TERRA_INCOGNITA_LAYER_IDS = [
  'terra-incognita-overlay',
  'terra-incognita-cloud-1',
  'terra-incognita-cloud-2',
  'terra-incognita-cloud-3',
  'terra-incognita-cloud-4',
] as const

function removeTerraIncognitaLayers(map: maplibregl.Map) {
  for (const id of TERRA_INCOGNITA_LAYER_IDS) {
    if (map.getLayer(id)) map.removeLayer(id)
  }
  if (map.getSource('terra-incognita')) map.removeSource('terra-incognita')
  if (map.getSource('terra-incognita-explored')) map.removeSource('terra-incognita-explored')
}

/**
 * Add terra incognita overlay: weißer Wolken-Nebel über unerkundeten Regionen.
 *
 * Aufbau (WebGL GPU-gerendert via MapLibre):
 *  1. Fill-Layer: opake weiße Wolkenmasse über dem gesamten unerkundeten Bereich.
 *  2. Feather-Layer (4×): Line-Layer mit gestaffeltem line-blur auf dem Rand des
 *     erkundeten Polygons. Die Wolkenfarbe "läuft" in die erkundete Fläche aus
 *     und erzeugt einen weichen, natürlichen Wolkenrand — kein harter Schnitt.
 *     Je weiter vom Rand, desto transparenter die ausgelaufene Wolke.
 *
 * Diese Funktion ist self-cleaning: ruft intern removeTerraIncognitaLayers() auf.
 */
function addTerraIncognitaLayers(map: maplibregl.Map, exploredGeometry: GeoJSON.Geometry) {
  // World-covering polygon (outer ring, counter-clockwise = RFC 7946 exterior)
  const worldRing: GeoJSON.Position[] = [
    [-180, -85], [180, -85], [180, 85], [-180, 85], [-180, -85],
  ]

  // PostGIS ST_AsGeoJSON returns exterior rings as CCW (RFC 7946).
  // GeoJSON holes (interior rings) must be CW → reverse the ring.
  const toHoleRing = (ring: GeoJSON.Position[]): GeoJSON.Position[] => ring.slice().reverse()

  const holes: GeoJSON.Position[][] = []
  if (exploredGeometry.type === 'Polygon' && exploredGeometry.coordinates.length > 0) {
    holes.push(toHoleRing(exploredGeometry.coordinates[0]))
  } else if (exploredGeometry.type === 'MultiPolygon') {
    for (const poly of exploredGeometry.coordinates) {
      if (poly.length > 0) holes.push(toHoleRing(poly[0]))
    }
  }

  const fogPolygon: GeoJSON.Feature<GeoJSON.Polygon> = {
    type: 'Feature',
    properties: {},
    geometry: { type: 'Polygon', coordinates: [worldRing, ...holes] },
  }

  // Defensive Bereinigung: verhindert "Source already exists"-Fehler.
  removeTerraIncognitaLayers(map)

  // Source 1: Fog-Polygon (Welt minus Erkundetes = Wolkenfläche)
  map.addSource('terra-incognita', {
    type: 'geojson',
    data: fogPolygon,
  })

  // Source 2: Erkundetes Polygon (für weichen Wolkenrand)
  map.addSource('terra-incognita-explored', {
    type: 'geojson',
    data: { type: 'Feature', properties: {}, geometry: exploredGeometry } as GeoJSON.Feature,
  })

  // Layer 1: Wolkenmasse — opake weiße Basis über unerkundeten Gebieten
  map.addLayer({
    id: 'terra-incognita-overlay',
    type: 'fill',
    source: 'terra-incognita',
    paint: {
      'fill-color': '#eef1f7',
      'fill-opacity': 0.92,
    },
  })

  // Layer 2–5: Wolkenrand-Auslauf (cloud dissolve effect)
  // Jeder Layer ist ein Line-Layer auf dem Rand des erkundeten Polygons.
  // Die Wolkenfarbe läuft durch line-blur in die erkundete Fläche aus —
  // von dickem Wolkenrand (innen) bis zum transparenten Auslauf (außen).
  // GPU-gerendert: kein Pixelraster, echter Farbverlauf.
  map.addLayer({
    id: 'terra-incognita-cloud-1',
    type: 'line',
    source: 'terra-incognita-explored',
    paint: { 'line-color': '#eef1f7', 'line-width': 120, 'line-opacity': 0.55, 'line-blur': 60 },
  })
  map.addLayer({
    id: 'terra-incognita-cloud-2',
    type: 'line',
    source: 'terra-incognita-explored',
    paint: { 'line-color': '#eef1f7', 'line-width': 65, 'line-opacity': 0.70, 'line-blur': 32 },
  })
  map.addLayer({
    id: 'terra-incognita-cloud-3',
    type: 'line',
    source: 'terra-incognita-explored',
    paint: { 'line-color': '#eef1f7', 'line-width': 30, 'line-opacity': 0.82, 'line-blur': 15 },
  })
  map.addLayer({
    id: 'terra-incognita-cloud-4',
    type: 'line',
    source: 'terra-incognita-explored',
    paint: { 'line-color': '#eef1f7', 'line-width': 12, 'line-opacity': 0.90, 'line-blur': 6 },
  })
}

function buildLineColor(filters: MapFilterState): string | maplibregl.ExpressionSpecification {
  if (filters.colorConfig.mode === 'uniform') {
    return filters.colorConfig.uniformColor
  }
  // Build match expression from taxonomy: each sport_type → its category color
  const entries: string[] = []
  for (const s of SPORT_DEFS) {
    const catKey = categoryForSport(s.sportType).key
    entries.push(s.sportType, filters.colorConfig.colors[catKey] ?? sportColor(s.sportType))
  }
  return [
    'match', ['get', 'sport_type'],
    ...entries,
    filters.colorConfig.uniformColor, // Fallback
  ] as unknown as maplibregl.ExpressionSpecification
}

/** Erzeugt aus einer Hex-Farbe die 5 Glow-Stufen fuer den Heatmap-Modus. */
function deriveHeatmapGlowColors(hex: string): [string, string, string, string, string] {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  const toHex = (n: number) => Math.round(Math.max(0, Math.min(255, n))).toString(16).padStart(2, '0')
  const rgb = (rr: number, gg: number, bb: number) => `#${toHex(rr)}${toHex(gg)}${toHex(bb)}`
  return [
    rgb(r * 0.15, g * 0.15, b * 0.15),   // glow-outer: sehr dunkel
    rgb(r * 0.35, g * 0.35, b * 0.35),   // glow-mid: dunkel
    hex,                                   // base: Original
    rgb(r + (255 - r) * 0.35, g + (255 - g) * 0.25, b * 0.6),  // bright: heller/waermer
    rgb(r + (255 - r) * 0.7, g + (255 - g) * 0.7, b + (255 - b) * 0.5),  // hot: sehr hell
  ]
}

function addOverlayLayers(
  map: maplibregl.Map,
  filters: MapFilterState,
  viewMode: ViewMode,
  teleportThresholdM: number,
  samplingIntervalM?: number,
  userId?: string,
) {
  // Layer und Source vollständig entfernen und neu aufbauen.
  // removeOverlayLayers() stellt sicher, dass kein "Source already exists"-Fehler entsteht.
  removeOverlayLayers(map)
  map.addSource('heatmap', {
    type: 'vector',
    tiles: [buildTileUrl('heatmap_lines', filters, teleportThresholdM, samplingIntervalM, userId)],
    minzoom: 0,
    maxzoom: 14,
  })

  if (viewMode === 'tracks') {
    const lineColor = buildLineColor(filters)
    const opacity = Math.pow(filters.opacity / 100, 2)
    const glowOpacity = opacity * 0.35

    // Subtiler Glow-Layer unter den eigentlichen Tracks
    map.addLayer({
      id: 'heatmap-tracks-glow',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': '#F97316',
        'line-width': 5,
        'line-opacity': glowOpacity,
        'line-blur': 2,
      },
    })

    // Eigentliche Track-Linien mit nutzerdefinierter Farbe
    map.addLayer({
      id: 'heatmap-lines',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': lineColor,
        'line-width': 2,
        'line-opacity': opacity,
      },
    })
  } else {
    // Heatmap: 5 gestapelte Glow-Layer — Alpha-Compositing erzeugt natuerliche
    // Helligkeitsunterschiede: viel befahrene Strecken leuchten heller/waermer.
    const [cOuter, cMid, cBase, cBright, cHot] = deriveHeatmapGlowColors(filters.heatmapColor)
    map.addLayer({
      id: 'heatmap-glow-outer',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': cOuter,
        'line-width': 16,
        'line-opacity': 0.10,
        'line-blur': 8,
      },
    })
    map.addLayer({
      id: 'heatmap-glow-mid',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': cMid,
        'line-width': 9,
        'line-opacity': 0.15,
        'line-blur': 4,
      },
    })
    map.addLayer({
      id: 'heatmap-base',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': cBase,
        'line-width': 4,
        'line-opacity': 0.28,
        'line-blur': 0,
      },
    })
    map.addLayer({
      id: 'heatmap-bright',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': cBright,
        'line-width': 2,
        'line-opacity': 0.20,
        'line-blur': 0,
      },
    })
    map.addLayer({
      id: 'heatmap-hot',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': cHot,
        'line-width': 1,
        'line-opacity': 0.12,
        'line-blur': 0,
      },
    })
  }
}
