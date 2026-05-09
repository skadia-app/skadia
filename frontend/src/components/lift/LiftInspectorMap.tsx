import { useEffect, useRef, useState, useMemo } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import { useTheme } from '@/lib/theme'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl } from '@/lib/mapHeatmap'
import { useCurrentUserId } from '@/providers/UserProvider'
import type { MapLayer } from '@/api/mapLayers'
import type { LiftCandidate, StreamPoint } from '@/api/types'

// Pause gaps longer than this threshold are treated as hard breaks — no lift
// candidate spanning the gap will be detected. MUST stay in sync with backend
// enrichment/lifts/window.go SmartRecMaxGapS default (currently 75s).
const PAUSE_GAP_THRESHOLD_S = 75

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

/**
 * Splits the stream into contiguous segments, breaking at every consecutive
 * point pair whose time delta exceeds gapThresholdS. Returned as a
 * MultiLineString coordinates array — each inner array is one segment.
 *
 * Points without lat/lng are skipped. A break resets the current segment.
 */
export function splitTrackAtGaps(
  streams: StreamPoint[],
  gapThresholdS: number,
): [number, number][][] {
  const segments: [number, number][][] = []
  let current: [number, number][] = []
  let prevTime: number | null = null

  for (const s of streams) {
    if (s.lat == null || s.lng == null) continue
    const t = s.time ? new Date(s.time).getTime() : null
    if (t !== null && prevTime !== null) {
      const dt = (t - prevTime) / 1000
      if (dt > gapThresholdS && current.length > 0) {
        segments.push(current)
        current = []
      }
    }
    current.push([s.lng!, s.lat!])
    if (t !== null) prevTime = t
  }
  if (current.length > 0) segments.push(current)
  return segments
}

/**
 * Selects the most appropriate base layer for the inspector given the active
 * UI theme.
 *
 * Rules (in priority order):
 *   1. Dark mode → prefer a layer with key === 'dark' (matches MapPage.tsx:333
 *      convention for the SCOPE-GAP dark-layer check).
 *   2. Light mode → skip layers whose key or name matches
 *      /satellite|aerial|imagery|sat\b/i (satellite tiles hide OSM lift
 *      glyphs). Pick the first remaining layer (respects existing sort_order).
 *   3. Fallback → return layers[0] (or undefined if the array is empty).
 */
