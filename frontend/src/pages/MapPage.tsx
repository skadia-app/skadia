import { useRef, useEffect, useState, useCallback, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useLocation } from 'react-router-dom'
import maplibregl, { type RequestParameters } from 'maplibre-gl'
import { getToken } from '@/lib/auth'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'
import CustomMapsPanel from '@/components/CustomMapsPanel'
import IndoorAssignPanel from '@/components/IndoorAssignPanel'
import ColorPicker from '@/components/ColorPicker'
import RoutePlannerPanel from '@/components/route-planner/RoutePlannerPanel'
import RouteSuggestionDialog from '@/components/route-planner/RouteSuggestionDialog'
import RouteSaveDialog from '@/components/route-planner/RouteSaveDialog'
import RouteProfileChart from '@/components/route-planner/RouteProfileChart'
import RouteSurfacesBar from '@/components/route-planner/RouteSurfacesBar'
import RouteComparisonPanel, { SUGGESTION_COLORS } from '@/components/route-planner/RouteComparisonPanel'
import type { RouteSuggestion } from '@/api/routes'
import { useRoutePlanner } from '@/hooks/useRoutePlanner'
import { getTagTextColor, getTagBadgeStyle } from '@/lib/tagUtils'
import { fmtDateCompact } from '@/lib/format'
import { useRouteEditor } from '@/components/route-planner/useRouteEditor'
import { getAllSportKeys, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { SPORT_DEFS, categoryForSport, sportColor } from '@/lib/sportTaxonomy'
import { BASE_STYLES, buildRasterStyle, type BaseStyle } from '@/lib/mapStyles'
import {
  HEATMAP_SOURCE_ID,
  addHeatmapLayers as addHeatmapGlowLayers,
  removeHeatmapLayers,
} from '@/lib/mapHeatmap'
import { useTheme } from '@/lib/theme'
import { useAuthImage } from '@/hooks/useAuthImage'
import { useCoverageOverlays } from '@/hooks/useCoverageOverlays'
import { useModal } from '@/lib/modal'
import { getHeatmapSettings } from '@/api/settings'
import { listCustomMaps, listCustomMapMarkers, createCustomMapMarker, updateCustomMapMarker } from '@/api/customMaps'
import { listMapPhotos, type MapPhotoEntry } from '@/api/activityPhotos'
import { apiClient, apiDownload } from '@/api/client'
import AuthenticatedImage from '@/components/AuthenticatedImage'
import { fetchSegments, type Segment } from '@/api/segments'
import { fetchTags, updateTag, mergeTags, fetchTagActivities, syncTagLocation, clearTagCoverImage, assignActivitiesToTag, type Tag, type TagActivity } from '@/api/tags'
import { fetchActivity, updateActivity } from '@/api/activities'
import { SegmentsOverlay, DEFAULT_SEGMENT_COLORS, DEFAULT_SEGMENT_FILTERS, type SegmentColorConfig, type SegmentsOverlayFilters } from '@/components/Map/SegmentsOverlay'
import { fetchTerraIncognita, type TerraIncognitaCoverage } from '@/api/terraIncognita'
import { useToast } from '@/lib/toast'
import { haversineM, findNearbyLocationTag } from '@/lib/geoUtils'
import { removeTerraIncognitaLayers, addTerraIncognitaLayers } from '@/lib/mapTerraIncognita'
import GeoSearch from '@/components/map/GeoSearch'
import { useStickyHighlight } from '@/hooks/useStickyHighlight'
import { isF26Key } from '@/lib/descriptorFilter'

// Re-export so existing consumers (if any) of the old MapPage exports still compile.
export type { SportOption } from '@/lib/sportOptions'
export { SPORT_OPTIONS, getAllSportKeys } from '@/lib/sportOptions'

type ViewMode = 'off' | 'tracks' | 'heatmap' | 'terra_incognita'
const VALID_VIEW_MODES: readonly ViewMode[] = ['off', 'tracks', 'heatmap', 'terra_incognita']

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
  isElectric: boolean | null  // null=neutral, true=only E-Bike, false=exclude E-Bike
}

export interface MapOverlayState {
  showSummits: boolean
  summitsOnlyVisited: boolean
  showWaters: boolean
  showForests: boolean
  showVisitedPois: boolean
  showAmenities: boolean
  showSegmentsOverlay: boolean
}

const defaultOverlayState: MapOverlayState = {
  showSummits: false,
  summitsOnlyVisited: false,
  showWaters: false,
  showForests: false,
  showVisitedPois: false,
  showAmenities: false,
  showSegmentsOverlay: false,
}

