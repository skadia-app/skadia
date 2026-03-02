import { useRef, useEffect, useState, useCallback, type ChangeEvent, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'
import { getAllSportKeys } from '@/lib/sportOptions'
import { getHeatmapSettings } from '@/api/settings'

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

export interface MapFilterState {
  sportTypes: Set<string>
  dateFrom: string
  dateTo: string
  opacity: number
}

const defaultFilters: MapFilterState = {
  sportTypes: new Set(ALL_SPORT_KEYS),
  dateFrom: '',
  dateTo: '',
  opacity: 70,
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
  const [filters, setFilters] = useState<MapFilterState>(defaultFilters)
  const [viewMode, setViewMode] = useState<ViewMode>('tracks')
  // Merkt sich, ob wir den Stil automatisch auf Dark gesetzt haben,
  // damit wir ihn beim Zurueckwechseln auch wieder zuruecksetzen koennen.
  const autoSwitchedToDark = useRef(false)
  const styleBeforeAutoSwitch = useRef<BaseStyle>('osm')

  // Heatmap-Settings laden (Teleportfilter)
  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
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
  // Läuft auch beim ersten Render (baseStyle='osm'), was einen zweiten setStyle-
  // Aufruf verursacht. Das ist kein Problem: removeOverlayLayers im style.load-
  // Handler räumt vorher auf, und die Kameraposition ist noch unverändert.
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

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

  const handleFilterChange = useCallback((next: MapFilterState) => {
    setFilters(next)
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

        {/* Filters panel */}
        <MapFilters filters={filters} onChange={handleFilterChange} />
      </div>
    </div>
  )
}

function removeOverlayLayers(map: maplibregl.Map) {
  if (map.getLayer('heatmap-lines')) map.removeLayer('heatmap-lines')
  if (map.getSource('heatmap')) map.removeSource('heatmap')
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
    // Tracks: nutzerdefinierte Opacity, orange
    map.addLayer({
      id: 'heatmap-lines',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': '#F97316',
        'line-width': 2,
        'line-opacity': Math.pow(filters.opacity / 100, 2),
      },
    })
  } else {
    // Heatmap: niedrige feste Opacity — Alpha-Compositing erzeugt natuerliche
    // Helligkeitsunterschiede: viele ueberlagerte Tracks leuchten heller auf.
    map.addLayer({
      id: 'heatmap-lines',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': '#F97316',
        'line-width': 2,
        'line-opacity': 0.25,
      },
    })
  }
}
