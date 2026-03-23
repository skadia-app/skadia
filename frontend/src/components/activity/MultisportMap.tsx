import { useRef, useEffect, useImperativeHandle, forwardRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { apiFetch } from '@/lib/auth'
import { sportChartColor } from '@/lib/sportColors'

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

export interface MultisportMapHandle {
  setHoverPoint(point: { lat: number; lng: number } | null): void
}

interface Props {
  activityId: string
  className?: string
  hoveredActivityId?: string | null
}

export const MultisportMap = forwardRef<MultisportMapHandle, Props>(
function MultisportMap({ activityId, className, hoveredActivityId }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const featureIdsRef = useRef<{ sourceId: string; activityId: string }[]>([])
  const hoverMarkerRef = useRef<maplibregl.Marker | null>(null)

  useImperativeHandle(ref, () => ({
    setHoverPoint(point) {
      hoverMarkerRef.current?.remove()
      hoverMarkerRef.current = null
      if (!point) return
      const map = mapRef.current
      if (!map) return
      const el = document.createElement('div')
      el.style.width = '10px'
      el.style.height = '10px'
      el.style.borderRadius = '50%'
      el.style.backgroundColor = 'white'
      el.style.border = '2px solid #f97316'
      el.style.boxShadow = '0 0 4px #f97316'
      hoverMarkerRef.current = new maplibregl.Marker({ element: el })
        .setLngLat([point.lng, point.lat])
        .addTo(map)
    },
  }), [])

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

    mapRef.current = map

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
      featureIdsRef.current = []
      data.features.forEach((feature, i) => {
        const sourceId = `ms-track-${i}`
        const color = sportChartColor(feature.properties.sport_type)

        map.addSource(sourceId, {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: feature.geometry as GeoJSON.Geometry,
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

        featureIdsRef.current.push({ sourceId, activityId: feature.properties.activity_id })
      })

      if (!bounds.isEmpty()) {
        map.fitBounds(bounds, { padding: 48, animate: false, maxZoom: 15 })
      }
    })

    return () => {
      hoverMarkerRef.current?.remove()
      hoverMarkerRef.current = null
      mapRef.current = null
      featureIdsRef.current = []
      map.remove()
    }
  }, [activityId])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    featureIdsRef.current.forEach(({ sourceId, activityId }) => {
      const isHovered = hoveredActivityId == null || activityId === hoveredActivityId
      const lineOpacity = isHovered ? 1 : 0.15
      const glowOpacity = isHovered ? 0.22 : 0.05
      if (map.getLayer(`${sourceId}-line`)) {
        map.setPaintProperty(`${sourceId}-line`, 'line-opacity', lineOpacity)
      }
      if (map.getLayer(`${sourceId}-glow`)) {
        map.setPaintProperty(`${sourceId}-glow`, 'line-opacity', glowOpacity)
      }
    })
  }, [hoveredActivityId])

  return (
    <div ref={containerRef} className={className ?? 'h-80 w-full rounded-lg overflow-hidden'} />
  )
})
