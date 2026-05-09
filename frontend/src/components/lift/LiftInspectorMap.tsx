import { useEffect, useRef, useState } from 'react'
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

// Legend entry definition matching liftColor() — keeps color encoding and
// text label in sync (F16 a11y: dual channel, color + text).
const LEGEND_ENTRIES: { dot: string; label: string; dashed?: boolean }[] = [
  { dot: '#3b82f6', label: 'OSM-Treffer' },
  { dot: '#22c55e', label: 'Heuristik (hoch)' },
  { dot: '#f97316', label: 'Heuristik (mittel)' },
  { dot: '#8b5cf6', label: 'Heuristik (bidirektional)' },
  { dot: '#ef4444', label: 'Abgelehnt', dashed: true },
]

export default function LiftInspectorMap({
  candidates,
  streams,
  highlightedCandidateId,
  onCandidateClick,
}: LiftInspectorMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const initMapRef = useRef<(() => void) | null>(null)
  // Track which candidate IDs currently have sources/layers on the map so we
  // can remove stale ones when the candidates prop changes (F10 fix).
  const mountedCandidateIdsRef = useRef<Set<string>>(new Set())
  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const baseLayer = base[0]

  const gpsStreams = streams.filter(
    (s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null,
  )
  const trackCoords: [number, number][] = gpsStreams.map(s => [s.lng, s.lat])
  const hasStream = trackCoords.length >= 2

  // Track whether the map has been fully initialized and the style loaded.
  const [mapReady, setMapReady] = useState(false)

  // ── Effect 1: mount — initializes map, ResizeObserver, style listeners ────────
  // Depends on [] — runs once. Style-switch listeners are registered symmetrically
  // with m.off(...) to prevent double-registration on style reload.
  useEffect(() => {
    const el = mapContainerRef.current
    if (!el) return

    let map: maplibregl.Map | null = null
    let disposed = false

    const addTrackLayer = (m: maplibregl.Map) => {
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

      if (trackCoords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const coord of trackCoords) bounds.extend(coord)
        m.fitBounds(bounds, { padding: 40, maxZoom: 16, animate: false })
      }
    }

    const onLoad = () => {
      if (!map) return
      map.resize()
      addTrackLayer(map)
      setMapReady(true)
    }
    const onStyleLoad = () => {
      if (!map) return
      addTrackLayer(map)
      // Clear the mounted-candidate tracking so the candidates effect
      // re-adds layers after a style switch.
      mountedCandidateIdsRef.current = new Set()
      setMapReady(r => !r ? true : r)
    }

    // Container with no measured size renders nothing — no retry counter, no
    // warning. Acceptable for the modal context where the parent always has size
    // after layout settles; the ResizeObserver below fires initMap on first resize.
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
      map.on('load', onLoad)
      map.on('style.load', onStyleLoad)

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
      // Remove event listeners before map.remove() to avoid dangling callbacks.
      map?.off('load', onLoad)
      map?.off('style.load', onStyleLoad)
      map?.remove()
      mapRef.current = null
      mountedCandidateIdsRef.current = new Set()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Retry init when useMapLayers / useCapabilities resolve after mount.
  useEffect(() => {
    if (mapRef.current) return
    initMapRef.current?.()
  }, [baseLayer, caps])

  // ── Effect 2: candidates — re-renders lift polylines when the prop changes ────
  // Triggered by changes to candidates (e.g. soft-reject mid-session) so the
  // dashed-rejected line updates without needing a modal close/reopen (F10 fix).
  useEffect(() => {
    const m = mapRef.current
    if (!m || !m.isStyleLoaded()) return

    const incoming = new Set(candidates.map(c => c.id))

    // Remove layers for candidates that are no longer in the prop.
    for (const oldId of mountedCandidateIdsRef.current) {
      if (!incoming.has(oldId)) {
        if (m.getLayer(`lift-line-${oldId}`)) m.removeLayer(`lift-line-${oldId}`)
        if (m.getSource(`lift-${oldId}`)) m.removeSource(`lift-${oldId}`)
        mountedCandidateIdsRef.current.delete(oldId)
      }
    }

    for (const c of candidates) {
      const sourceId = `lift-${c.id}`
      const layerId = `lift-line-${c.id}`
      const coords = streamSegmentForLift(streams, c.started_at, c.ended_at)

      if (coords.length < 2) {
        // Candidate's time range is not covered by the stream with enough GPS
        // points to draw a line — log in dev to aid debugging, skip silently otherwise.
        if (import.meta.env.DEV) {
          console.warn(`[LiftInspectorMap] candidate ${c.id} has only ${coords.length} GPS point(s) in stream — skipping map layer`)
        }
        continue
      }

      if (mountedCandidateIdsRef.current.has(c.id)) {
        // Layer already exists — update paint properties and source data in place
        // to reflect state changes (e.g. rejection, highlight) without full removal.
        const src = m.getSource(sourceId) as maplibregl.GeoJSONSource | undefined
        src?.setData({
          type: 'Feature',
          geometry: { type: 'LineString', coordinates: coords },
          properties: { candidateId: c.id },
        })
        if (m.getLayer(layerId)) {
          m.setPaintProperty(layerId, 'line-color', liftColor(c))
          m.setPaintProperty(layerId, 'line-width', 4)
          m.setPaintProperty(layerId, 'line-dasharray', c.rejected_by_user ? [4, 3] : [1, 0])
        }
      } else {
        // New candidate — add source + layer.
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
        mountedCandidateIdsRef.current.add(c.id)
      }
    }
  // mapReady is included so this re-runs once the map finishes loading on first render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [candidates, streams, mapReady])

  // ── Effect 3: highlight — updates line width/color when selection changes ─────
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
    <div className="relative w-full h-full" data-testid="lift-inspector-map-wrapper">
      <div
        ref={mapContainerRef}
        className="w-full h-full rounded-lg overflow-hidden"
        data-testid="lift-inspector-map"
      />

      {/* F11: empty-stream fallback overlay — shown above map without hiding the
          container (hiding would break the ResizeObserver and prevent map init). */}
      {!hasStream && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none rounded-lg bg-neutral-100/70 dark:bg-neutral-900/70">
          <p className="text-sm text-neutral-500 dark:text-neutral-400 text-center px-4">
            Karte nicht verfügbar — fehlender Stream
          </p>
        </div>
      )}

      {/* F16: on-map legend — color swatch + German text label for each line type.
          Both channels (color + text) are present so the encoding is accessible
          without color vision. */}
      <div
        className="absolute top-2 left-2 z-10 rounded-lg bg-white/90 dark:bg-neutral-900/90 border border-neutral-200 dark:border-neutral-700 shadow-sm px-2 py-1.5 text-xs space-y-1"
        aria-label="Kartenlegende"
      >
        {LEGEND_ENTRIES.map(entry => (
          <div key={entry.label} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              style={{
                display: 'inline-block',
                width: 20,
                height: 4,
                borderRadius: 2,
                backgroundColor: entry.dot,
                ...(entry.dashed
                  ? { backgroundImage: `repeating-linear-gradient(to right, ${entry.dot} 0, ${entry.dot} 4px, transparent 4px, transparent 7px)`, backgroundColor: 'transparent' }
                  : {}),
              }}
            />
            <span className="text-neutral-700 dark:text-neutral-300">{entry.label}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
