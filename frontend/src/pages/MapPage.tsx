import { useRef, useEffect, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import MapFilters from '@/components/MapFilters'

type BaseStyle = 'osm' | 'topo' | 'cyclosm'
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
    tiles: ['https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png'.replace('{s}', 'a')],
    attribution: '&copy; <a href="https://www.cyclosm.org">CyclOSM</a>',
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
      },
    },
    layers: [
      {
        id: 'basemap-tiles',
        type: 'raster',
        source: 'basemap',
        minzoom: 0,
        maxzoom: 19,
      },
    ],
  }
}

export const SPORT_OPTIONS = [
  { value: 'cycling', label: 'Radfahren' },
  { value: 'running', label: 'Laufen' },
  { value: 'hiking', label: 'Wandern' },
  { value: 'walking', label: 'Gehen' },
  { value: 'swimming', label: 'Schwimmen' },
  { value: 'e_biking', label: 'E-Bike' },
  { value: 'skiing', label: 'Ski' },
  { value: 'paddling', label: 'Paddeln' },
]

export interface MapFilterState {
  sportTypes: Set<string>
  dateFrom: string
  dateTo: string
  opacity: number
}

const defaultFilters: MapFilterState = {
  sportTypes: new Set(SPORT_OPTIONS.map(s => s.value)),
  dateFrom: '',
  dateTo: '',
  opacity: 70,
}

function buildTileUrl(source: 'heatmap_lines' | 'heatmap_points', filters: MapFilterState): string {
  const MARTIN_BASE = import.meta.env.VITE_MARTIN_URL ?? 'http://localhost:3000'
  const base = `${MARTIN_BASE}/${source}/{z}/{x}/{y}.pbf`
  const params = new URLSearchParams()

  const allSelected = filters.sportTypes.size === SPORT_OPTIONS.length
  if (!allSelected && filters.sportTypes.size > 0) {
    params.set('sport_type', Array.from(filters.sportTypes).join(','))
  }
  if (filters.dateFrom) params.set('activity_date_gte', filters.dateFrom)
  if (filters.dateTo) params.set('activity_date_lte', filters.dateTo)

  const qs = params.toString()
  return qs ? `${base}?${qs}` : base
}

export default function MapPage() {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const [baseStyle, setBaseStyle] = useState<BaseStyle>('osm')
  const [filters, setFilters] = useState<MapFilterState>(defaultFilters)
  const [viewMode, setViewMode] = useState<ViewMode>('tracks')
  const styleLoadedRef = useRef(false)

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: buildMapStyle(baseStyle),
      center: [10.45, 51.16],
      zoom: 6,
      attributionControl: { compact: false },
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    map.on('load', () => {
      styleLoadedRef.current = true
      addOverlayLayers(map, filters, viewMode)
    })

    mapRef.current = map

    return () => {
      map.remove()
      mapRef.current = null
      styleLoadedRef.current = false
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Update base style
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const center = map.getCenter()
    const zoom = map.getZoom()

    map.setStyle(buildMapStyle(baseStyle))

    map.once('style.load', () => {
      map.setCenter(center)
      map.setZoom(zoom)
      styleLoadedRef.current = true
      addOverlayLayers(map, filters, viewMode)
    })
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // Update overlay layers when filters or view mode change
  useEffect(() => {
    const map = mapRef.current
    if (!map || !styleLoadedRef.current) return

    const apply = () => {
      removeOverlayLayers(map)
      addOverlayLayers(map, filters, viewMode)
    }

    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  }, [filters, viewMode])

  const handleFilterChange = useCallback((next: MapFilterState) => {
    setFilters(next)
  }, [])

  return (
    <div className="fixed inset-0 z-40 flex flex-col" style={{ top: '3.5rem' }}>
      <div className="relative flex-1 min-h-0">
        <div ref={mapContainerRef} className="w-full h-full" />

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
            onClick={() => setViewMode('tracks')}
            className={`px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
              viewMode === 'tracks'
                ? 'bg-orange-500 text-white'
                : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
            }`}
          >
            Tracks
          </button>
          <button
            onClick={() => setViewMode('heatmap')}
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
  if (map.getLayer('density-heat')) map.removeLayer('density-heat')
  if (map.getSource('heatmap-points')) map.removeSource('heatmap-points')
}

function addOverlayLayers(map: maplibregl.Map, filters: MapFilterState, viewMode: ViewMode) {
  if (viewMode === 'tracks' || viewMode === 'heatmap') {
    // Lines layer (tracks mode)
    map.addSource('heatmap', {
      type: 'vector',
      tiles: [buildTileUrl('heatmap_lines', filters)],
      minzoom: 0,
      maxzoom: 14,
    })
    map.addLayer({
      id: 'heatmap-lines',
      type: 'line',
      source: 'heatmap',
      'source-layer': 'heatmap_lines',
      paint: {
        'line-color': '#F97316',
        'line-width': 2,
        'line-opacity': viewMode === 'tracks'
          ? Math.pow(filters.opacity / 100, 2)
          : ['interpolate', ['linear'], ['zoom'], 12, 0, 14, Math.pow(filters.opacity / 100, 2)],
      },
    })
  }

  if (viewMode === 'heatmap') {
    // Points layer (heatmap mode)
    map.addSource('heatmap-points', {
      type: 'vector',
      tiles: [buildTileUrl('heatmap_points', filters)],
      minzoom: 0,
      maxzoom: 14,
    })
    map.addLayer(
      {
        id: 'density-heat',
        type: 'heatmap',
        source: 'heatmap-points',
        'source-layer': 'heatmap_points',
        maxzoom: 14,
        paint: {
          'heatmap-weight': 1,
          'heatmap-intensity': ['interpolate', ['linear'], ['zoom'], 0, 1, 14, 3],
          'heatmap-color': [
            'interpolate', ['linear'], ['heatmap-density'],
            0, 'rgba(0, 0, 255, 0)',
            0.1, 'rgba(65, 105, 225, 0.4)',
            0.3, 'rgb(0, 191, 255)',
            0.5, 'rgb(0, 255, 127)',
            0.7, 'rgb(255, 255, 0)',
            0.9, 'rgb(255, 140, 0)',
            1, 'rgb(255, 0, 0)',
          ],
          'heatmap-radius': ['interpolate', ['linear'], ['zoom'], 0, 2, 6, 8, 10, 18, 14, 30],
          'heatmap-opacity': ['interpolate', ['linear'], ['zoom'], 12, 1, 14, 0],
        },
      },
      'heatmap-lines',
    )
  }
}