const SUMMIT_ICON_VISITED = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">' +
  '<path d="M2 20 L8 8 L12 14 L16 8 L22 20 Z" fill="#f97316" stroke="white" stroke-width="1.5"/>' +
  '</svg>'
)}`
const SUMMIT_ICON_UNVISITED = `data:image/svg+xml,${encodeURIComponent(
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
  isElectric: null,
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

function buildTileUrl(
  source: 'heatmap_lines' | 'heatmap_points',
  filters: MapFilterState,
  teleportThresholdM: number, // 0 = kein Filter
  samplingIntervalM?: number, // undefined = kein Filter (Standardwert aus DB)
  userId?: string, // privacy: filter by current user
  electricTagId?: string, // ID of the E-Bike system tag (for tristate filter)
): string {
  // Heatmap tiles are routed through the Go API for authentication + per-user isolation.
  // MapLibre v5 lädt Tiles in einem Web Worker. Worker können relative URLs
  // nicht auflösen → immer absolute URL erzwingen.
  const martinBase = import.meta.env.VITE_MARTIN_URL ?? `${window.location.origin}/api/v1/tiles`
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
  // Merge E-Bike tristate filter into tag arrays
  const effectiveIncludeTags = [...includeTags]
  const effectiveExcludeTags = [...excludeTags]
  if (filters.isElectric === true && electricTagId) {
    effectiveIncludeTags.push(electricTagId)
  } else if (filters.isElectric === false && electricTagId) {
    effectiveExcludeTags.push(electricTagId)
  }
  const includeGear = (filters.gearFilters ?? []).filter(f => f.state === 'include').map(f => f.id)
  const excludeGear = (filters.gearFilters ?? []).filter(f => f.state === 'exclude').map(f => f.id)
  if (effectiveIncludeTags.length) params.set('include_tags', effectiveIncludeTags.join(','))
  if (effectiveExcludeTags.length) params.set('exclude_tags', effectiveExcludeTags.join(','))
  if (includeGear.length) params.set('include_gear', includeGear.join(','))
  if (excludeGear.length) params.set('exclude_gear', excludeGear.join(','))

  // Cache-buster: erzwingt frische Tiles nach MVT-Funktionsänderungen.
  // Bump nach jeder heatmap_lines_mvt / heatmap_points_mvt Migration.
  params.set('_v', '125')

  const qs = params.toString()
  return qs ? `${base}?${qs}` : base
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
  const lastSummitsUserId = useRef<string>('')
  const lastForestsUserId = useRef<string>('')
  const lastPoisUserId = useRef<string>('')

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
    staleTime: Infinity, // serverseitiger Cache — kein automatisches Re-fetch
  })

  const [isMapTilesLoading, setIsMapTilesLoading] = useState(false)

  // Location Tags
  const locationTagMarkersRef = useRef<maplibregl.Marker[]>([])
  const snapMarkerRef = useRef<{ marker: maplibregl.Marker; el: HTMLElement } | null>(null)
  const snapClearTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [selectedLocationTagId, setSelectedLocationTagId] = useState<string | null>(null)
  const [tagActivitiesLoading, setTagActivitiesLoading] = useState(false)
  const [tagActivities, setTagActivities] = useState<TagActivity[]>([])
  const [tagActivitiesVersion, setTagActivitiesVersion] = useState(0)
  const [newLocationPick, setNewLocationPick] = useState<{
    name: string
    color: string
    activityIDs: string[]
    dropLngLat?: { lng: number; lat: number }
  } | null>(null)
  const [showIndoorAssign, setShowIndoorAssign] = useState(false)
  const locationTagsRef = useRef<Tag[]>([])

  // ── Route Planner ─────────────────────────────────────────────────────────
  // Activated via toolbar button or URL param ?planner=1 (e.g. from RoutesPage)
  const searchParams = new URLSearchParams(location.search)
  const [plannerActive, setPlannerActive] = useState(() => searchParams.get('planner') === '1')
  const [showSuggestionDialog, setShowSuggestionDialog] = useState(false)
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [routeSuggestions, setRouteSuggestions] = useState<RouteSuggestion[] | null>(null)
  const [suggestionPartialSearch, setSuggestionPartialSearch] = useState(false)
  const suggestionCountRef = useRef(0)
  const [hoveredSuggestion, setHoveredSuggestion] = useState<number | null>(null)
  const [showProfileChart, setShowProfileChart] = useState(false)
  const [bottomBarCollapsed, setBottomBarCollapsed] = useState(false)
  const highlight = useStickyHighlight()
  const [filterPanelVisible, setFilterPanelVisible] = useState(() => {
    try {
      const saved = localStorage.getItem('skadia-filter-panel-visible')
      return saved !== null ? saved === 'true' : true
    } catch { return true }
  })

  const planner = useRoutePlanner()
  const { addToast } = useToast()

  // Clear URL param after reading it (avoid re-activating on back-navigation)
  useEffect(() => {
    if (searchParams.get('planner') === '1') {
      navigate('/map', { replace: true })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Load waypoints from navigation state (e.g. "Open in Route Planner" from ActivityDetailPage)
  useEffect(() => {
    const waypoints = (location.state as { plannerWaypoints?: import('@/api/routes').RouteWaypoint[] } | null)?.plannerWaypoints
    if (waypoints && waypoints.length >= 2) {
      setPlannerActive(true)
      planner.loadWaypoints(waypoints)
      // Clear location state so back-navigation doesn't re-load the waypoints
      navigate('/map', { replace: true })
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // When planner is deactivated: clear route state and hide sub-panels
  const handlePlannerClose = useCallback(() => {
    setPlannerActive(false)
    setShowSuggestionDialog(false)
    setShowSaveDialog(false)
    setShowProfileChart(false)
    setRouteSuggestions(null)
    setSuggestionPartialSearch(false)
    planner.clear()
  }, [planner])

  // Show profile chart automatically when segments are available
  useEffect(() => {
    if (planner.segments.length > 0 && planner.elevationProfile) {
      setShowProfileChart(true)
    }
  }, [planner.segments.length, planner.elevationProfile])

  // ── Route Suggestion map layers ──────────────────────────────────────────
  const SUGGESTION_OFFSETS: [number, number][] = useMemo(() => [[0, 0], [-3, 0], [3, 0]], [])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const cleanup = () => {
      for (let i = 0; i < suggestionCountRef.current; i++) {
        const layerId = `suggestion-layer-${i}`
        const sourceId = `suggestion-source-${i}`
        if (map.getLayer(layerId)) {
          map.off('mouseenter', layerId, handleMouseEnter)
          map.off('mouseleave', layerId, handleMouseLeave)
          map.removeLayer(layerId)
        }
        if (map.getSource(sourceId)) map.removeSource(sourceId)
      }
    }

    function handleMouseEnter(e: maplibregl.MapLayerMouseEvent) {
      map!.getCanvas().style.cursor = 'pointer'
      const feature = e.features?.[0]
      if (feature?.layer?.id) {
        const idx = parseInt(feature.layer.id.replace('suggestion-layer-', ''), 10)
        if (!isNaN(idx)) setHoveredSuggestion(idx)
      }
    }

    function handleMouseLeave() {
      map!.getCanvas().style.cursor = ''
      setHoveredSuggestion(null)
    }

    // Clean up old layers first
    cleanup()

    if (!routeSuggestions) return

    // Track how many layers are active so cleanup can iterate the correct count
    suggestionCountRef.current = Math.min(routeSuggestions.length, 3)

    // Add layers for each suggestion (initial opacity/width, hover handled separately)
    routeSuggestions.forEach((s, i) => {
      if (i >= 3) return // max 3 suggestions
      const sourceId = `suggestion-source-${i}`
      const layerId = `suggestion-layer-${i}`

      map.addSource(sourceId, {
        type: 'geojson',
        data: s.geojson,
      })

      map.addLayer({
        id: layerId,
        type: 'line',
        source: sourceId,
        paint: {
          'line-color': SUGGESTION_COLORS[i % SUGGESTION_COLORS.length],
          'line-width': 4,
          'line-opacity': 0.4,
          'line-translate': SUGGESTION_OFFSETS[i % SUGGESTION_OFFSETS.length],
        },
        layout: {
          'line-cap': 'round',
          'line-join': 'round',
        },
      })

      map.on('mouseenter', layerId, handleMouseEnter)
      map.on('mouseleave', layerId, handleMouseLeave)
    })

    // Fit bounds to show all suggestions
    if (routeSuggestions.length > 0 && routeSuggestions[0]?.geojson?.features?.[0]?.geometry?.coordinates?.length) {
      const allCoords: [number, number][] = []
      routeSuggestions.forEach(s => {
        const coords = s.geojson?.features?.[0]?.geometry?.coordinates
        if (coords) {
          for (const c of coords) {
            allCoords.push([c[0], c[1]])
          }
        }
      })
      if (allCoords.length > 0) {
        const bounds = new maplibregl.LngLatBounds(allCoords[0], allCoords[0])
        allCoords.forEach(c => bounds.extend(c))
        map.fitBounds(bounds, { padding: 80, maxZoom: 15 })
      }
    }

    return cleanup
  }, [routeSuggestions, SUGGESTION_OFFSETS])

  // Update suggestion layer paint properties on hover without rebuilding layers
  useEffect(() => {
    const map = mapRef.current
    if (!map || !routeSuggestions) return

    for (let i = 0; i < routeSuggestions.length; i++) {
      const layerId = `suggestion-layer-${i}`
      if (!map.getLayer(layerId)) continue
      const isHovered = hoveredSuggestion === i
      map.setPaintProperty(layerId, 'line-opacity', isHovered ? 0.9 : 0.4)
      map.setPaintProperty(layerId, 'line-width', isHovered ? 6 : 4)
    }
  }, [hoveredSuggestion, routeSuggestions])

  // Build RoutePlannerState for useRouteEditor
  const routeEditorState = useMemo(() => ({
    waypoints: planner.waypoints,
    segments: planner.segments,
    addWaypoint: (latlng: [number, number]) => void planner.addWaypoint(latlng),
    moveWaypoint: planner.moveWaypoint,
    removeWaypoint: planner.removeWaypoint,
    insertWaypoint: (afterId: string, latlng: [number, number]): string | null => {
      // Find the waypoint after which we insert
      const idx = planner.waypoints.findIndex(w => w.id === afterId)
      if (idx >= 0 && idx < planner.waypoints.length - 1) {
        const newWp: import('@/api/routes').RouteWaypoint = {
          id: crypto.randomUUID(),
          latlng,
          type: 'via',
        }
        // Insert waypoint into the list and remove only the affected segment.
        // Do NOT start calculation here — the point enters drag mode immediately.
        // moveWaypoint (on mouseup) will calculate the 2 flanking segments.
        planner.insertViaWaypoint(afterId, newWp)
        return newWp.id
      } else {
        void planner.addWaypoint(latlng)
        return null
      }
    },
    setWaypointType: (_id: string, _type: import('@/api/routes').RouteWaypoint['type']) => {
      // type assignment is managed automatically by useRoutePlanner
    },
    toggleOffroad: planner.toggleOffroad,
    updateControlPoint: planner.updateControlPoint,
    insertControlPoint: planner.insertControlPoint,
    removeControlPoint: planner.removeControlPoint,
    highlightedType: highlight.effective,
    isCalculating: planner.isCalculating,
  }), [planner, highlight.effective])

  // Register MapLibre click handlers for route editing
  useRouteEditor(mapRef, routeEditorState, plannerActive)

  // Drag-to-merge: dialog state
  const [tagMergeDialog, setTagMergeDialog] = useState<{
    draggedTag: Tag
    nearbyTag: Tag
    marker: maplibregl.Marker
    originalLngLat: { lng: number; lat: number }
  } | null>(null)

  // Style version counter for re-adding layers after style changes
  const [styleVersion, setStyleVersion] = useState(0)

  const { coverageState, updateComposite, toggleLayer: toggleCoverageLayer } = useCoverageOverlays(
    mapRef.current,
    styleVersion,
  )

  const queryClient = useQueryClient()
  const locationSyncTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

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
    queryFn: () => apiClient<{ user_id: string }>('/me'),
    staleTime: Infinity,
  })
  const currentUserId = meData?.user_id ?? ''

  const { data: visitedPoisGeoJSON } = useQuery({
    queryKey: ['visited-pois'],
    queryFn: () => apiClient<GeoJSON.FeatureCollection>('/geo/visited-pois'),
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
    enabled: true,
  })
  const locationTags = useMemo(
    () => allTags.filter(t => t.lat != null && t.lng != null),
    [allTags]
  )
  locationTagsRef.current = locationTags

  // E-Bike tag — resolved by name (includes non-system tags for users with pre-existing E-Bike tag)
  const electricTag = allTags.find(t => t.name === 'E-Bike')

  // Derive selected tag from fresh query data to avoid stale state
  const selectedLocationTag = selectedLocationTagId
    ? (locationTags.find(t => t.id === selectedLocationTagId) ?? null)
    : null
  const coverImageBlobUrl = useAuthImage(
    selectedLocationTag?.cover_file_id ? `/api/v1/tags/${selectedLocationTag.id}/cover` : undefined
  )

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
  const electricTagIdRef = useRef<string | undefined>(electricTag?.id)
  useEffect(() => { electricTagIdRef.current = electricTag?.id }, [electricTag])
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
      transformRequest: (url: string): RequestParameters => {
        // Attach JWT Bearer token to tile requests routed through the API proxy.
        if (url.includes('/api/v1/tiles/')) {
          const token = getToken()
          if (token) {
            return { url, headers: { Authorization: `Bearer ${token}` } }
          }
        }
        return { url }
      },
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
      // Nur aufräumen — Layer werden ausschließlich vom useEffect (styleVersion-Dep)
      // aufgebaut. Verhindert doppeltes addSource→removeSource→addSource, was bei
      // Client-Side Navigation zu gecancelten Tile-Requests und Lücken führte.
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
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
    // styleVersion === 0 bedeutet: style.load wurde noch nie gefeuert → Map noch nicht initialisiert.
    // isStyleLoaded() wird NICHT geprüft: es gibt false zurück während Tiles noch von
    // Web-Workern dekodiert werden, obwohl addSource/addLayer problemlos funktionieren.
    if (!map || styleVersion === 0) return
    if (viewMode === 'terra_incognita') {
      // Beim Wechsel zu Terra Incognita: Overlay-Source komplett entfernen (inkl. Tile-Cache),
      // da die Source für Terra Incognita nicht mehr benötigt wird.
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
      // Sofortiger Nebel (bei fehlendem terraIncognitaData: volle Welt-Abdeckung als Fallback).
      const rings = terraIncognitaRef.current?.rings ?? []
      if (rings.length) addTerraIncognitaLayers(map, rings)
    } else if (viewMode === 'off') {
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
    } else {
      removeOverlayLayers(map)
      removeTerraIncognitaLayers(map)
      if (filters.sportTypes.size > 0) {
        addOverlayLayers(map, filters, viewMode, teleportThresholdM, samplingIntervalM, currentUserId, electricTag?.id)
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.sportTypes, filters.dateFrom, filters.dateTo, filters.opacity, filters.colorConfig, filters.heatmapColor, filters.tagFilters, filters.gearFilters, filters.isElectric, viewMode, heatmapSettings, teleportThresholdM, samplingIntervalM, styleVersion, currentUserId, electricTag?.id])

  // ── Terra Incognita Overlay Layer ────────────────────────────────────────
  // styleVersion in deps: nach Stilwechsel muss der Overlay neu aufgebaut werden,
  // weil setStyle() alle Custom-Layers löscht. Dieser Effect aktualisiert die Ref
  // und baut den Layer neu auf. Hinweis: der style.load-Handler liest terraIncognitaRef
  // synchron, kann dabei aber eine veraltete (noch nicht durch diesen Effect befüllte)
  // Ref sehen. styleVersion stellt sicher, dass der Layer spätestens beim nächsten
  // styleVersion-Trigger korrekt vorhanden ist.
  useEffect(() => {
    const map = mapRef.current
    // styleVersion === 0: Map noch nicht initialisiert — isStyleLoaded() würde sowieso false liefern.
    // Nach dem ersten style.load (styleVersion >= 1) ist addSource/addLayer sicher, auch wenn
    // isStyleLoaded() wegen laufender Tile-Dekodierung noch false zurückgibt.
    if (!map || styleVersion === 0) return
    terraIncognitaRef.current = terraIncognitaData ?? null
    removeTerraIncognitaLayers(map)
    if (viewMode === 'terra_incognita' && terraIncognitaData?.rings?.length) {
      addTerraIncognitaLayers(map, terraIncognitaData.rings)
    }
  }, [viewMode, terraIncognitaData, styleVersion])

  // ── Custom Map Marker-Layer ──────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || styleVersion === 0) return

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
        color: ${getTagTextColor(tag.color || '#6366F1')};
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

      // Drop-Aktivitäten direkt auf den Marker (Marker liegt im DOM über dem Canvas,
      // deshalb kommt der Canvas-Drop-Handler nicht an wenn der Cursor genau auf dem Pin ist)
      el.addEventListener('dragover', (e) => {
        if (!e.dataTransfer?.types.includes('application/skadia-activities')) return
        e.preventDefault()
        e.stopPropagation()
        if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
        // Debounce-Timer canceln (verhindert Flicker wenn Cursor zwischen Canvas und Marker wechselt)
        if (snapClearTimerRef.current) { clearTimeout(snapClearTimerRef.current); snapClearTimerRef.current = null }
        if (!snapMarkerRef.current || snapMarkerRef.current.el !== el) {
          if (snapMarkerRef.current) {
            const prev = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
            if (prev) { prev.style.scale = ''; prev.style.transition = '' }
          }
          inner.style.transition = 'scale 0.15s ease'
          inner.style.scale = '1.35'
          snapMarkerRef.current = { marker, el }
        }
      })
      el.addEventListener('dragleave', (e) => {
        // Nur debounced clearen — canvas-dragover könnte gleich folgen
        if (snapClearTimerRef.current) clearTimeout(snapClearTimerRef.current)
        snapClearTimerRef.current = setTimeout(() => {
          if (snapMarkerRef.current?.el === el) {
            inner.style.scale = ''
            inner.style.transition = ''
            snapMarkerRef.current = null
          }
          snapClearTimerRef.current = null
        }, 120)
      })
      el.addEventListener('drop', async (e) => {
        e.preventDefault()
        e.stopPropagation()
        inner.style.scale = ''
        inner.style.transition = ''
        const raw = e.dataTransfer?.getData('application/skadia-activities')
        if (!raw) return
        let activityIDs: string[]
        try { activityIDs = JSON.parse(raw) } catch { return }
        try {
          await assignActivitiesToTag(tag.id, activityIDs)
          queryClient.invalidateQueries({ queryKey: ['activities'] })
          queryClient.invalidateQueries({ queryKey: ['unlocated-activities'] })
          queryClient.invalidateQueries({ queryKey: ['tags'] })
          syncTagLocation(tag.id).catch(() => {})
          setTagActivitiesVersion(v => v + 1)
        } catch (err) {
          console.error('assignActivitiesToTag fehlgeschlagen:', err)
        }
        setShowIndoorAssign(false)
        // Pulse
        inner.style.transition = 'scale 0.15s ease'
        inner.style.scale = '1.5'
        setTimeout(() => { inner.style.scale = '1.0' }, 150)
        setTimeout(() => { inner.style.transition = ''; inner.style.scale = '' }, 400)
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
      queryClient.invalidateQueries({ queryKey: ['unlocated-activities'] })
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

    function clearSnapNow() {
      if (snapClearTimerRef.current) { clearTimeout(snapClearTimerRef.current); snapClearTimerRef.current = null }
      if (snapMarkerRef.current) {
        const innerEl = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
        if (innerEl) { innerEl.style.scale = ''; innerEl.style.transition = '' }
        snapMarkerRef.current = null
      }
    }

    function scheduleSnapClear() {
      if (snapClearTimerRef.current) clearTimeout(snapClearTimerRef.current)
      snapClearTimerRef.current = setTimeout(clearSnapNow, 120)
    }

    function applySnap(markerEntry: maplibregl.Marker, markerEl: HTMLElement) {
      if (snapClearTimerRef.current) { clearTimeout(snapClearTimerRef.current); snapClearTimerRef.current = null }
      // Clear old snap if different marker
      if (snapMarkerRef.current && snapMarkerRef.current.el !== markerEl) {
        const prevInner = snapMarkerRef.current.el.firstElementChild as HTMLElement | null
        if (prevInner) { prevInner.style.scale = ''; prevInner.style.transition = '' }
        snapMarkerRef.current = null
      }
      if (!snapMarkerRef.current) {
        const innerEl = markerEl.firstElementChild as HTMLElement | null
        if (innerEl) { innerEl.style.transition = 'scale 0.15s ease'; innerEl.style.scale = '1.35' }
        snapMarkerRef.current = { marker: markerEntry, el: markerEl }
      }
    }

    // Pixel-Radius für Snap (zoomstufenunabhängig)
    const SNAP_PX = 60

    function findNearbyMarkerByPixel(cursorX: number, cursorY: number): maplibregl.Marker | null {
      const map = mapRef.current
      if (!map) return null
      let nearest: maplibregl.Marker | null = null
      let nearestDist = Infinity
      for (const m of locationTagMarkersRef.current) {
        const tagId = m.getElement().dataset.tagId
        const tag = locationTagsRef.current.find(t => t.id === tagId)
        if (!tag?.lat || !tag?.lng) continue
        const px = map.project([tag.lng, tag.lat])
        const d = Math.hypot(px.x - cursorX, px.y - cursorY)
        if (d < SNAP_PX && d < nearestDist) { nearest = m; nearestDist = d }
      }
      return nearest
    }

    function onCanvasDragOver(e: DragEvent) {
      if (!e.dataTransfer?.types.includes('application/skadia-activities')) return
      e.preventDefault()
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'

      const rect = canvas.getBoundingClientRect()
      const cursorX = e.clientX - rect.left
      const cursorY = e.clientY - rect.top

      const markerEntry = findNearbyMarkerByPixel(cursorX, cursorY)
      if (markerEntry) {
        applySnap(markerEntry, markerEntry.getElement())
      } else {
        scheduleSnapClear()
      }
    }

    function onCanvasDragLeave() {
      scheduleSnapClear()
    }

    async function onCanvasDrop(e: DragEvent) {
      e.preventDefault()
      // Clear magnetic snap visual
      clearSnapNow()
      const raw = e.dataTransfer?.getData('application/skadia-activities')
      if (!raw) return
      let activityIDs: string[]
      try { activityIDs = JSON.parse(raw) } catch { return }
      const rect = canvas.getBoundingClientRect()
      const cursorX = e.clientX - rect.left
      const cursorY = e.clientY - rect.top

      // Pixel-basierte Snap-Prüfung (zoomstufenunabhängig)
      const nearbyMarker = findNearbyMarkerByPixel(cursorX, cursorY)
      if (nearbyMarker) {
        const tagId = nearbyMarker.getElement().dataset.tagId!
        try {
          await assignActivitiesToTag(tagId, activityIDs)
          queryClient.invalidateQueries({ queryKey: ['activities'] })
          queryClient.invalidateQueries({ queryKey: ['unlocated-activities'] })
          queryClient.invalidateQueries({ queryKey: ['tags'] })
          syncTagLocation(tagId).catch(() => {})
          setTagActivitiesVersion(v => v + 1)
        } catch (err) {
          console.error('assignActivitiesToTag fehlgeschlagen:', err)
        }
        setShowIndoorAssign(false)
        // Pulse animation on successful drop
        const markerEl = nearbyMarker.getElement()
        const innerEl = markerEl.firstElementChild as HTMLElement | null
        if (innerEl) {
          innerEl.style.transition = 'scale 0.15s ease'
          innerEl.style.scale = '1.5'
          setTimeout(() => { innerEl.style.scale = '1.0' }, 150)
          setTimeout(() => { innerEl.style.transition = ''; innerEl.style.scale = '' }, 400)
        }
      } else {
        // Kein naher Pin → neuen Standort anlegen
        const lngLat = mapRef.current?.unproject([cursorX, cursorY])
        if (!lngLat) return
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
    if (!map || styleVersion === 0) return

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
  }, [selectedLocationTagId, tagActivitiesVersion])

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
    if (!map || styleVersion === 0) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/api/v1/tiles`
    const summitTileUrl = currentUserId
      ? `${martinBase}/summits/{z}/{x}/{y}?user_id=${encodeURIComponent(currentUserId)}`
      : `${martinBase}/summits/{z}/{x}/{y}`

    const addSummitsLayer = () => {
      // Source neu laden wenn sich user_id geändert hat (visited-Flag korrekt rendern)
      if (map.getSource('summits-source') && lastSummitsUserId.current !== currentUserId) {
        if (map.getLayer('summits-layer')) map.removeLayer('summits-layer')
        map.removeSource('summits-source')
      }
      if (!map.getSource('summits-source')) {
        map.addSource('summits-source', {
          type: 'vector',
          tiles: [summitTileUrl],
          minzoom: 0,
          maxzoom: 14,
        })
        lastSummitsUserId.current = currentUserId
      }
      if (!map.getLayer('summits-layer')) {
        map.addLayer({
          id: 'summits-layer',
          type: 'symbol',
          source: 'summits-source',
          'source-layer': 'summits',
          minzoom: 10,
          layout: {
            'icon-image': ['case', ['get', 'visited'], 'summit-visited', 'summit-unvisited'] as maplibregl.ExpressionSpecification,
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
          filter: overlayState.summitsOnlyVisited
            ? ['==', ['get', 'visited'], true]
            : ['literal', true],
        })
      } else {
        map.setFilter('summits-layer', overlayState.summitsOnlyVisited
          ? ['==', ['get', 'visited'], true]
          : ['literal', true])
      }
    }

    if (overlayState.showSummits) {
      if (!map.hasImage('summit-visited')) {
        let loaded = 0
        const onLoad = () => { if (++loaded === 2) addSummitsLayer() }
        const img1 = new Image(24, 24)
        const img2 = new Image(24, 24)
        img1.onload = () => { if (!map.hasImage('summit-visited')) map.addImage('summit-visited', img1); onLoad() }
        img2.onload = () => { if (!map.hasImage('summit-unvisited')) map.addImage('summit-unvisited', img2); onLoad() }
        img1.onerror = onLoad
        img2.onerror = onLoad
        img1.src = SUMMIT_ICON_VISITED
        img2.src = SUMMIT_ICON_UNVISITED
      } else {
        addSummitsLayer()
      }
    } else {
      if (map.getLayer('summits-layer')) map.removeLayer('summits-layer')
      if (map.getSource('summits-source')) map.removeSource('summits-source')
    }
  }, [overlayState.showSummits, overlayState.summitsOnlyVisited, styleVersion, currentUserId]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Gewässer-Layer ────────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || styleVersion === 0) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/api/v1/tiles`

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
    if (!map || styleVersion === 0) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/api/v1/tiles`
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
    if (!map || styleVersion === 0) return

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

  // ── POIs-Layer (Martin MVT) ───────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || styleVersion === 0) return
    const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? `${window.location.origin}/api/v1/tiles`

    if (overlayState.showAmenities) {
      const poisTileUrl = `${martinBase}/pois_mvt/{z}/{x}/{y}`

      // Reload source when user_id changes
      if (map.getSource('pois-source') && lastPoisUserId.current !== currentUserId) {
        if (map.getLayer('pois-layer')) map.removeLayer('pois-layer')
        map.removeSource('pois-source')
      }
      if (!map.getSource('pois-source')) {
        map.addSource('pois-source', {
          type: 'vector',
          tiles: [poisTileUrl],
          minzoom: 0,
          maxzoom: 14,
        })
        lastPoisUserId.current = currentUserId
      }
      if (!map.getLayer('pois-layer')) {
        map.addLayer({
          id: 'pois-layer',
          type: 'circle',
          source: 'pois-source',
          'source-layer': 'pois_mvt',
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
      if (map.getLayer('pois-layer')) map.removeLayer('pois-layer')
      if (map.getSource('pois-source')) map.removeSource('pois-source')
    }
  }, [overlayState.showAmenities, styleVersion, currentUserId]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="fixed inset-0 z-40 flex flex-col" style={{ top: '3.5rem' }}>
      <div className="flex flex-row flex-1 min-h-0">

        {/* ── Planner-Spalte (links, volle Höhe) ────────────────────────── */}
        {plannerActive && (
          <RoutePlannerPanel
            waypoints={planner.waypoints}
            segments={planner.segments}
            profileConfig={planner.profileConfig}
            showHeatmap={viewMode === 'heatmap'}
            routingEngine={planner.routingEngine}
            brouterProfile={planner.brouterProfile}
            badRouterAvailable={planner.capabilities?.routing ?? false}
            onRoutingEngineChange={planner.setRoutingEngine}
            onBrouterProfileChange={planner.setBrouterProfile}
            totalDistanceM={planner.totals.distance_m}
            totalElevationGain={planner.totals.elevation_gain}
            totalElevationLoss={planner.totals.elevation_loss}
            estimatedTimeMin={undefined}
            onProfileChange={planner.setProfileAndRecalculate}
            onRemoveWaypoint={planner.removeWaypoint}
            onReorderWaypoints={planner.reorderWaypoints}
            onRecalculateAll={planner.recalculateAll}
            onToggleSegmentMode={(waypointId) => {
              const seg = planner.segments.find(s => s.from_waypoint === waypointId)
              if (seg) planner.toggleOffroad({ from: seg.from_waypoint, to: seg.to_waypoint })
            }}
            onToggleHeatmap={(enabled) => {
              const map = mapRef.current
              if (!map) return
              removeOverlayLayers(map)
              removeTerraIncognitaLayers(map)
              if (enabled) {
                if (filtersRef.current.sportTypes.size > 0) {
                  addOverlayLayers(map, filtersRef.current, 'heatmap', teleportRef.current, samplingRef.current, currentUserIdRef.current, electricTagIdRef.current)
                }
                setViewMode('heatmap')
              } else {
                setViewMode('off')
              }
            }}
            onClearRoute={planner.clear}
            onSave={() => setShowSaveDialog(true)}
            onSuggest={() => setShowSuggestionDialog(true)}
            onClose={handlePlannerClose}
            calculationError={planner.calculationError}
            onClearError={planner.clearError}
            editingRoute={null}
            isSaving={planner.isSaving}
          />
        )}

        {/* ── Mittlere Spalte: Karte + Bottom-Bar ───────────────────────── */}
        <div className="flex flex-col flex-1 min-h-0 min-w-0">

        {/* ── Kartenbereich ─────────────────────────────────────────────── */}
        <div className="relative flex-1 min-h-0">
        <div
          ref={mapContainerRef}
          className="w-full h-full"
        />


        {/* ── Kartenstil-Buttons — links unten ────────────────────────── */}
        <div className="absolute bottom-4 left-4 z-10 flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md">
          {(Object.keys(BASE_STYLES) as BaseStyle[]).map((key) => {
            const isOfflineStyle = key === 'local' || key === 'offline'
            const offlineAvailable = planner.capabilities?.offline_map ?? false
            const disabled = isOfflineStyle && !offlineAvailable
            return (
              <button
                key={key}
                onClick={() => {
                  if (disabled) {
                    addToast({
                      type: 'info',
                      title: 'Lokale Karten nicht verfügbar',
                      message: 'Aktiviere lokale Kartendaten in den Einstellungen.',
                    })
                    return
                  }
                  setBaseStyle(key)
                }}
                title={disabled ? 'Lokale Kartendaten nicht verfügbar. Aktiviere sie in den Einstellungen.' : undefined}
                className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                  disabled
                    ? 'opacity-40 cursor-not-allowed text-neutral-600 dark:text-neutral-300'
                    : baseStyle === key
                      ? 'bg-accent-DEFAULT text-white cursor-pointer'
                      : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer'
                }`}
              >
                {BASE_STYLES[key].label}
              </button>
            )
          })}
        </div>

        {/* ── View-Mode + Badges — mittig unten ────────────────────────── */}
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 flex flex-col items-center gap-1.5">

          {/* Lade-Indikator */}
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

          {/* Terra Incognita Flächen-Badge */}
          {viewMode === 'terra_incognita' && terraIncognitaData && !isMapTilesLoading && !terraIncognitaFetching && (
            <div className="flex justify-center px-3 py-1.5 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm border border-neutral-300 dark:border-neutral-600 shadow-md">
              <span className="text-xs font-medium text-neutral-700 dark:text-neutral-200 whitespace-nowrap">
                {terraIncognitaData.area_km2.toFixed(1)} km² entdeckt
              </span>
            </div>
          )}

          {/* View-Mode-Gruppe */}
          <div className="flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md">
              <button
                onClick={() => {
                  const map = mapRef.current
                  if (map) {
                    removeOverlayLayers(map)
                    removeTerraIncognitaLayers(map)
                  }
                  setViewMode('off')
                }}
                className={`px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
                  viewMode === 'off'
                    ? 'bg-neutral-500 text-white'
                    : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
                }`}
              >
                Aus
              </button>
              <button
                onClick={() => {
                  const map = mapRef.current
                  if (map) {
                    removeOverlayLayers(map)
                    removeTerraIncognitaLayers(map)
                    if (filtersRef.current.sportTypes.size > 0) {
                      addOverlayLayers(map, filtersRef.current, 'tracks', teleportRef.current, samplingRef.current, currentUserIdRef.current, electricTagIdRef.current)
                    }
                  }
                  setViewMode('tracks')
                }}
                className={`px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
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
                      addOverlayLayers(map, filtersRef.current, 'heatmap', teleportRef.current, samplingRef.current, currentUserIdRef.current, electricTagIdRef.current)
                    }
                  }
                  setViewMode('heatmap')
                }}
                className={`px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
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
                    const rings = terraIncognitaRef.current?.rings ?? []
                    if (rings.length) addTerraIncognitaLayers(map, rings)
                  }
                  setViewMode('terra_incognita')
                }}
                className={`px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
                  viewMode === 'terra_incognita'
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
                }`}
              >
                Terra Inc.
              </button>
          </div>
        </div>

        {/* Geocoding search — centered on full map width */}
        <GeoSearch mapRef={mapRef} />

        {/* Toggle buttons — top right, links vom Filter-Toggle-Button */}
        <div className="absolute top-3 right-[5.5rem] z-10 flex gap-1.5 flex-wrap justify-end">
          {/* Route Planner toggle button */}
          {(() => {
            const plannerAvailable = planner.capabilities?.routing ?? false
            return (
          <button
            onClick={() => {
              if (!plannerAvailable) {
                addToast({ type: 'info', title: 'Routenplaner nicht verfügbar', message: 'Benötigt BRouter-Segmente oder einen BadRouter-Graph. Konfiguriere beides unter Einstellungen → Geodaten.' })
                return
              }
              if (plannerActive) {
                handlePlannerClose()
              } else {
                setPlannerActive(true)
              }
            }}
            title={plannerAvailable ? 'Routenplaner' : 'Routenplaner benötigt BRouter-Segmente oder BadRouter-Graph.'}
            className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors ${
              !plannerAvailable
                ? 'opacity-50 cursor-not-allowed bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600'
                : plannerActive
                  ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT cursor-pointer'
                  : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer'
            }`}
          >
            {/* Route icon (arrow path) */}
            <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M12.207 2.232a.75.75 0 00.025 1.06l4.146 3.958H6.375a5.375 5.375 0 000 10.75H9.75a.75.75 0 000-1.5H6.375a3.875 3.875 0 010-7.75h10.003l-4.146 3.957a.75.75 0 001.036 1.085l5.5-5.25a.75.75 0 000-1.085l-5.5-5.25a.75.75 0 00-1.061.025z" clipRule="evenodd" />
            </svg>
            Planer
          </button>
            )
          })()}
          <button
            onClick={() => setShowIndoorAssign(!showIndoorAssign)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showIndoorAssign
                ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Zuweisung
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

        {/* Route Suggestion Dialog */}
        {showSuggestionDialog && (() => {
          const map = mapRef.current
          const center = map ? map.getCenter() : { lat: 47.5, lng: 11.0 }
          return (
            <RouteSuggestionDialog
              start={[center.lng, center.lat]}
              onClose={() => setShowSuggestionDialog(false)}
              onResult={(response) => {
                if (response.suggestions.length === 0) {
                  addToast({ type: 'info', title: 'Keine Route gefunden',
                    message: 'Versuche eine groessere Distanz oder anderen Startpunkt', duration: 4000 })
                  return
                }
                setRouteSuggestions(response.suggestions)
                setSuggestionPartialSearch(response.partial_search)
                setShowSuggestionDialog(false)
              }}
            />
          )
        })()}

        {/* Route Comparison Panel — multi-route overlay */}
        {routeSuggestions && (
          <RouteComparisonPanel
            suggestions={routeSuggestions}
            partialSearch={suggestionPartialSearch}
            hoveredIndex={hoveredSuggestion}
            onHover={setHoveredSuggestion}
            onSelect={(index) => {
              const s = routeSuggestions[index]
              planner.clear()
              // Start point
              const coords = s.geojson?.features?.[0]?.geometry?.coordinates
              if (coords?.length >= 2) {
                void planner.addWaypoint([coords[0][1], coords[0][0]])
              }
              // Via point from suggestion
              if (s.via_point) {
                void planner.addWaypoint([s.via_point[1], s.via_point[0]])
              }
              // End point
              if (coords?.length >= 2) {
                void planner.addWaypoint([coords[coords.length - 1][1], coords[coords.length - 1][0]])
              }
              setRouteSuggestions(null)
            }}
            onCancel={() => setRouteSuggestions(null)}
            onHoverDescriptor={(key) => {
              if (key == null) { highlight.setHover(null); return }
              if (!isF26Key(key)) return // unknown key — no-op (defensive)
              highlight.setHover({ kind: 'descriptor', descriptorKey: key })
            }}
            onClickDescriptor={(key) => {
              if (!isF26Key(key)) return
              highlight.handleClick({ kind: 'descriptor', descriptorKey: key })
            }}
          />
        )}

        {/* Route Save Dialog */}
        {showSaveDialog && (
          <RouteSaveDialog
            waypoints={planner.waypoints}
            segments={planner.segments}
            currentProfile={planner.profile}
            editingRouteId={planner.editingRouteId}
            onClose={() => setShowSaveDialog(false)}
            onSaved={(saved) => {
              planner.loadRoute(saved)
              setShowSaveDialog(false)
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
              {selectedLocationTag.cover_file_id ? (
                <div className="relative flex-shrink-0 mt-2">
                  {coverImageBlobUrl ? (
                    <img src={coverImageBlobUrl} alt="" className="w-full h-36 object-cover" />
                  ) : (
                    <div className="w-full h-36 bg-neutral-100 dark:bg-neutral-800 animate-pulse" />
                  )}
                  <button
                    onClick={async () => {
                      const clearedTag = await clearTagCoverImage(selectedLocationTag.id)
                      queryClient.setQueryData<Tag[]>(['tags'], old =>
                        old?.map(t => t.id === selectedLocationTag.id ? clearedTag : t) ?? []
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
                        const updatedTag = await uploadTagCoverImage(selectedLocationTag.id, file)
                        // Sofort im Cache aktualisieren → live ohne Refetch-Verzögerung
                        queryClient.setQueryData<Tag[]>(['tags'], old =>
                          old?.map(t => t.id === selectedLocationTag.id ? updatedTag : t) ?? []
                        )
                      } catch (err) {
                        console.error('Cover image upload failed:', err)
                      }
                    }}
                  />
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M1 8a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 018.07 3h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0016.07 6H17a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V8zm13.5 3a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zM10 14a2.5 2.5 0 100-5 2.5 2.5 0 000 5z" clipRule="evenodd"/>
                  </svg>
                  {selectedLocationTag.cover_file_id ? 'Ersetzen' : 'Bild hinzufügen'}
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
                  style={getTagBadgeStyle(selectedLocationTag.color)}
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
                      const date = fmtDateCompact(act.started_at, act.timezone)
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
                                const updatedTag = await setTagCoverImageFromPhoto(selectedLocationTag.id, photo.full_url, photo.id)
                                queryClient.setQueryData<Tag[]>(['tags'], old =>
                                  old?.map(t => t.id === selectedLocationTag.id ? updatedTag : t) ?? []
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
        </div>{/* Ende: Kartenbereich (relative flex-1 min-h-0) */}

        {/* ── Bottom-Bar: Höhenprofil (gedockt, ein-/ausklappbar) ────────── */}
        {plannerActive && showProfileChart && planner.elevationProfile && planner.elevationProfile.length > 0 && (
          <div className="flex-shrink-0 border-t border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
            {/* Toggle-Tab */}
            <div className="flex items-center justify-between px-4 py-1.5 border-b border-neutral-100 dark:border-neutral-800">
              <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400">Höhenprofil</span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setBottomBarCollapsed(c => !c)}
                  className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
                  aria-label={bottomBarCollapsed ? 'Höhenprofil einblenden' : 'Höhenprofil ausblenden'}
                  title={bottomBarCollapsed ? 'Einblenden' : 'Ausblenden'}
                >
                  <svg className={`w-3.5 h-3.5 transition-transform ${bottomBarCollapsed ? 'rotate-180' : ''}`} viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                  </svg>
                </button>
                <button
                  onClick={() => setShowProfileChart(false)}
                  className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
                  aria-label="Höhenprofil schließen"
                  title="Schließen"
                >
                  <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                    <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                  </svg>
                </button>
              </div>
            </div>
            {!bottomBarCollapsed && (
              <div className="px-3 py-2">
                <RouteProfileChart
                  profile={planner.elevationProfile}
                  totalDistanceM={planner.totals.distance_m}
                  segments={planner.segments}
                  height={140}
                />
                <RouteSurfacesBar segments={planner.segments} onHighlight={highlight.setHover} />
              </div>
            )}
          </div>
        )}

        </div>{/* Ende: Mittlere Spalte (flex flex-col flex-1 min-h-0 min-w-0) */}

        {/* ── Filter-Panel-Spalte (rechts, volle Höhe, ein-/ausklappbar) ── */}
        <div className={`flex-shrink-0 flex transition-[width] duration-200 ease-in-out ${filterPanelVisible ? 'w-64' : 'w-5'}`}>
          {/* Collapse toggle tab */}
          <button
            onClick={() => {
              const next = !filterPanelVisible
              setFilterPanelVisible(next)
              try { localStorage.setItem('skadia-filter-panel-visible', String(next)) } catch { /* ignore */ }
            }}
            className="w-5 flex-shrink-0 flex items-center justify-center bg-neutral-50 dark:bg-neutral-800 border-l border-neutral-200 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-700 cursor-pointer text-neutral-400 dark:text-neutral-500 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            title={filterPanelVisible ? 'Filter ausblenden' : 'Filter einblenden'}
          >
            <svg className={`w-3 h-3 transition-transform ${filterPanelVisible ? '' : 'rotate-180'}`} viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
            </svg>
          </button>
          {/* Panel content */}
          {filterPanelVisible && (
            <div className="flex-1 border-l border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 overflow-y-auto">
              <MapFilters
                filters={filters}
                onChange={handleFilterChange}
                forceDark={baseStyle === 'dark'}
                viewMode={viewMode}
                overlayState={overlayState}
                onOverlayChange={updateOverlay}
                segmentFilters={segmentFilters}
                onSegmentFiltersChange={(f) => setSegmentFilters(f)}
                segmentColors={segmentColors}
                onSegmentColorsChange={(c) => setSegmentColors(c)}
                coverageState={coverageState}
                onCoverageUpdate={updateComposite}
                onCoverageToggleLayer={toggleCoverageLayer}
                docked
              />
            </div>
          )}
        </div>

      </div>{/* Ende: Zeilen-Wrapper (flex flex-row flex-1 min-h-0) */}
    </div>
  )
}

const TRACKS_LAYER_IDS = ['heatmap-tracks-glow', 'heatmap-lines'] as const

function removeOverlayLayers(map: maplibregl.Map) {
  for (const id of TRACKS_LAYER_IDS) {
    if (map.getLayer(id)) map.removeLayer(id)
  }
  removeHeatmapLayers(map)
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


function addOverlayLayers(
  map: maplibregl.Map,
  filters: MapFilterState,
  viewMode: ViewMode,
  teleportThresholdM: number,
  samplingIntervalM?: number,
  userId?: string,
  electricTagId?: string,
) {
  // Layer und Source vollständig entfernen und neu aufbauen.
  // removeOverlayLayers() stellt sicher, dass kein "Source already exists"-Fehler entsteht.
  removeOverlayLayers(map)

  const tileUrl = buildTileUrl('heatmap_lines', filters, teleportThresholdM, samplingIntervalM, userId, electricTagId)

  if (viewMode === 'tracks') {
    map.addSource(HEATMAP_SOURCE_ID, {
      type: 'vector',
      tiles: [tileUrl],
      minzoom: 0,
      maxzoom: 14,
    })

    const lineColor = buildLineColor(filters)
    const opacity = Math.pow(filters.opacity / 100, 2)
    const glowOpacity = opacity * 0.35

    // Subtiler Glow-Layer unter den eigentlichen Tracks
    map.addLayer({
      id: 'heatmap-tracks-glow',
      type: 'line',
      source: HEATMAP_SOURCE_ID,
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
      source: HEATMAP_SOURCE_ID,
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
    addHeatmapGlowLayers(map, tileUrl, filters.heatmapColor, undefined, Math.pow(filters.opacity / 100, 2))
  }
}
