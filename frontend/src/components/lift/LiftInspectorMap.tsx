import { useEffect, useRef, useState, useMemo } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { HelpCircle, Flame } from 'lucide-react'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import { useTheme } from '@/lib/theme'
import { addHeatmapLayers, removeHeatmapLayers, buildHeatmapTileUrl, loadHeatmapColor } from '@/lib/mapHeatmap'
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
  onOpenHelp?: () => void
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

/**
 * Display labels keyed by (source / confidence) — never exposes backend IDs.
 * Exported so LiftReviewModal can share the same lookup without duplication.
 */
export const CONFIDENCE_LABEL = {
  osm:      'OSM-Treffer',
  high:     'Heuristik (Speed + Steigung)',
  medium:   'Heuristik (Höhenrate)',
  bidir:    'Heuristik (bidirektional)',
  rejected: 'Abgelehnt',
} as const

/**
 * Tooltip descriptions for each confidence/source variant.
 * Exported so LiftReviewModal can reuse the same strings for badge title attributes
 * without duplicating them.
 */
export const CONFIDENCE_DESCRIPTIONS: Record<keyof typeof CONFIDENCE_LABEL, string> = {
  osm:
    'Geometrie-Match gegen einen OSM-Lift-Eintrag (Methode A). Zählt sofort, ohne Review.',
  high:
    'Heuristik-Detektor: stabile Geschwindigkeit + ausreichend steiler Gradient — klares Lift-Signal aus den Sensordaten.',
  medium:
    'Heuristik-Detektor ohne brauchbares Speed-Signal — nur über Höhenmeter pro Sekunde erkannt (typisch bei Smart-Recording).',
  bidir:
    'Lift mit U- oder ∩-förmigem Höhenprofil — das Cable bewegt sich während dieser einen Fahrt sowohl auf- als auch abwärts. „Bidirektional" bezieht sich auf die Höhenbewegung der Cable, nicht auf die Richtung des Nutzers (kein Hin-und-zurück).',
  rejected:
    'Per Soft-Reject ausgeblendet — wird nicht in Lifetime-Stats gezählt; "Wiederherstellen" macht es rückgängig.',
}

/**
 * Maps a LiftCandidate to its CONFIDENCE_LABEL key for display purposes.
 * The backend's `confidence` field values ('high'|'medium'|'bidir') remain unchanged;
 * only the UI display text is signal-based.
 */
export function getConfidenceKey(c: LiftCandidate): keyof typeof CONFIDENCE_LABEL {
  if (c.rejected_by_user) return 'rejected'
  if (c.source === 'osm') return 'osm'
  if (c.confidence === 'high') return 'high'
  if (c.confidence === 'bidir') return 'bidir'
  return 'medium'
}

// Legend entry definition matching liftColor() — keeps color encoding and
// text label in sync (F16 a11y: dual channel, color + text).
const LEGEND_ENTRIES: { dot: string; key: keyof typeof CONFIDENCE_LABEL; dashed?: boolean }[] = [
  { dot: '#3b82f6', key: 'osm' },
  { dot: '#22c55e', key: 'high' },
  { dot: '#f97316', key: 'medium' },
  { dot: '#8b5cf6', key: 'bidir' },
  { dot: '#ef4444', key: 'rejected', dashed: true },
]

