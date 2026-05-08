import { useEffect, useRef } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import type { LiftCandidate, StreamPoint } from '@/api/types'

interface LiftInspectorMapProps {
  candidates: LiftCandidate[]
  streams: StreamPoint[]
  highlightedCandidateId: string | null
  onCandidateClick: (id: string) => void
}

// Returns the map line color for a lift candidate.
function liftColor(c: LiftCandidate): string {
  if (c.rejected_by_user) return '#ef4444'
  if (c.source === 'osm') return '#3b82f6'
  if (c.confidence === 'high') return '#22c55e'
  if (c.confidence === 'bidir') return '#8b5cf6'
  return '#f97316'
}

// Extracts [lng, lat] pairs from the stream that fall within a lift's time range.
function streamSegmentForLift(
  streams: StreamPoint[],
  startedAt: string,
  endedAt: string,
): [number, number][] {
  const t0 = new Date(startedAt).getTime()
  const t1 = new Date(endedAt).getTime()
  return streams
    .filter(s => {
      if (s.lat == null || s.lng == null) return false
      const t = new Date(s.time).getTime()
      return t >= t0 && t <= t1
    })
    .map(s => [s.lng!, s.lat!])
}

export default function LiftInspectorMap({
  candidates,
  streams,
  highlightedCandidateId,
  onCandidateClick,
}: LiftInspectorMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const initMapRef = useRef<(() => void) | null>(null)
  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const baseLayer = base[0]

  useEffect(() => {
    const el = mapContainerRef.current
    if (!el) return

    let map: maplibregl.Map | null = null
    let disposed = false

    const gpsStreams = streams.filter(
      (s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null,
    )
    const trackCoords: [number, number][] = gpsStreams.map(s => [s.lng, s.lat])

    const addLayers = (m: maplibregl.Map) => {
      if (m.getSource('inspector-track')) return

      // Gray activity track as background
      m.addSource('inspector-track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          geometry: { type: 'LineString', coordinates: trackCoords },
          properties: {},
        },
      })
      m.addLayer({
        id: 'inspector-track-line',
        type: 'line',
        source: 'inspector-track',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#999999', 'line-width': 2, 'line-opacity': 0.7 },
      })

      // One layer per lift candidate, colored by source + confidence
      for (const c of candidates) {
        const coords = streamSegmentForLift(streams, c.started_at, c.ended_at)
        if (coords.length < 2) continue

        const sourceId = `lift-${c.id}`
        const layerId = `lift-line-${c.id}`

        m.addSource(sourceId, {
          type: 'geojson',
          data: {
            type: 'Feature',
            geometry: { type: 'LineString', coordinates: coords },
            properties: { candidateId: c.id },
          },
        })
        m.addLayer({
          id: layerId,
          type: 'line',
          source: sourceId,
          layout: { 'line-join': 'round', 'line-cap': 'round' },
          paint: {
            'line-color': liftColor(c),
            'line-width': 4,
            'line-opacity': 0.9,
            ...(c.rejected_by_user ? { 'line-dasharray': [4, 3] } : {}),
          },
        })

        m.on('click', layerId, () => onCandidateClick(c.id))
        m.on('mouseenter', layerId, () => { m.getCanvas().style.cursor = 'pointer' })
        m.on('mouseleave', layerId, () => { m.getCanvas().style.cursor = '' })
      }

      if (trackCoords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const coord of trackCoords) bounds.extend(coord)
        m.fitBounds(bounds, { padding: 40, maxZoom: 16, animate: false })
      }
    }

    const initMap = () => {
      if (disposed || map) return
      if (el.clientWidth === 0 || el.clientHeight === 0) return

      const style = baseLayer ? buildLayerStyle(baseLayer, caps ?? {}) : null
      if (!style) return

      map = new maplibregl.Map({
        container: el,
        style: style as import('maplibre-gl').StyleSpecification | string,
        attributionControl: {},
      })

      map.addControl(new maplibregl.NavigationControl(), 'top-right')
      map.on('load', () => { map!.resize(); addLayers(map!) })
      map.on('style.load', () => { addLayers(map!) })

      mapRef.current = map
    }

    initMapRef.current = initMap

    const ro = new ResizeObserver(() => {
      if (!map) { initMap() } else { map.resize() }
    })
    ro.observe(el)
    initMap()

    return () => {
      disposed = true
      ro.disconnect()
      initMapRef.current = null
      map?.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Retry init when useMapLayers / useCapabilities resolve after mount.
  useEffect(() => {
    if (mapRef.current) return
    initMapRef.current?.()
  }, [baseLayer, caps])

  // Update highlight paint when selection changes.
  useEffect(() => {
    const m = mapRef.current
    if (!m || !m.isStyleLoaded()) return
    for (const c of candidates) {
      const layerId = `lift-line-${c.id}`
      if (!m.getLayer(layerId)) continue
      const highlighted = c.id === highlightedCandidateId
      m.setPaintProperty(layerId, 'line-width', highlighted ? 7 : 4)
      m.setPaintProperty(layerId, 'line-color', highlighted ? '#e879f9' : liftColor(c))
    }
  }, [highlightedCandidateId, candidates])

  return (
    <div
      ref={mapContainerRef}
      className="w-full h-full rounded-lg overflow-hidden"
      data-testid="lift-inspector-map"
    />
  )
}
