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
        paint: {
          'line-color': segment.course_kind === 'checkpoint_sequence' ? '#0F766E' : '#FF6B35',
          'line-width': segment.course_kind === 'checkpoint_sequence' ? 2 : 3,
          ...(segment.course_kind === 'checkpoint_sequence' ? { 'line-dasharray': [1.5, 2.5] } : {}),
        },
        layout: { 'line-cap': 'round', 'line-join': 'round' },
      })
      if (segment.course_kind === 'checkpoint_sequence') {
        const controlFeatures = checkpointControlFeatures(segment)
        if (controlFeatures.length > 0) {
          map.addSource('checkpoint-controls', {
            type: 'geojson',
            data: { type: 'FeatureCollection', features: controlFeatures },
          })
          map.addLayer({
            id: 'checkpoint-control-areas',
            type: 'fill',
            source: 'checkpoint-controls',
            filter: ['==', ['get', 'kind'], 'area'],
            paint: { 'fill-color': '#0F766E', 'fill-opacity': 0.16 },
          })
          map.addLayer({
            id: 'checkpoint-control-area-outlines',
            type: 'line',
            source: 'checkpoint-controls',
            filter: ['==', ['get', 'kind'], 'area'],
            paint: { 'line-color': '#0F766E', 'line-width': 1.5, 'line-opacity': 0.8 },
          })
          map.addLayer({
            id: 'checkpoint-control-points',
            type: 'circle',
            source: 'checkpoint-controls',
            filter: ['==', ['get', 'kind'], 'point'],
            paint: {
              'circle-radius': 9,
              'circle-color': '#ffffff',
              'circle-stroke-width': 2,
              'circle-stroke-color': '#0F766E',
            },
          })
          map.addLayer({
            id: 'checkpoint-control-labels',
            type: 'symbol',
            source: 'checkpoint-controls',
            filter: ['==', ['get', 'kind'], 'point'],
            layout: { 'text-field': ['get', 'label'], 'text-size': 10, 'text-allow-overlap': true },
            paint: { 'text-color': '#0f172a' },
          })
        }
      }
    })

    return () => {
      mapRef.current?.remove()
      mapRef.current = null
    }
  }, [segment])

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

function checkpointControlFeatures(segment: Segment): GeoJSON.Feature[] {
  const controls = segment.controls ?? []
  return controls.flatMap((control, index): GeoJSON.Feature[] => {
    const geometry = control.geometry as GeoJSON.Geometry | undefined
    if (!geometry) return []
    const label = control.role === 'start'
      ? 'Start'
      : control.role === 'finish'
        ? 'Ziel'
        : String(index + 1)
    const point = control.geometry_shape === 'point' || control.geometry_shape === 'circle'
      ? geometry
      : centroidFeatureGeometry(geometry)
    const features: GeoJSON.Feature[] = []
    if (control.primitive === 'area' && geometry.type !== 'Point') {
      features.push({ type: 'Feature', geometry, properties: { kind: 'area' } })
    }
    if (point) {
      features.push({ type: 'Feature', geometry: point, properties: { kind: 'point', label } })
    }
    return features
  })
}

function centroidFeatureGeometry(geometry: GeoJSON.Geometry): GeoJSON.Point | null {
  const coords: [number, number][] = []
  const collect = (value: unknown): void => {
    if (!Array.isArray(value)) return
    if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') {
      coords.push([value[0], value[1]])
      return
    }
    value.forEach(collect)
  }
  collect((geometry as { coordinates?: unknown }).coordinates)
  if (coords.length === 0) return null
  const [lngSum, latSum] = coords.reduce(([lng, lat], coord) => [lng + coord[0], lat + coord[1]], [0, 0])
  return { type: 'Point', coordinates: [lngSum / coords.length, latSum / coords.length] }
}