export default function LiftInspectorMap({
  candidates,
  streams,
  highlightedCandidateId,
  onCandidateClick,
  onOpenHelp,
}: LiftInspectorMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const initMapRef = useRef<(() => void) | null>(null)
  // Track which candidate IDs currently have sources/layers on the map so we
  // can remove stale ones when the candidates prop changes (F10 fix).
  const mountedCandidateIdsRef = useRef<Set<string>>(new Set())
  // Refs for track coords — updated each render so addTrackLayer closures always
  // see the latest data even when Effect 1 runs only once.
  const continuousTrackCoordsRef = useRef<[number, number][]>([])
  const gapBridgesRef = useRef<[number, number][][]>([])
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

  // Fix 5a — Continuous LineString of all valid GPS coords.
  // The track is drawn as one unbroken line; gap bridges are rendered as a
  // separate dashed-amber overlay layer so discontinuities are still visible.
  const continuousTrackCoords = useMemo(
    () => gpsStreams.map(s => [s.lng, s.lat] as [number, number]),
    [gpsStreams],
  )

  // Fix 5b — Keep splitTrackAtGaps for gap-bridge computation.
  // The bridge for each adjacent segment pair is the two-point line
  // [lastOf(seg_i), firstOf(seg_{i+1})].
  const trackSegments = useMemo(
    () => splitTrackAtGaps(streams, PAUSE_GAP_THRESHOLD_S),
    [streams],
  )

  const gapBridges = useMemo((): [number, number][][] => {
    if (trackSegments.length < 2) return []
    const bridges: [number, number][][] = []
    for (let i = 0; i < trackSegments.length - 1; i++) {
      const seg = trackSegments[i]
      const next = trackSegments[i + 1]
      if (seg.length > 0 && next.length > 0) {
        bridges.push([seg[seg.length - 1], next[0]])
      }
    }
    return bridges
  }, [trackSegments])

  // Keep refs in sync so the Effect-1 closure ([] deps) sees latest coords.
  continuousTrackCoordsRef.current = continuousTrackCoords
  gapBridgesRef.current = gapBridges

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
      // Fix 5a: activity track as continuous LineString — all valid GPS coords.
      // Gap-bridge overlay (inspector-track-bridges) renders the teleport segments
      // in dashed amber so discontinuities are visible without breaking the line.
      // Read from ref so this closure always uses the latest coords even with [] deps.
      const coords = continuousTrackCoordsRef.current
      const bridges = gapBridgesRef.current
      m.addSource('inspector-track', {
        type: 'geojson',
        data: {
          type: 'Feature',
          geometry: { type: 'LineString', coordinates: coords },
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

      // Fix 5b: dashed-amber bridge layer over each gap section, inserted above
      // the gray track line so the gap is clearly marked.
      m.addSource('inspector-track-bridges', {
        type: 'geojson',
        data: {
          type: 'Feature',
          geometry: { type: 'MultiLineString', coordinates: bridges },
          properties: {},
        },
      })
      // Pause-bridges painted in the same gray as the track but dashed and a
      // touch heavier so the gap is visible without recruiting a colour that
      // the legend already binds to one of the heuristic classes (orange = medium,
      // amber overlapped visually with that). Track-coloured dashes read as
      // 'track, but interrupted'.
      m.addLayer({
        id: 'inspector-track-bridges-line',
        type: 'line',
        source: 'inspector-track-bridges',
        layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': '#999999',
          'line-width': 2,
          'line-dasharray': [3, 3],
          'line-opacity': 0.7,
        },
      })

      // Fit to all GPS points.
      if (coords.length >= 2) {
        const bounds = new maplibregl.LngLatBounds()
        for (const coord of coords) bounds.extend(coord)
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
  // the picker. The map is initially created with `baseLayer` already selected;
  // we must NOT call setStyle() on first render or it triggers a redundant
  // "rebuild from scratch" that leaves isStyleLoaded() = false during the very
  // window where Effect 2 tries to add lift-line layers — which then silently
  // skips because the style isn't ready. So on the first render after the map
  // exists we just snapshot the initial baseLayer.id and bail.
  const prevBaseLayerIdRef = useRef<number | undefined>(undefined)
  useEffect(() => {
    const m = mapRef.current
    if (!m || !baseLayer) return
    if (prevBaseLayerIdRef.current === undefined) {
      prevBaseLayerIdRef.current = baseLayer.id
      return
    }
    if (prevBaseLayerIdRef.current === baseLayer.id) return
    prevBaseLayerIdRef.current = baseLayer.id
    const style = buildLayerStyle(baseLayer, caps ?? {})
    if (style) {
      m.setStyle(style as import('maplibre-gl').StyleSpecification | string)
    }
  }, [baseLayer, caps])

  // ── Effect 2: candidates — re-renders lift polylines when the prop changes ────
  // Triggered by changes to candidates (e.g. soft-reject mid-session) so the
  // dashed-rejected line updates without needing a modal close/reopen (F10 fix).
  //
  // Gate on `mapReady` rather than `m.isStyleLoaded()`. The latter returns
  // false transiently after we addSource() inside onStyleLoad (the new sources
  // are flagged "pending" until MapLibre's next idle tick), which means Effect 2
  // would bail right after a style.load and never get re-invoked. mapReady is
  // sticky-true once `load` has fired and is enough to guarantee addSource/
  // addLayer work — they are synchronous style mutations that don't require all
  // sources to be loaded.
  useEffect(() => {
    const m = mapRef.current
    if (!m || !mapReady) return

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
  // Gate on mapReady, see Effect 2 rationale.
  useEffect(() => {
    const m = mapRef.current
    if (!m || !mapReady) return
    for (const c of candidates) {
      const layerId = `lift-line-${c.id}`
      if (!m.getLayer(layerId)) continue
      const highlighted = c.id === highlightedCandidateId
      m.setPaintProperty(layerId, 'line-width', highlighted ? 7 : 4)
      m.setPaintProperty(layerId, 'line-color', highlighted ? '#e879f9' : liftColor(c))
    }
  }, [highlightedCandidateId, candidates, mapReady])

  // ── Effect 4: heatmap — adds / removes heatmap layers when toggle changes ─────
  // styleVersion in deps ensures the heatmap is re-added after a style switch
  // (style.load wipes all custom layers). Gate on mapReady, see Effect 2 rationale.
  useEffect(() => {
    const m = mapRef.current
    if (!m || !mapReady) return
    removeHeatmapLayers(m)
    if (showHeatmap && userId && userId !== 'anon') {
      // Insert heatmap below lift-line layers so lift polylines remain on top.
      const firstLiftLayer = candidates[0] ? `lift-line-${candidates[0].id}` : undefined
      // Reuse the user's persisted heatmap color from MapPage (localStorage)
      // so the inspector view stays consistent with the global map.
      addHeatmapLayers(m, buildHeatmapTileUrl(userId), loadHeatmapColor(), firstLiftLayer)
    }
  }, [showHeatmap, mapReady, styleVersion, userId, candidates])

  // ── Effect 5: direction arrows — HTML Marker arrows along the continuous track ──
  // Markers are DOM elements and survive style switches, but we include styleVersion
  // so the effect re-runs after a style reload for safety. Capped at ~15 arrows.
  useEffect(() => {
    const m = mapRef.current
    if (!m || !mapReady) return
    const markers: maplibregl.Marker[] = []
    const n = continuousTrackCoords.length
    if (n < 2) return
    const interval = Math.max(20, Math.floor(n / 15))
    for (let i = 0; i < n - 1; i += interval) {
      const [lng1, lat1] = continuousTrackCoords[i]
      const next = continuousTrackCoords[Math.min(i + 5, n - 1)]
      const [lng2, lat2] = next
      const bearingDeg = (Math.atan2(lng2 - lng1, lat2 - lat1) * 180) / Math.PI
      const el = document.createElement('div')
      el.style.pointerEvents = 'none'
      el.innerHTML = `<svg width="14" height="14" viewBox="0 0 20 20" style="transform: rotate(${bearingDeg}deg);"><path d="M10 2 L16 16 L10 13 L4 16 Z" fill="#525252" stroke="#fff" stroke-width="1.2"/></svg>`
      markers.push(
        new maplibregl.Marker({ element: el, rotationAlignment: 'map' })
          .setLngLat([lng1, lat1])
          .addTo(m),
      )
    }
    return () => markers.forEach(mk => mk.remove())
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [continuousTrackCoords, mapReady, styleVersion])

    // The outer wrapper is the positioning context for legend / picker / heatmap
    // overlays (z-10+). min-h-[480px] is a safety net: if the flex-height
    // cascade collapses to 0 (modal layout edge cases, missing min-h-0 somewhere
    // up the tree), the wrapper still gives the map container a non-zero size.
    // The map container itself is a plain w-full h-full child — NOT absolute,
    // because MapLibre overrides `position: absolute` on its container with
    // `position: relative` during init, which would null out inset-0 stretching.
    return (
    <div className="relative w-full h-full min-h-[480px]" data-testid="lift-inspector-map-wrapper">
      <div
        ref={mapContainerRef}
        className="w-full h-full rounded-lg overflow-hidden bg-neutral-100 dark:bg-neutral-800"
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

      {/* Toolbar bottom-left — layer picker as inline pill-buttons + heatmap toggle
          as a sibling card. NavigationControl owns top-right (MapLibre default),
          legend sits top-left, controls bottom-left. */}
      <div className="absolute bottom-3 left-3 z-10 flex items-center gap-1.5 flex-wrap pointer-events-auto">
        {base.length > 1 && (
          <div className="flex items-center gap-0.5 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md p-0.5">
            {base.map(l => (
              <button
                key={l.id}
                onClick={() => setSelectedBaseLayer(l)}
                className={[
                  'px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer',
                  selectedBaseLayer?.id === l.id
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700',
                ].join(' ')}
              >
                {l.name}
              </button>
            ))}
          </div>
        )}
        <div className="flex items-center gap-1 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm rounded-lg shadow-md px-2 py-1">
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
            <Flame className="w-3.5 h-3.5" aria-hidden="true" />
            Heatmap
          </button>
        </div>
      </div>

      {/* Legend top-left — line-style → meaning mapping plus a help-icon that
          opens LiftHelpModal. */}
      <div
        className="absolute top-3 left-3 z-10 rounded-lg bg-white/90 dark:bg-neutral-900/90 border border-neutral-200 dark:border-neutral-700 shadow-md px-3 py-2 text-xs space-y-1 pointer-events-auto"
        aria-label="Kartenlegende"
      >
        <div className="flex items-center justify-between gap-2 mb-0.5">
          <span className="text-neutral-500 dark:text-neutral-400 font-medium text-[11px] uppercase tracking-wide">Legende</span>
          {onOpenHelp && (
            <button
              onClick={onOpenHelp}
              aria-label="Hilfe zur Lifterkennungslegende öffnen"
              className="text-neutral-400 dark:text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-200 cursor-pointer transition-colors"
            >
              <HelpCircle className="w-4 h-4" aria-hidden="true" />
            </button>
          )}
        </div>
        {LEGEND_ENTRIES.map(entry => (
          <div
            key={entry.key}
            className="flex items-center gap-1.5"
            title={CONFIDENCE_DESCRIPTIONS[entry.key]}
          >
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
            <span className="text-neutral-700 dark:text-neutral-300">{CONFIDENCE_LABEL[entry.key]}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
