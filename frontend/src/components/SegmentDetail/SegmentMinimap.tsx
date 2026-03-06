import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'

interface SegmentMinimapProps {
  segment: any
}

const MAP_STYLE: maplibregl.StyleSpecification = {
  version: 8,
  sources: {
    basemap: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      maxzoom: 19,
    },
  },
  layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap' }],
}

export function SegmentMinimap({ segment }: SegmentMinimapProps) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)

  useEffect(() => {
    if (!mapContainer.current || !segment?.geojson) return

    const geojson = typeof segment.geojson === 'string'
      ? JSON.parse(segment.geojson)
      : segment.geojson

    if (!geojson?.geometry?.coordinates?.length) return

    mapRef.current = new maplibregl.Map({
      container: mapContainer.current,
      style: MAP_STYLE,
      center: [0, 0],
      zoom: 12,
      interactive: false,
    })

    mapRef.current.on('load', () => {
      const map = mapRef.current
      if (!map) return

      map.addSource('segment', { type: 'geojson', data: geojson })
      map.addLayer({
        id: 'segment-line',
        type: 'line',
        source: 'segment',
        paint: { 'line-color': '#FF6B35', 'line-width': 3 },
      })

      const coords: [number, number][] = geojson.geometry.coordinates
      const bounds = coords.reduce(
        (b, c) => b.extend(c),
        new maplibregl.LngLatBounds(coords[0], coords[0])
      )
      map.fitBounds(bounds, { padding: 40 })
    })

    return () => {
      mapRef.current?.remove()
      mapRef.current = null
    }
  }, [segment])

  if (!segment?.geojson) {
    return (
      <div
        className="border rounded-lg overflow-hidden bg-neutral-100 dark:bg-neutral-800 flex items-center justify-center"
        style={{ height: '350px' }}
      >
        <span className="text-neutral-400 text-sm">Keine Kartendaten verfügbar</span>
      </div>
    )
  }

  return (
    <div className="border rounded-lg overflow-hidden">
      <div ref={mapContainer} style={{ height: '350px', width: '100%' }} />
    </div>
  )
}