export function pickDefaultBase(
  layers: MapLayer[],
  theme: 'light' | 'dark',
): MapLayer | undefined {
  if (layers.length === 0) return undefined
  if (theme === 'dark') {
    const dark = layers.find(l => l.key === 'dark')
    if (dark) return dark
  }
  // Light mode (or dark with no 'dark' key): skip satellite-type layers so OSM
  // lift glyphs and labels remain readable by default.
  const satelliteRe = /satellite|aerial|imagery|\bsat\b/i
  const notSat = layers.find(l => !satelliteRe.test(l.key) && !satelliteRe.test(l.name))
  if (notSat) return notSat
  return layers[0]
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
  const { theme } = useTheme()
  const userId = useCurrentUserId()

  // A. Base-layer picker — component-local state; resets to smart default when
  // modal session closes (the component unmounts). Initial pick synchronised
  // when base[] first becomes non-empty via the effect below.
  const [selectedBaseLayer, setSelectedBaseLayer] = useState<MapLayer | undefined>(undefined)

  // F. Heatmap toggle — off by default; no color-picker per scope (user didn't ask).
  const [showHeatmap, setShowHeatmap] = useState(false)

  // styleVersion increments on every style.load so heatmap/candidates effects
  // re-run after a style switch even when their other deps haven't changed.
  const [styleVersion, setStyleVersion] = useState(0)

  // Synchronise selectedBaseLayer when base[] resolves: if not yet set (or if
  // the previously selected layer disappeared from the list), pick the smart
  // default. A user's explicit picker choice is preserved as long as the layer
  // stays in the list.
  useEffect(() => {
    if (base.length === 0) return
    setSelectedBaseLayer(prev => {
      if (prev && base.some(l => l.id === prev.id)) return prev
      return pickDefaultBase(base, theme)
    })
  // Re-run when the layer list or theme changes. Only replaces if the layer vanished.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [base, theme])

  const baseLayer = selectedBaseLayer ?? base[0]

  const gpsStreams = streams.filter(
    (s): s is StreamPoint & { lat: number; lng: number } => s.lat != null && s.lng != null,
  )
  const hasStream = gpsStreams.length >= 2

  // D.1 — Split the activity track at pause gaps > PAUSE_GAP_THRESHOLD_S.
  // Produces a MultiLineString coordinates array so gaps render as polyline
  // breaks rather than straight teleportation lines.
  const trackSegments = useMemo(
    () => splitTrackAtGaps(streams, PAUSE_GAP_THRESHOLD_S),
    [streams],
  )

  // C. Memoize per-candidate coords keyed on [candidates, streams] — avoids
  // recomputing streamSegmentForLift (O(|streams|) per candidate) on every
  // style-switch triggered re-render.
  const candidateCoords = useMemo(() => {
    const map = new Map<string, [number, number][]>()
    for (const c of candidates) {
      map.set(c.id, streamSegmentForLift(streams, c.started_at, c.ended_at))
    }
    return map
  }, [candidates, streams])

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
      // D.1: activity track rendered as MultiLineString so pause-gaps break the
      // polyline visually instead of teleporting straight through the gap.
      m.addSource('inspector-track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          geometry: { type: 'MultiLineString', coordinates: trackSegments },
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

      // Fit to all GPS points across all segments.
      const allCoords = trackSegments.flat()
      if (allCoords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const coord of allCoords) bounds.extend(coord)
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
      // Increment styleVersion so heatmap / candidates effects re-run after the
      // style switch even when their other deps haven't changed.
      setStyleVersion(v => v + 1)
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

  // A. Style-switch effect — triggers when the user changes the base-layer via
  // the picker. Does nothing on first render (map not yet created) or when the
  // selected layer matches what was used for map init.
  const prevBaseLayerIdRef = useRef<number | undefined>(undefined)
  useEffect(() => {
    const m = mapRef.current
    if (!m || !baseLayer) return
    if (prevBaseLayerIdRef.current === baseLayer.id) return
    prevBaseLayerIdRef.current = baseLayer.id
    const style = buildLayerStyle(baseLayer, caps ?? {})
    if (style) {
      m.setStyle(style as import('maplibre-gl').StyleSpecification | string)
      // onStyleLoad fires after setStyle completes; it clears mountedCandidateIdsRef
      // and increments styleVersion to trigger the candidates + heatmap effects.
    }
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
      // C: read from memoized map instead of recomputing per candidate.
      const coords = candidateCoords.get(c.id) ?? []

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
  // mapReady + styleVersion are included so this re-runs once the map finishes
  // loading on first render and after every style switch.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [candidates, candidateCoords, mapReady, styleVersion])

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

  // ── Effect 4: heatmap — adds / removes heatmap layers when toggle changes ─────
  // styleVersion in deps ensures the heatmap is re-added after a style switch
  // (style.load wipes all custom layers).
  useEffect(() => {
    const m = mapRef.current
    if (!m || !m.isStyleLoaded()) return
    removeHeatmapLayers(m)
    if (showHeatmap && userId && userId !== 'anon') {
      // Insert heatmap below lift-line layers so lift polylines remain on top.
      const firstLiftLayer = candidates[0] ? `lift-line-${candidates[0].id}` : undefined
      addHeatmapLayers(m, buildHeatmapTileUrl(userId), '#ff6b35', firstLiftLayer)
    }
  }, [showHeatmap, mapReady, styleVersion, userId, candidates])

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

      {/* A + F: top-right control strip — base-layer picker and heatmap toggle.
          Positioned below the MapLibre NavigationControl (~88px tall at top-right). */}
      <div className="absolute top-28 right-2 z-10 flex flex-col gap-1.5 items-end">
        {/* A. Base-layer picker — only rendered when more than one base layer is available. */}
        {base.length > 1 && (
          <select
            value={selectedBaseLayer?.id ?? ''}
            onChange={e => {
              const id = Number(e.target.value)
              const layer = base.find(l => l.id === id)
              if (layer) setSelectedBaseLayer(layer)
            }}
            aria-label="Karten-Stil wählen"
            className="
              rounded-lg bg-white/90 dark:bg-neutral-900/90
              border border-neutral-200 dark:border-neutral-700
              shadow-sm px-2 py-1 text-xs
              text-neutral-700 dark:text-neutral-300
              cursor-pointer
              focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT
            "
          >
            {base.map(l => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        )}

        {/* F. Heatmap toggle button — same visual pattern as MultiTrackMap.tsx:255-271. */}
        <div className="bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
          <button
            onClick={() => setShowHeatmap(v => !v)}
            title={showHeatmap ? 'Heatmap ausblenden' : 'Heatmap einblenden'}
            data-testid="heatmap-toggle"
            className={[
              'flex items-center gap-1.5 text-xs font-medium rounded-md px-1.5 py-0.5 transition-colors cursor-pointer',
              showHeatmap
                ? 'text-accent-DEFAULT'
                : 'text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white',
            ].join(' ')}
          >
            <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 2C8.5 6 6 9 6 13a6 6 0 0012 0c0-2.5-1-4.5-2.5-6.5C14.5 8.5 13.5 10 13 12c-.5-1.5-.5-3 0-5.5C12.5 5 12.2 3.5 12 2z"/>
            </svg>
            Heatmap
          </button>
        </div>
      </div>
    </div>
  )
}
