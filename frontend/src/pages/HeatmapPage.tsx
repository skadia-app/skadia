import { useRef, useEffect, useState, useCallback } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { getHeatmapTileUrl } from '@/api/heatmap'
import HeatmapFilters from '@/components/HeatmapFilters'

type BaseStyle = 'osm' | 'topo' | 'cyclosm'

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

export interface HeatmapFilterState {
  sportTypes: Set<string>
  dateFrom: string
  dateTo: string
  opacity: number
}

const defaultFilters: HeatmapFilterState = {
  sportTypes: new Set(),
  dateFrom: '',
  dateTo: '',
  opacity: 70,
}

export default function HeatmapPage() {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const [baseStyle, setBaseStyle] = useState<BaseStyle>('osm')
  const [filters, setFilters] = useState<HeatmapFilterState>(defaultFilters)

  // Initialize map
  useEffect(() => {
    if (!mapContainerRef.current) return

    const map = new maplibregl.Map({
      container: mapContainerRef.current,
      style: buildMapStyle(baseStyle),
      center: [10.45, 51.16], // Germany center
      zoom: 6,
      attributionControl: true,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    map.on('load', () => {
      // Heatmap vector tile source
      map.addSource('heatmap', {
        type: 'vector',
        tiles: [getHeatmapTileUrl()],
        minzoom: 0,
        maxzoom: 14,
      })

      // Heatmap line layer
      map.addLayer({
        id: 'heatmap-lines',
        type: 'line',
        source: 'heatmap',
        'source-layer': 'heatmap_lines',
        paint: {
          'line-color': '#F97316',
          'line-width': 2,
          'line-opacity': filters.opacity / 100,
        },
      })
    })

    mapRef.current = map

    return () => {
      map.remove()
      mapRef.current = null
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

      // Re-add heatmap source + layer after style change
      map.addSource('heatmap', {
        type: 'vector',
        tiles: [getHeatmapTileUrl()],
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
          'line-opacity': filters.opacity / 100,
        },
      })
    })
  }, [baseStyle]) // eslint-disable-line react-hooks/exhaustive-deps

  // Update opacity when filter changes
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const apply = () => {
      if (map.getLayer('heatmap-lines')) {
        map.setPaintProperty('heatmap-lines', 'line-opacity', filters.opacity / 100)
      }
    }

    if (map.isStyleLoaded()) apply()
    else map.once('style.load', apply)
  }, [filters.opacity])

  const handleFilterChange = useCallback((next: HeatmapFilterState) => {
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
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                baseStyle === key
                  ? 'bg-orange-500 text-white'
                  : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
              }`}
            >
              {BASE_STYLES[key].label}
            </button>
          ))}
        </div>

        {/* Filters panel */}
        <HeatmapFilters filters={filters} onChange={handleFilterChange} />
      </div>
    </div>
  )
}
