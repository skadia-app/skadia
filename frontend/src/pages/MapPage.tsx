import { useRef, useEffect, useState, useCallback, type ChangeEvent, type KeyboardEvent } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'
import CustomMapsPanel from '@/components/CustomMapsPanel'
import IndoorAssignPanel from '@/components/IndoorAssignPanel'
import { getAllSportKeys, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { getHeatmapSettings } from '@/api/settings'
import { listCustomMaps, listCustomMapMarkers, createCustomMapMarker } from '@/api/customMaps'
import { listMapPhotos, type MapPhotoEntry } from '@/api/activityPhotos'
import { fetchSegments, type Segment } from '@/api/segments'
import { fetchTags, updateTag, type Tag } from '@/api/tags'

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
  showLocationTags: false,
}

function loadColorConfig(): SportColorConfig {
  try {
    const saved = localStorage.getItem('skadia-sport-colors')
    if (saved) return { ...defaultFilters.colorConfig, ...JSON.parse(saved) }
  } catch { /* ignorieren */ }
  return defaultFilters.colorConfig
}

function buildTileUrl(
  source: 'heatmap_lines' | 'heatmap_points',
  filters: MapFilterState,
  teleportThresholdM: number, // 0 = kein Filter
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
  const [newLocationPick, setNewLocationPick] = useState<{
    name: string
    color: string
    activityIDs: string[]
  } | null>(null)
  const [showIndoorAssign, setShowIndoorAssign] = useState(false)

  // Style version counter for re-adding layers after style changes
  const [styleVersion, setStyleVersion] = useState(0)

  const queryClient = useQueryClient()

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

  const teleportThresholdM = heatmapSettings?.teleport_filter_map
    ? (heatmapSettings.teleport_threshold_m ?? 2000)
    : 0

  // Refs damit der style.load-Handler immer die aktuellen Werte sieht,
  // ohne jedes Mal neu registriert werden zu müssen.
  const filtersRef = useRef(filters)
  const viewModeRef = useRef(viewMode)
  const teleportRef = useRef(teleportThresholdM)
  useEffect(() => { filtersRef.current = filters }, [filters])
  useEffect(() => { viewModeRef.current = viewMode }, [viewMode])
  useEffect(() => { teleportRef.current = teleportThresholdM }, [teleportThresholdM])

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
      addOverlayLayers(map, filtersRef.current, viewModeRef.current, teleportRef.current)
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
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    removeOverlayLayers(map)
    addOverlayLayers(map, filters, viewMode, teleportThresholdM)
  }, [filters, viewMode, heatmapSettings])

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

      // Build bubble element
      const el = document.createElement('div')
      el.style.cssText = `
        position: relative;
        cursor: pointer;
      `

      const bubble = document.createElement('div')
      bubble.style.cssText = `
        background: ${tag.color || '#6366F1'};
        color: ${tag.text_color || '#ffffff'};
        padding: 4px 10px;
        border-radius: 12px;
        font-size: 12px;
        font-weight: 600;
        white-space: nowrap;
        box-shadow: 0 2px 8px rgba(0,0,0,0.3);
        border: 2px solid rgba(255,255,255,0.3);
        user-select: none;
      `
      bubble.textContent = tag.name + (tag.activity_count > 0 ? ` (${tag.activity_count})` : '')

      // CSS arrow below bubble
      const arrow = document.createElement('div')
      arrow.style.cssText = `
        position: absolute;
        bottom: -7px;
        left: 50%;
        transform: translateX(-50%);
        width: 0;
        height: 0;
        border-left: 6px solid transparent;
        border-right: 6px solid transparent;
        border-top: 7px solid ${tag.color || '#6366F1'};
      `

      el.appendChild(bubble)
      el.appendChild(arrow)

      el.addEventListener('click', (e) => {
        e.stopPropagation()
        setSelectedLocationTag(tag)
      })

      const marker = new maplibregl.Marker({ element: el, draggable: true, anchor: 'bottom' })
        .setLngLat([tag.lng, tag.lat])
        .addTo(map)

      marker.on('dragend', () => {
        const { lat, lng } = marker.getLngLat()
        // Optimistic update
        queryClient.setQueryData<Tag[]>(['tags'], (prev) =>
          prev ? prev.map(t => t.id === tag.id ? { ...t, lat, lng } : t) : prev
        )
        updateTagLocationMutation.mutate({ id: tag.id, lat, lng })
      })

      locationTagMarkersRef.current.push(marker)
    })

    return () => {
      locationTagMarkersRef.current.forEach(m => m.remove())
      locationTagMarkersRef.current = []
    }
  }, [filters.showLocationTags, locationTags, styleVersion]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Crosshair-Modus für neues Standort-Tag ──────────────────────────────
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (!newLocationPick) {
      map.getCanvas().style.cursor = ''
      return
    }
    map.getCanvas().style.cursor = 'crosshair'

    async function handleLocationClick(e: maplibregl.MapMouseEvent) {
      if (!newLocationPick) return
      const { lat, lng } = e.lngLat
      const { createTag } = await import('@/api/tags')
      const { updateActivity } = await import('@/api/activities')

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
          // Fetch each activity to get current tags, then PATCH
          const { fetchActivity } = await import('@/api/activities')
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
    localStorage.setItem('skadia-sport-colors', JSON.stringify(next.colorConfig))
  }, [])

  return (
    <div className="fixed inset-0 z-40 flex flex-col" style={{ top: '3.5rem' }}>
      <div className="relative flex-1 min-h-0">
        <div ref={mapContainerRef} className="w-full h-full" />

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

        {/* Filters panel */}
        <MapFilters filters={filters} onChange={handleFilterChange} forceDark={baseStyle === 'dark'} />

        {/* Location Tag Info Popup */}
        {selectedLocationTag && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
            onClick={() => setSelectedLocationTag(null)}
          >
            <div
              className="bg-white dark:bg-neutral-900 rounded-xl shadow-2xl max-w-sm w-full mx-4 overflow-hidden"
              onClick={e => e.stopPropagation()}
            >
              <div className="p-4 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-3">
                <span
                  className="inline-block px-3 py-1 rounded-full text-sm font-semibold"
                  style={{ backgroundColor: selectedLocationTag.color, color: selectedLocationTag.text_color }}
                >
                  {selectedLocationTag.name}
                </span>
                <span className="text-xs text-neutral-400 ml-auto">
                  {selectedLocationTag.lat?.toFixed(4)}, {selectedLocationTag.lng?.toFixed(4)}
                </span>
              </div>
              <div className="p-4">
                <p className="text-sm text-neutral-600 dark:text-neutral-400">
                  {selectedLocationTag.activity_count > 0
                    ? `${selectedLocationTag.activity_count} Aktivitäten mit diesem Tag`
                    : 'Noch keine Aktivitäten zugeordnet'}
                </p>
                {selectedLocationTag.description && (
                  <p className="text-xs text-neutral-500 mt-1">{selectedLocationTag.description}</p>
                )}
              </div>
              <div className="px-4 pb-4 flex gap-2">
                <a
                  href={`/activities?tag=${encodeURIComponent(selectedLocationTag.name)}`}
                  className="flex-1 text-center px-3 py-1.5 rounded-lg text-sm text-orange-500 border border-orange-200 dark:border-orange-800 hover:bg-orange-50 dark:hover:bg-orange-900/20"
                  onClick={() => setSelectedLocationTag(null)}
                >
                  Aktivitäten anzeigen
                </a>
                <button
                  onClick={() => setSelectedLocationTag(null)}
                  className="px-3 py-1.5 rounded-lg text-sm text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800 cursor-pointer"
                >
                  Schließen
                </button>
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
  return [
    'match', ['get', 'sport_type'],
    'cycling',                 filters.colorConfig.colors['cycling']                 ?? '#F97316',
    'e_biking',                filters.colorConfig.colors['e_biking']                ?? '#F97316',
    'running',                 filters.colorConfig.colors['running']                 ?? '#EF4444',
    'hiking',                  filters.colorConfig.colors['hiking']                  ?? '#22C55E',
    'walking',                 filters.colorConfig.colors['walking']                 ?? '#22C55E',
    'swimming',                filters.colorConfig.colors['swimming']                ?? '#3B82F6',
    'paddling',                filters.colorConfig.colors['paddling']                ?? '#3B82F6',
    'stand_up_paddleboarding', filters.colorConfig.colors['stand_up_paddleboarding'] ?? '#3B82F6',
    'skiing',                  filters.colorConfig.colors['skiing']                  ?? '#06B6D4',
    'rock_climbing',           filters.colorConfig.colors['rock_climbing']           ?? '#A855F7',
    'strength',                filters.colorConfig.colors['strength']                ?? '#6B7280',
    'generic',                 filters.colorConfig.colors['generic']                 ?? '#6B7280',
    filters.colorConfig.uniformColor, // Fallback
  ] as maplibregl.ExpressionSpecification
}

function addOverlayLayers(
  map: maplibregl.Map,
  filters: MapFilterState,
  viewMode: ViewMode,
  teleportThresholdM: number,
) {
  // Linien-Layer fuer beide Modi
  map.addSource('heatmap', {
    type: 'vector',
    tiles: [buildTileUrl('heatmap_lines', filters, teleportThresholdM)],
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
    map.addLayer({
      id: 'heatmap-glow-outer',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': '#3D0000',
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
        'line-color': '#8B0000',
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
        'line-color': '#F97316',
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
        'line-color': '#FFAA00',
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
        'line-color': '#FFFFA0',
        'line-width': 1,
        'line-opacity': 0.12,
        'line-blur': 0,
      },
    })
  }
}
