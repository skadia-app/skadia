import { useRef, useEffect } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'

type EditorMapMode = 'trim' | 'split'

interface EditorMapProps {
  trackGeoJSON: GeoJSON.LineString
  outlierPoints?: { lat: number; lng: number; time: string }[]
  trimStart?: number  // Index in track coordinates
  trimEnd?: number
  /** Index of split point in track coordinates (split mode) */
  splitIndex?: number
  /** Current mode controls which visualization is shown */
  mode?: EditorMapMode
  onTrackClick?: (index: number, time: string) => void
  className?: string
}

export function EditorMap({
  trackGeoJSON, outlierPoints, trimStart, trimEnd,
  splitIndex, mode = 'trim',
  onTrackClick, className,
}: EditorMapProps) {
  const mapRef = useRef<maplibregl.Map | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  // Keep stable refs for callbacks to avoid re-creating the map
  const onTrackClickRef = useRef(onTrackClick)
  onTrackClickRef.current = onTrackClick
  const trackRef = useRef(trackGeoJSON)
  trackRef.current = trackGeoJSON

  // Map einmalig erstellen
  useEffect(() => {
    if (!containerRef.current) return
    if (trackGeoJSON.coordinates.length < 2) return

    const bounds = getBounds(trackGeoJSON.coordinates)

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: 'https://tiles.openfreemap.org/styles/liberty',
      bounds: bounds as maplibregl.LngLatBoundsLike,
      fitBoundsOptions: { padding: 40 },
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      // Vollstandiger Track (grau, dunn)
      map.addSource('track-full', {
        type: 'geojson',
        data: { type: 'Feature', geometry: trackRef.current, properties: {} },
      })
      map.addLayer({
        id: 'track-full-line',
        type: 'line',
        source: 'track-full',
        paint: { 'line-color': '#6b7280', 'line-width': 2, 'line-opacity': 0.4 },
      })

      // Aktiver Bereich (nach Trim, blau, dick)
      const activeCoords = trackRef.current.coordinates.slice(
        trimStart ?? 0,
        (trimEnd ?? trackRef.current.coordinates.length - 1) + 1,
      )
      map.addSource('track-active', {
        type: 'geojson',
        data: {
          type: 'Feature',
          geometry: { type: 'LineString', coordinates: activeCoords },
          properties: {},
        },
      })
      map.addLayer({
        id: 'track-active-line',
        type: 'line',
        source: 'track-active',
        paint: { 'line-color': '#3b82f6', 'line-width': 3 },
      })

      // Start/End-Marker Quellen
      map.addSource('trim-markers', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'trim-marker-start',
        type: 'circle',
        source: 'trim-markers',
        filter: ['==', ['get', 'role'], 'start'],
        paint: {
          'circle-radius': 7,
          'circle-color': '#22c55e',
          'circle-stroke-width': 2,
          'circle-stroke-color': '#fff',
        },
      })
      map.addLayer({
        id: 'trim-marker-end',
        type: 'circle',
        source: 'trim-markers',
        filter: ['==', ['get', 'role'], 'end'],
        paint: {
          'circle-radius': 7,
          'circle-color': '#ef4444',
          'circle-stroke-width': 2,
          'circle-stroke-color': '#fff',
        },
      })

      // Split-point marker source (yellow/orange diamond)
      map.addSource('split-marker', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'split-marker-point',
        type: 'circle',
        source: 'split-marker',
        paint: {
          'circle-radius': 9,
          'circle-color': '#f59e0b',
          'circle-stroke-width': 2.5,
          'circle-stroke-color': '#fff',
        },
      })
      // Split label
      map.addLayer({
        id: 'split-marker-label',
        type: 'symbol',
        source: 'split-marker',
        layout: {
          'text-field': '\u2702',
          'text-size': 14,
          'text-offset': [0, -1.4],
          'text-allow-overlap': true,
        },
      })

      // GPS-Outlier Source
      map.addSource('outliers', {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      })
      map.addLayer({
        id: 'outlier-points',
        type: 'circle',
        source: 'outliers',
        paint: {
          'circle-radius': 6,
          'circle-color': '#f97316',
          'circle-stroke-width': 2,
          'circle-stroke-color': '#fff',
        },
      })

      // Klick auf Track-Linie -> nachsten Punkt finden und Callback auslosen
      map.on('click', 'track-full-line', (e) => {
        if (!e.lngLat) return
        const clickedCoord = [e.lngLat.lng, e.lngLat.lat]
        const nearest = findNearestIndex(trackRef.current.coordinates, clickedCoord)
        onTrackClickRef.current?.(nearest.index, '')
      })
      // Auch Klick auf aktive Linie abfangen
      map.on('click', 'track-active-line', (e) => {
        if (!e.lngLat) return
        const clickedCoord = [e.lngLat.lng, e.lngLat.lat]
        const nearest = findNearestIndex(trackRef.current.coordinates, clickedCoord)
        onTrackClickRef.current?.(nearest.index, '')
      })

      map.getCanvas().style.cursor = 'crosshair'
    })

    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
    }
    // Only recreate map if the track itself changes (different activity)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trackGeoJSON])

  // Trim-Bereich aktualisieren (OHNE Map-Neuerstellung)
  useEffect(() => {
    const map = mapRef.current
    if (!map?.isStyleLoaded()) return

    const coords = trackGeoJSON.coordinates

    if (mode === 'trim') {
      // Show trimmed region: full track gray, active region blue
      const start = trimStart ?? 0
      const end = (trimEnd ?? coords.length - 1) + 1
      const activeCoords = coords.slice(start, end)

      const activeSource = map.getSource('track-active') as maplibregl.GeoJSONSource | undefined
      activeSource?.setData({
        type: 'Feature',
        geometry: { type: 'LineString', coordinates: activeCoords.length >= 2 ? activeCoords : coords },
        properties: {},
      })

      // Trim-Marker aktualisieren
      const markerSource = map.getSource('trim-markers') as maplibregl.GeoJSONSource | undefined
      const features: GeoJSON.Feature[] = []
      if (coords[start]) {
        features.push({
          type: 'Feature',
          geometry: { type: 'Point', coordinates: coords[start] },
          properties: { role: 'start' },
        })
      }
      const endIdx = end - 1
      if (coords[endIdx]) {
        features.push({
          type: 'Feature',
          geometry: { type: 'Point', coordinates: coords[endIdx] },
          properties: { role: 'end' },
        })
      }
      markerSource?.setData({ type: 'FeatureCollection', features })

      // Hide split marker
      const splitSource = map.getSource('split-marker') as maplibregl.GeoJSONSource | undefined
      splitSource?.setData({ type: 'FeatureCollection', features: [] })

    } else {
      // Split mode: show full track as active, show split marker
      const activeSource = map.getSource('track-active') as maplibregl.GeoJSONSource | undefined
      activeSource?.setData({
        type: 'Feature',
        geometry: { type: 'LineString', coordinates: coords },
        properties: {},
      })

      // Hide trim markers
      const markerSource = map.getSource('trim-markers') as maplibregl.GeoJSONSource | undefined
      markerSource?.setData({ type: 'FeatureCollection', features: [] })

      // Show split marker
      const splitSource = map.getSource('split-marker') as maplibregl.GeoJSONSource | undefined
      const idx = splitIndex ?? 0
      if (idx > 0 && idx < coords.length && coords[idx]) {
        splitSource?.setData({
          type: 'FeatureCollection',
          features: [{
            type: 'Feature',
            geometry: { type: 'Point', coordinates: coords[idx] },
            properties: {},
          }],
        })
      } else {
        splitSource?.setData({ type: 'FeatureCollection', features: [] })
      }
    }
  }, [trimStart, trimEnd, splitIndex, trackGeoJSON, mode])

  // Outlier-Punkte aktualisieren
  useEffect(() => {
    const map = mapRef.current
    if (!map?.isStyleLoaded()) return

    const source = map.getSource('outliers') as maplibregl.GeoJSONSource | undefined
    source?.setData({
      type: 'FeatureCollection',
      features: (outlierPoints ?? []).map(p => ({
        type: 'Feature' as const,
        geometry: { type: 'Point' as const, coordinates: [p.lng, p.lat] },
        properties: { time: p.time },
      })),
    })
  }, [outlierPoints])

  return (
    <div
      ref={containerRef}
      className={className ?? 'h-64 rounded-lg overflow-hidden'}
    />
  )
}

function getBounds(coords: number[][]): [[number, number], [number, number]] {
  let minLng = Infinity, maxLng = -Infinity, minLat = Infinity, maxLat = -Infinity
  for (const [lng, lat] of coords) {
    if (lng < minLng) minLng = lng
    if (lng > maxLng) maxLng = lng
    if (lat < minLat) minLat = lat
    if (lat > maxLat) maxLat = lat
  }
  return [[minLng, minLat], [maxLng, maxLat]]
}

function findNearestIndex(coords: number[][], target: number[]) {
  let bestIdx = 0, bestDist = Infinity
  for (let i = 0; i < coords.length; i++) {
    const d = (coords[i][0] - target[0]) ** 2 + (coords[i][1] - target[1]) ** 2
    if (d < bestDist) { bestDist = d; bestIdx = i }
  }
  return { index: bestIdx, distance: Math.sqrt(bestDist) }
}
