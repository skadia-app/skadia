import { useRef, useEffect } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { apiFetch } from '@/lib/auth'

const SPORT_COLORS: Record<string, string> = {
  cycling:               '#f97316',
  running:               '#22c55e',
  swimming:              '#3b82f6',
  open_water_swimming:   '#06b6d4',
  hiking:                '#84cc16',
  walking:               '#eab308',
  trail_running:         '#10b981',
  mountain_biking:       '#a3e635',
  transition:            '#a855f7',
  rock_climbing:         '#ef4444',
  skiing:                '#60a5fa',
  cross_country_skiing:  '#93c5fd',
  paddling:              '#0ea5e9',
  stand_up_paddleboarding: '#38bdf8',
}

function colorForSport(sportType: string): string {
  return SPORT_COLORS[sportType] ?? '#6b7280'
}

interface GeoJSONFeature {
  type: 'Feature'
  geometry: {
    type: string
    coordinates: number[][] | number[][][]
  }
  properties: {
    activity_id: string
    sport_type: string
    title: string
    child_index: number
  }
}

interface FeatureCollection {
  type: 'FeatureCollection'
  features: GeoJSONFeature[]
}

interface Props {
  activityId: string
  className?: string
}

export function MultisportMap({ activityId, className }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    const map = new maplibregl.Map({
      container: el,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '© OpenStreetMap contributors',
          },
        },
        layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm', minzoom: 0, maxzoom: 19 }],
      },
      zoom: 10,
      center: [10, 48],
      attributionControl: false,
    })

    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
    map.addControl(new maplibregl.AttributionControl({ compact: true }), 'bottom-right')

    map.on('load', async () => {
      let data: FeatureCollection
      try {
        const res = await apiFetch(`/api/v1/activities/${activityId}/combined-track`)
        if (!res.ok) return
        data = await res.json() as FeatureCollection
      } catch {
        return
      }

      if (!data.features?.length) return

      // Compute bounds across all features.
      const bounds = new maplibregl.LngLatBounds()
      data.features.forEach(f => {
        const coords = f.geometry.coordinates
        // LineString: coords is number[][]
        // MultiLineString: coords is number[][][]
        if (f.geometry.type === 'LineString') {
          ;(coords as number[][]).forEach(([lng, lat]) => bounds.extend([lng, lat]))
        } else if (f.geometry.type === 'MultiLineString') {
          ;(coords as number[][][]).forEach(line =>
            line.forEach(([lng, lat]) => bounds.extend([lng, lat]))
          )
        }
      })

      // Add one source+layer pair per child track so each gets its own color.
      data.features.forEach((feature, i) => {
        const sourceId = `ms-track-${i}`
        const color = colorForSport(feature.properties.sport_type)

        map.addSource(sourceId, {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: feature.geometry,
            properties: feature.properties,
          },
        })

        // Soft glow layer underneath.
        map.addLayer({
          id: `${sourceId}-glow`,
          type: 'line',
          source: sourceId,
          layout: { 'line-cap': 'round', 'line-join': 'round' },
          paint: { 'line-color': color, 'line-width': 8, 'line-opacity': 0.22 },
        })

        // Main track line.
        map.addLayer({
          id: `${sourceId}-line`,
          type: 'line',
          source: sourceId,
          layout: { 'line-cap': 'round', 'line-join': 'round' },
          paint: { 'line-color': color, 'line-width': 3 },
        })
      })

      if (!bounds.isEmpty()) {
        map.fitBounds(bounds, { padding: 48, animate: false, maxZoom: 15 })
      }
    })

    return () => { map.remove() }
  }, [activityId])

  return (
    <div ref={containerRef} className={className ?? 'h-80 w-full rounded-lg overflow-hidden'} />
  )
}
