import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { Segment } from '@/api/segments'

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

interface SegmentMinimapProps {
  segment: Segment
}

export function SegmentMinimap({ segment }: SegmentMinimapProps) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)

  useEffect(() => {
    if (!mapContainer.current || !segment.track_geojson) return

    let geometry: GeoJSON.Geometry
    try {
      geometry = JSON.parse(segment.track_geojson) as GeoJSON.Geometry
    } catch { return }

    if (!geometry || !('coordinates' in geometry) || !geometry.coordinates?.length) return

    const coords = geometry.type === 'LineString'
      ? geometry.coordinates as [number, number][]
      : (geometry as GeoJSON.MultiLineString).coordinates.flat() as [number, number][]

    if (coords.length === 0) return

    // Berechne Bounds VOR der Map-Erstellung, damit kein Zoom-Sprung passiert
    const bounds = coords.reduce(
      (b, c) => b.extend(c),
      new maplibregl.LngLatBounds(coords[0], coords[0])
    )

    mapRef.current = new maplibregl.Map({
      container: mapContainer.current,
      style: MAP_STYLE,
      bounds,
      fitBoundsOptions: { padding: 40 },
      interactive: true,
    })

    mapRef.current.on('load', () => {
      const map = mapRef.current
      if (!map) return

      map.addSource('segment', {
        type: 'geojson',
        data: { type: 'Feature', properties: {}, geometry },
      })
      map.addLayer({
        id: 'segment-line',
        type: 'line',
        source: 'segment',
        paint: { 'line-color': '#FF6B35', 'line-width': 3 },
        layout: { 'line-cap': 'round', 'line-join': 'round' },
      })
    })

    return () => {
      mapRef.current?.remove()
      mapRef.current = null
    }
  }, [segment.track_geojson])

  if (!segment.track_geojson) {
    return (
      <div
        className="border border-neutral-200 dark:border-neutral-800 rounded-lg overflow-hidden bg-neutral-100 dark:bg-neutral-800 flex items-center justify-center"
        style={{ height: '350px' }}
      >
        <span className="text-neutral-400 text-sm">Keine Kartendaten verfügbar</span>
      </div>
    )
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-lg overflow-hidden">
      <div ref={mapContainer} style={{ height: '350px', width: '100%' }} />
    </div>
  )
}
