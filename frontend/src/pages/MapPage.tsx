import { useRef, useEffect, useState, useCallback, type ChangeEvent, type KeyboardEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'
import CustomMapsPanel from '@/components/CustomMapsPanel'
import IndoorAssignPanel from '@/components/IndoorAssignPanel'
import ColorPicker from '@/components/ColorPicker'
import { getAllSportKeys, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { SPORT_DEFS, categoryForSport, sportColor } from '@/lib/sportTaxonomy'
import { getHeatmapSettings } from '@/api/settings'
import { listCustomMaps, listCustomMapMarkers, createCustomMapMarker } from '@/api/customMaps'
import { listMapPhotos, type MapPhotoEntry } from '@/api/activityPhotos'
import { fetchSegments, type Segment } from '@/api/segments'
import { fetchTags, updateTag, mergeTags, fetchTagActivities, syncTagLocation, type Tag, type TagActivity } from '@/api/tags'
import { updateActivity, fetchActivity } from '@/api/activities'

// Re-export so existing consumers (if any) of the old MapPage exports still compile.
export type { SportOption } from '@/lib/sportOptions'
export { SPORT_OPTIONS, getAllSportKeys } from '@/lib/sportOptions'

type BaseStyle = 'osm' | 'topo' | 'cyclosm' | 'satellite' | 'dark'
type ViewMode = 'tracks' | 'heatmap'

const BASE_STYLES: Record<BaseStyle, { label: string; tiles: string[]; attribution: string }> = {
  osm: {
    label: 'OSM',
    tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  },
  topo: {
    label: 'Topo',
    tiles: ['https://tile.opentopomap.org/{z}/{x}/{y}.png'],
    attribution: '&copy; <a href="https://opentopomap.org">OpenTopoMap</a>',
  },
  cyclosm: {
    label: 'CyclOSM',
    tiles: [
      'https://a.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
      'https://b.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
      'https://c.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png',
    ],
    attribution: '&copy; <a href="https://www.cyclosm.org">CyclOSM</a>',
  },
  satellite: {
    label: 'Satellit',
    tiles: ['https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}'],
    attribution: '&copy; <a href="https://www.esri.com">Esri</a>, Maxar, Earthstar Geographics',
  },
  dark: {
    label: 'Dark',
    tiles: [
      'https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
      'https://b.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
      'https://c.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png',
    ],
    attribution: '&copy; <a href="https://carto.com">CARTO</a> &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  },
}

function buildMapStyle(base: BaseStyle): maplibregl.StyleSpecification {
  const { tiles, attribution } = BASE_STYLES[base]
  return {
    version: 8,
    sources: {
      basemap: {
        type: 'raster',
        tiles,
        tileSize: 256,
        attribution,
        // maxzoom tells MapLibre the highest zoom tiles are actually available at.
        // Without this, MapLibre requests tiles at zoom 20+ (which don't exist →
        // blank tiles). With it, MapLibre overzooms zoom-19 tiles at higher zoom
        // levels instead of showing white.
        maxzoom: 19,
      },
    },
    layers: [
      {
        id: 'basemap-tiles',
        type: 'raster',
        source: 'basemap',
        minzoom: 0,
        // No maxzoom on layer — allows overzoomed rendering beyond zoom 19.
      },
    ],
  }
}

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
}

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
): string {
  // MapLibre v5 lädt Tiles in einem Web Worker. Worker können relative URLs
  // (/tiles/...) nicht auflösen → immer absolute URL erzwingen.
  // VITE_MARTIN_URL muss daher eine absolute URL sein (z.B. https://skadia.example.com/tiles).
  const martinBase = import.meta.env.VITE_MARTIN_URL ?? `${window.location.origin}/tiles`
  const base = `${martinBase}/${source}/{z}/{x}/{y}`
  const params = new URLSearchParams()

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
          className="w-full px-3 py-2 pr-8 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 dark:placeholder-neutral-500 focus:outline-none focus:ring-2 focus:ring-orange-500 focus:border-transparent"
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
                    ? 'bg-orange-500 text-white'
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

// Kartenstile, die als "hell" gelten — beim Wechsel zu Heatmap wird automatisch
// auf Dark gewechselt. Satellite bleibt unangetastet.
const LIGHT_STYLES: BaseStyle[] = ['osm', 'topo', 'cyclosm']

export default function MapPage() {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const [baseStyle, setBaseStyle] = useState<BaseStyle>('osm')
  const [filters, setFilters] = useState<MapFilterState>({
    ...defaultFilters,
    colorConfig: loadColorConfig(),
    heatmapColor: loadHeatmapColor(),
  })
  const [viewMode, setViewMode] = useState<ViewMode>('tracks')
  // Merkt sich, ob wir den Stil automatisch auf Dark gesetzt haben,
  // damit wir ihn beim Zurueckwechseln auch wieder zuruecksetzen koennen.
  const autoSwitchedToDark = useRef(false)
  const styleBeforeAutoSwitch = useRef<BaseStyle>('osm')

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

  // Location Tags
  const locationTagMarkersRef = useRef<maplibregl.Marker[]>([])
  const [selectedLocationTag, setSelectedLocationTag] = useState<Tag | null>(null)
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
  const { data: segmentData = [] } = useQuery({
    queryKey: ['map-segments', segmentBbox],
    queryFn: () => fetchSegments(segmentBbox ? { bbox: segmentBbox } : undefined),
    enabled: showSegments,
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
    enabled: filters.showLocationTags || showIndoorAssign,
  })
  const locationTags = allTags.filter(t => t.lat != null && t.lng != null)
  locationTagsRef.current = locationTags

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
  useEffect(() => { filtersRef.current = filters }, [filters])
  useEffect(() => { viewModeRef.current = viewMode }, [viewMode])
  useEffect(() => { teleportRef.current = teleportThresholdM }, [teleportThresholdM])
  useEffect(() => { samplingRef.current = samplingIntervalM }, [samplingIntervalM])

  // ── Einmalige Karteninitialisierung ─────────────────────────────────────
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: buildMapStyle('osm'),
      center: [10.45, 51.16],
      zoom: 6,
      attributionControl: { compact: false },
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    // Persistenter Handler: feuert bei JEDEM setStyle(), also initial und bei
    // Kartenstil-Wechsel. removeOverlayLayers() wird zuerst aufgerufen, damit
    // kein "Source already exists"-Fehler entsteht falls der Handler mehrfach
    // feuert (z. B. React StrictMode double-invoke).
    map.on('style.load', () => {
      map.resize()
      removeOverlayLayers(map)
      addOverlayLayers(map, filtersRef.current, viewModeRef.current, teleportRef.current, samplingRef.current)
      setStyleVersion(v => v + 1)
    })

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
  const prevBaseStyleRef = useRef<BaseStyle>('osm')
  useEffect(() => {
    const map = mapRef.current
    if (!map || prevBaseStyleRef.current === baseStyle) return
    prevBaseStyleRef.current = baseStyle

    const center = map.getCenter()
    const zoom = map.getZoom()

    map.setStyle(buildMapStyle(baseStyle))

    // Kameraposition nach Stilwechsel wiederherstellen
    map.once('style.load', () => {
      map.setCenter(center)
      map.setZoom(zoom)
    })
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Overlay bei Filter-/Modus-/Settings-Änderung aktualisieren ──────────
  // showLocationTags absichtlich NICHT in deps — das Flag beeinflusst nur
  // MapLibre-Marker, nicht die Tile-Layer → kein unnötiger Layer-Rebuild.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeOverlayLayers(map)
    if (filters.sportTypes.size > 0) {
      addOverlayLayers(map, filters, viewMode, teleportThresholdM, samplingIntervalM)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.sportTypes, filters.dateFrom, filters.dateTo, filters.opacity, filters.colorConfig, filters.heatmapColor, viewMode, heatmapSettings, teleportThresholdM, samplingIntervalM])

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

  // ── Click-Handler fuer Marker-Hinzufuegen ──────────────────────────────
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
      const name = window.prompt('Marker-Name:')
      if (!name) {
        setAddMarkerModeMapID(null)
        map!.getCanvas().style.cursor = ''
        return
      }
      createMarkerMutation.mutate({ mapID: addMarkerModeMapID, lat, lng, name })
      setAddMarkerModeMapID(null)
      map!.getCanvas().style.cursor = ''
    }

    map.on('click', handleClick)
    return () => {
      map.off('click', handleClick)
      map.getCanvas().style.cursor = ''
    }
  }, [addMarkerModeMapID]) // eslint-disable-line react-hooks/exhaustive-deps

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
      img.src = photo.thumb_url
      img.style.cssText = 'width: 100%; height: 100%; object-fit: cover;'
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

      const el = document.createElement('div')
      el.style.cssText = `
        display: inline-flex;
        flex-direction: column;
        align-items: center;
        cursor: pointer;
        width: 36px;
      `

      // Pin circle
      const circle = document.createElement('div')
      circle.style.cssText = `
        width: 32px;
        height: 32px;
        border-radius: 50%;
        background: ${tag.color || '#6366F1'};
        border: 2px solid rgba(255,255,255,0.9);
        box-shadow: 0 2px 6px rgba(0,0,0,0.4);
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

      el.appendChild(circle)
      el.appendChild(tail)

      el.addEventListener('click', (e) => {
        e.stopPropagation()
        setSelectedLocationTag(tag)
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
      if (e.dataTransfer?.types.includes('application/skadia-activities')) {
        e.preventDefault()
        if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
      }
    }

    async function onCanvasDrop(e: DragEvent) {
      e.preventDefault()
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
        await Promise.all(activityIDs.map(async (actId) => {
          try {
            const act = await fetchActivity(actId)
            const cur = act.tags ?? []
            if (!cur.includes(nearbyTag.name)) {
              await updateActivity(actId, { tags: [...cur, nearbyTag.name] })
            }
          } catch { /* ignorieren */ }
        }))
        queryClient.invalidateQueries({ queryKey: ['activities'] })
        queryClient.invalidateQueries({ queryKey: ['indoor-activities'] })
        syncTagLocation(nearbyTag.id).catch(() => {})
        setShowIndoorAssign(false)
      } else {
        // Kein naher Pin → neuen Standort anlegen
        setShowIndoorAssign(false)
        setNewLocationPick({ name: '', color: '#f97316', activityIDs, dropLngLat: { lng: lngLat.lng, lat: lngLat.lat } })
      }
    }

    canvas.addEventListener('dragover', onCanvasDragOver)
    canvas.addEventListener('drop', onCanvasDrop)
    return () => {
      canvas.removeEventListener('dragover', onCanvasDragOver)
      canvas.removeEventListener('drop', onCanvasDrop)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Segment Layer ──────────────────────────────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return

    // Cleanup old segment layers
    if (map.getLayer('segments-lines')) map.removeLayer('segments-lines')
    if (map.getSource('segments-geojson')) map.removeSource('segments-geojson')
    if (segmentPopupRef.current) {
      segmentPopupRef.current.remove()
      segmentPopupRef.current = null
    }

    if (!showSegments || segmentData.length === 0) return

    // Segments API returns segment metadata; we need track geometry which
    // the API could provide via a geojson field. For now, since segments
    // are just points/metadata, we show them as markers. If the API returns
    // a track/geometry field in the future, we can render lines.
    // For now, show as circle markers using centroid approximation from the
    // segment bbox / start coordinates if available.

    // Build GeoJSON from segments that have coordinate data
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
    if (!selectedLocationTag) {
      setTagActivities([])
      return
    }
    setTagActivitiesLoading(true)
    fetchTagActivities(selectedLocationTag.id)
      .then(acts => setTagActivities(acts))
      .catch(() => setTagActivities([]))
      .finally(() => setTagActivitiesLoading(false))
  }, [selectedLocationTag])

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
                  ? 'bg-orange-500 text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              {BASE_STYLES[key].label}
            </button>
          ))}
        </div>

        {/* View mode toggle */}
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md">
          <button
            onClick={() => {
              setViewMode('tracks')
              // Automatisch gesetzten Dark-Stil zuruecksetzen
              if (autoSwitchedToDark.current) {
                autoSwitchedToDark.current = false
                setBaseStyle(styleBeforeAutoSwitch.current)
              }
            }}
            className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
              viewMode === 'tracks'
                ? 'bg-orange-500 text-white'
                : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Tracks
          </button>
          <button
            onClick={() => {
              setViewMode('heatmap')
              // Automatisch auf Dark wechseln, wenn aktueller Stil ein heller Stil ist
              if (LIGHT_STYLES.includes(baseStyle)) {
                styleBeforeAutoSwitch.current = baseStyle
                autoSwitchedToDark.current = true
                setBaseStyle('dark')
              }
            }}
            className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
              viewMode === 'heatmap'
                ? 'bg-orange-500 text-white'
                : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Heatmap
          </button>
        </div>

        {/* Toggle buttons for custom maps + photos + segments + indoor */}
        <div className="absolute top-4 right-[17.5rem] z-10 flex gap-1.5 flex-wrap justify-end">
          <button
            onClick={() => setShowIndoorAssign(!showIndoorAssign)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showIndoorAssign
                ? 'bg-orange-500 text-white border-orange-500'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Indoor
          </button>
          <button
            onClick={() => setShowSegments(!showSegments)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showSegments
                ? 'bg-orange-500 text-white border-orange-500'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Segmente
          </button>
          <button
            onClick={() => setShowPhotos(!showPhotos)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showPhotos
                ? 'bg-orange-500 text-white border-orange-500'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Bilder
          </button>
          <button
            onClick={() => setShowCustomMaps(!showCustomMaps)}
            className={`px-3 py-1.5 text-xs font-medium rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
              showCustomMaps
                ? 'bg-orange-500 text-white border-orange-500'
                : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Eigene Karten
          </button>
        </div>

        {/* Custom Maps Panel */}
        {showCustomMaps && (
          <CustomMapsPanel
            onAddMarkerMode={setAddMarkerModeMapID}
            addMarkerModeMapID={addMarkerModeMapID}
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
              className="w-full px-2.5 py-1.5 text-xs rounded-lg border border-neutral-200 dark:border-neutral-700 bg-transparent text-neutral-900 dark:text-neutral-100 outline-none focus:border-orange-400"
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
              className="w-full py-1.5 text-xs font-medium rounded-lg bg-orange-500 text-white hover:bg-orange-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              Standort erstellen + zuweisen
            </button>
          </div>
        )}

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
                className="flex-1 py-1.5 text-xs font-medium rounded-lg bg-orange-500 text-white hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
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
            onClick={() => setSelectedLocationTag(null)}
          >
            <div
              className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-sm w-full mx-4 overflow-hidden max-h-[80vh] flex flex-col"
              onClick={e => e.stopPropagation()}
            >
              {/* Header */}
              <div className="p-4 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-3 flex-shrink-0">
                <span
                  className="inline-block px-3 py-1 rounded-full text-sm font-semibold"
                  style={{ backgroundColor: selectedLocationTag.color, color: selectedLocationTag.text_color }}
                >
                  {selectedLocationTag.name}
                </span>
                <span className="text-xs text-neutral-400 ml-auto">
                  {selectedLocationTag.lat?.toFixed(4)}, {selectedLocationTag.lng?.toFixed(4)}
                </span>
                <button
                  onClick={() => setSelectedLocationTag(null)}
                  className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer ml-1"
                >
                  ✕
                </button>
              </div>

              {/* Activity list */}
              <div className="overflow-y-auto flex-1">
                {tagActivitiesLoading ? (
                  <div className="flex items-center justify-center py-6 text-sm text-neutral-400">Laden...</div>
                ) : tagActivities.length === 0 ? (
                  <div className="px-4 py-6 text-sm text-neutral-400 text-center">
                    {selectedLocationTag.description && (
                      <p className="mb-2 text-xs">{selectedLocationTag.description}</p>
                    )}
                    Noch keine Aktivitäten zugeordnet
                  </div>
                ) : (
                  <ul className="divide-y divide-neutral-100 dark:divide-neutral-800">
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
                            className="flex items-center justify-between px-4 py-2.5 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
                            onClick={() => setSelectedLocationTag(null)}
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
            </div>
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
              <img
                src={selectedPhoto.thumb_url.replace('/thumb', '/full')}
                alt={selectedPhoto.activity_title ?? 'Foto'}
                className="w-full object-contain max-h-96"
              />
              <div className="p-4">
                {selectedPhoto.activity_id && selectedPhoto.activity_title && (
                  <a
                    href={`/activities/${selectedPhoto.activity_id}`}
                    className="text-sm font-medium text-orange-500 hover:underline"
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
) {
  // Linien-Layer fuer beide Modi
  map.addSource('heatmap', {
    type: 'vector',
    tiles: [buildTileUrl('heatmap_lines', filters, teleportThresholdM, samplingIntervalM)],
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
