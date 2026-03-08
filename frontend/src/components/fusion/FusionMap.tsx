import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { buildRasterStyle } from '@/lib/mapStyles'

export interface TrackPoint {
  lat: number
  lng: number
}

export interface TrackInfo {
  activityId: string
  label: string
  color: string
  stream: TrackPoint[]
}

interface FusionMapProps {
  tracks: TrackInfo[]
}

export default function FusionMap({ tracks }: FusionMapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: buildRasterStyle('osm'),
      center: [14, 47],
      zoom: 4,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')
    mapRef.current = map

    map.on('load', () => {
      const allPoints = tracks.flatMap(t => t.stream)
      if (allPoints.length > 0) {
        const bounds = new maplibregl.LngLatBounds()
        allPoints.forEach(p => bounds.extend([p.lng, p.lat]))
        map.fitBounds(bounds, { padding: 40, maxZoom: 16 })
      }

      tracks.forEach(track => {
        if (track.stream.length === 0) return
        const sourceId = `track-${track.activityId}`
        const layerId = `track-line-${track.activityId}`

        const geojson: GeoJSON.Feature<GeoJSON.LineString> = {
          type: 'Feature',
          properties: { label: track.label },
          geometry: {
            type: 'LineString',
            coordinates: track.stream.map(p => [p.lng, p.lat]),
          },
        }

        map.addSource(sourceId, { type: 'geojson', data: geojson })
        map.addLayer({
          id: layerId,
          type: 'line',
          source: sourceId,
          paint: {
            'line-color': track.color,
            'line-width': 3,
            'line-opacity': 0.85,
          },
          layout: { 'line-cap': 'round', 'line-join': 'round' },
        })
      })
    })

    return () => {
      map.remove()
      mapRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Update track data when tracks change
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.loaded()) return

    tracks.forEach(track => {
      const sourceId = `track-${track.activityId}`
      const source = map.getSource(sourceId) as maplibregl.GeoJSONSource | undefined
      if (source && track.stream.length > 0) {
        source.setData({
          type: 'Feature',
          properties: {},
          geometry: {
            type: 'LineString',
            coordinates: track.stream.map(p => [p.lng, p.lat]),
          },
        })
      }
    })

    // Re-fit bounds if we have points
    const allPoints = tracks.flatMap(t => t.stream)
    if (allPoints.length > 0) {
      const bounds = new maplibregl.LngLatBounds()
      allPoints.forEach(p => bounds.extend([p.lng, p.lat]))
      map.fitBounds(bounds, { padding: 40, maxZoom: 16, duration: 800 })
    }
  }, [tracks])

  return (
    <div className="relative w-full h-full min-h-64 rounded-xl overflow-hidden border border-neutral-200 dark:border-neutral-800">
      <div ref={containerRef} className="w-full h-full" />
      {tracks.length > 0 && tracks.some(t => t.stream.length > 0) && (
        <div className="absolute bottom-3 left-3 bg-white/90 dark:bg-neutral-900/90 backdrop-blur-sm rounded-lg px-3 py-2 space-y-1 pointer-events-none">
          {tracks.map(t => (
            <div key={t.activityId} className="flex items-center gap-2">
              <div
                className="rounded-full flex-shrink-0"
                style={{ backgroundColor: t.color, width: '16px', height: '3px' }}
              />
              <span className="text-xs text-neutral-700 dark:text-neutral-300">{t.label}</span>
            </div>
          ))}
        </div>
      )}
      {tracks.every(t => t.stream.length === 0) && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <span className="text-xs text-neutral-400 bg-white/80 dark:bg-neutral-900/80 px-3 py-1.5 rounded-lg">
            GPS-Daten werden geladen...
          </span>
        </div>
      )}
    </div>
  )
}
